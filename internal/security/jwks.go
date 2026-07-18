package security

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"sort"
	"sync"

	identra_v1_pb "github.com/slhmy/identra/gen/go/identra/v1"
)

const (
	// RSAKeySize is the size of RSA keys in bits
	RSAKeySize = 2048
)

// KeyState represents the lifecycle state of a signing key
type KeyState string

const (
	// KeyStateActive indicates the key is currently used for signing new tokens
	KeyStateActive KeyState = "ACTIVE"
	// KeyStatePassive indicates the key is published for verification but not used for signing.
	KeyStatePassive KeyState = "PASSIVE"
	// KeyStateRetired indicates the key is no longer published and should be removed
	KeyStateRetired KeyState = "RETIRED"
)

// KeyEntry represents a single key in the key ring with its lifecycle state
type KeyEntry struct {
	privateKey *rsa.PrivateKey
	publicKey  *rsa.PublicKey
	keyID      string
	state      KeyState
}

// KeyManager manages RSA key pairs for JWT signing and verification
// with support for key rotation. It maintains a key ring where:
// - Exactly one key is ACTIVE for signing new tokens
// - Zero or more keys are PASSIVE for verification only
// - RETIRED keys are removed from the ring
type KeyManager struct {
	// Legacy single-key fields for backwards compatibility
	privateKey *rsa.PrivateKey
	publicKey  *rsa.PublicKey
	keyID      string

	// Key ring for rotation support
	keys map[string]*KeyEntry // keyed by keyID
	mu   sync.RWMutex
}

var (
	globalKeyManager *KeyManager
	keyManagerOnce   sync.Once
)

// GetKeyManager returns the global KeyManager instance.
func GetKeyManager() *KeyManager {
	keyManagerOnce.Do(func() {
		globalKeyManager = &KeyManager{
			keys: make(map[string]*KeyEntry),
		}
	})
	return globalKeyManager
}

// InitializeFromPEM initializes the key manager from a PEM-encoded private key.
// The key is added to the key ring in ACTIVE state.
// If an ACTIVE key already exists, it is demoted to PASSIVE.
func (km *KeyManager) InitializeFromPEM(privateKeyPEM string) error {
	km.mu.Lock()
	defer km.mu.Unlock()

	// Initialize keys map if not already done
	if km.keys == nil {
		km.keys = make(map[string]*KeyEntry)
	}

	block, _ := pem.Decode([]byte(privateKeyPEM))
	if block == nil {
		return fmt.Errorf("failed to decode PEM block")
	}

	var privateKey *rsa.PrivateKey
	var err error

	switch block.Type {
	case "RSA PRIVATE KEY":
		privateKey, err = x509.ParsePKCS1PrivateKey(block.Bytes)
	case "PRIVATE KEY":
		key, parseErr := x509.ParsePKCS8PrivateKey(block.Bytes)
		if parseErr != nil {
			return fmt.Errorf("failed to parse PKCS8 private key: %w", parseErr)
		}
		var ok bool
		privateKey, ok = key.(*rsa.PrivateKey)
		if !ok {
			return fmt.Errorf("private key is not RSA")
		}
	default:
		return fmt.Errorf("unsupported PEM block type: %s", block.Type)
	}

	if err != nil {
		return fmt.Errorf("failed to parse private key: %w", err)
	}

	publicKey := &privateKey.PublicKey
	keyID := generateKeyID(publicKey)

	// Demote any existing ACTIVE key to PASSIVE
	for _, entry := range km.keys {
		if entry.state == KeyStateActive {
			entry.state = KeyStatePassive
		}
	}

	// Add to key ring as ACTIVE
	km.keys[keyID] = &KeyEntry{
		privateKey: privateKey,
		publicKey:  publicKey,
		keyID:      keyID,
		state:      KeyStateActive,
	}

	// Maintain backwards compatibility
	km.privateKey = privateKey
	km.publicKey = publicKey
	km.keyID = keyID

	return nil
}

// GenerateKeyPair generates a new RSA key pair and adds it to the key ring in ACTIVE state.
// If an ACTIVE key already exists, it is demoted to PASSIVE.
func (km *KeyManager) GenerateKeyPair() error {
	km.mu.Lock()
	defer km.mu.Unlock()

	// Initialize keys map if not already done
	if km.keys == nil {
		km.keys = make(map[string]*KeyEntry)
	}

	privateKey, err := rsa.GenerateKey(rand.Reader, RSAKeySize)
	if err != nil {
		return fmt.Errorf("failed to generate RSA key pair: %w", err)
	}

	publicKey := &privateKey.PublicKey
	keyID := generateKeyID(publicKey)

	// Demote any existing ACTIVE key to PASSIVE
	for _, entry := range km.keys {
		if entry.state == KeyStateActive {
			entry.state = KeyStatePassive
		}
	}

	// Add to key ring as ACTIVE
	km.keys[keyID] = &KeyEntry{
		privateKey: privateKey,
		publicKey:  publicKey,
		keyID:      keyID,
		state:      KeyStateActive,
	}

	// Maintain backwards compatibility
	km.privateKey = privateKey
	km.publicKey = publicKey
	km.keyID = keyID

	return nil
}

// generateKeyID creates a unique key ID based on the public key.
func generateKeyID(publicKey *rsa.PublicKey) string {
	if publicKey == nil {
		return ""
	}

	hash := sha256.Sum256(publicKey.N.Bytes())
	return base64.RawURLEncoding.EncodeToString(hash[:8])
}

// GetPrivateKey returns the RSA private key for signing.
func (km *KeyManager) GetPrivateKey() *rsa.PrivateKey {
	km.mu.RLock()
	defer km.mu.RUnlock()
	return km.privateKey
}

// GetPublicKey returns the RSA public key for verification.
func (km *KeyManager) GetPublicKey() *rsa.PublicKey {
	km.mu.RLock()
	defer km.mu.RUnlock()
	return km.publicKey
}

// GetKeyID returns the key ID.
func (km *KeyManager) GetKeyID() string {
	km.mu.RLock()
	defer km.mu.RUnlock()
	return km.keyID
}

// IsInitialized checks if the key manager has been initialized.
func (km *KeyManager) IsInitialized() bool {
	km.mu.RLock()
	defer km.mu.RUnlock()
	return km.privateKey != nil
}

// ListSigningKeys returns all ACTIVE and PASSIVE public keys.
// This enables smooth key rotation as both old and new keys are published during the transition.
// Keys are sorted by key ID to ensure deterministic output.
func (km *KeyManager) ListSigningKeys() *identra_v1_pb.ListSigningKeysResponse {
	km.mu.RLock()
	defer km.mu.RUnlock()

	var keys []*identra_v1_pb.SigningKey

	// Collect key IDs first to enable sorting for deterministic output
	var keyIDs []string
	for keyID, entry := range km.keys {
		if entry.state == KeyStateActive || entry.state == KeyStatePassive {
			keyIDs = append(keyIDs, keyID)
		}
	}

	// Sort key IDs to ensure deterministic order
	sort.Strings(keyIDs)

	// Build the response in sorted order.
	for _, keyID := range keyIDs {
		entry := km.keys[keyID]
		keys = append(keys, &identra_v1_pb.SigningKey{
			KeyId:     entry.keyID,
			Algorithm: identra_v1_pb.SigningAlgorithm_SIGNING_ALGORITHM_RS256,
			PublicKey: &identra_v1_pb.SigningKey_Rsa{
				Rsa: &identra_v1_pb.RsaPublicKey{
					Modulus:  entry.publicKey.N.Bytes(),
					Exponent: uint32(entry.publicKey.E),
				},
			},
		})
	}

	return &identra_v1_pb.ListSigningKeysResponse{
		Keys: keys,
	}
}

// AddKeyPassive adds a new key to the key ring in PASSIVE state.
// This allows the key to be published for verification before promoting it to ACTIVE.
func (km *KeyManager) AddKeyPassive() (string, error) {
	km.mu.Lock()
	defer km.mu.Unlock()

	// Initialize keys map if not already done
	if km.keys == nil {
		km.keys = make(map[string]*KeyEntry)
	}

	privateKey, err := rsa.GenerateKey(rand.Reader, RSAKeySize)
	if err != nil {
		return "", fmt.Errorf("failed to generate RSA key pair: %w", err)
	}

	publicKey := &privateKey.PublicKey
	keyID := generateKeyID(publicKey)

	km.keys[keyID] = &KeyEntry{
		privateKey: privateKey,
		publicKey:  publicKey,
		keyID:      keyID,
		state:      KeyStatePassive,
	}

	return keyID, nil
}

// PromoteKey promotes a PASSIVE key to ACTIVE state and demotes the current ACTIVE key to PASSIVE.
// This is the core operation for key rotation.
func (km *KeyManager) PromoteKey(keyID string) error {
	km.mu.Lock()
	defer km.mu.Unlock()

	entry, exists := km.keys[keyID]
	if !exists {
		return fmt.Errorf("key not found: %s", keyID)
	}

	if entry.state != KeyStatePassive {
		return fmt.Errorf("key %s is not in PASSIVE state (current: %s)", keyID, entry.state)
	}

	// Demote current ACTIVE key to PASSIVE
	for _, e := range km.keys {
		if e.state == KeyStateActive {
			e.state = KeyStatePassive
		}
	}

	// Promote the specified key to ACTIVE
	entry.state = KeyStateActive

	// Update backwards compatibility fields
	km.privateKey = entry.privateKey
	km.publicKey = entry.publicKey
	km.keyID = entry.keyID

	return nil
}

// DemoteKey demotes an ACTIVE key to PASSIVE state.
// Use this if you need to temporarily stop signing with a key while keeping it for verification.
func (km *KeyManager) DemoteKey(keyID string) error {
	km.mu.Lock()
	defer km.mu.Unlock()

	entry, exists := km.keys[keyID]
	if !exists {
		return fmt.Errorf("key not found: %s", keyID)
	}

	if entry.state != KeyStateActive {
		return fmt.Errorf("key %s is not in ACTIVE state (current: %s)", keyID, entry.state)
	}

	entry.state = KeyStatePassive

	// Clear backwards compatibility fields if this was the active key
	if km.keyID == keyID {
		km.privateKey = nil
		km.publicKey = nil
		km.keyID = ""
	}

	return nil
}

// RetireKey removes a key from the key ring.
// Only PASSIVE keys can be retired. ACTIVE keys must be demoted first.
func (km *KeyManager) RetireKey(keyID string) error {
	km.mu.Lock()
	defer km.mu.Unlock()

	entry, exists := km.keys[keyID]
	if !exists {
		return fmt.Errorf("key not found: %s", keyID)
	}

	if entry.state == KeyStateActive {
		return fmt.Errorf("cannot retire ACTIVE key %s; demote it first", keyID)
	}

	delete(km.keys, keyID)
	return nil
}

// KeyInfo contains information about a key in the key ring.
type KeyInfo struct {
	KeyID string
	State KeyState
}

// ListKeys returns information about all keys in the key ring.
func (km *KeyManager) ListKeys() []KeyInfo {
	km.mu.RLock()
	defer km.mu.RUnlock()

	result := make([]KeyInfo, 0, len(km.keys))

	for _, entry := range km.keys {
		result = append(result, KeyInfo{
			KeyID: entry.keyID,
			State: entry.state,
		})
	}

	return result
}

// ExportPrivateKeyPEM exports the private key in PEM format.
func (km *KeyManager) ExportPrivateKeyPEM() (string, error) {
	km.mu.RLock()
	defer km.mu.RUnlock()

	if km.privateKey == nil {
		return "", fmt.Errorf("no private key available")
	}

	privateKeyBytes := x509.MarshalPKCS1PrivateKey(km.privateKey)
	privateKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: privateKeyBytes,
	})

	return string(privateKeyPEM), nil
}

// ExportPublicKeyPEM exports the public key in PEM format.
func (km *KeyManager) ExportPublicKeyPEM() (string, error) {
	km.mu.RLock()
	defer km.mu.RUnlock()

	if km.publicKey == nil {
		return "", fmt.Errorf("no public key available")
	}

	publicKeyBytes, err := x509.MarshalPKIXPublicKey(km.publicKey)
	if err != nil {
		return "", fmt.Errorf("failed to marshal public key: %w", err)
	}

	publicKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: publicKeyBytes,
	})

	return string(publicKeyPEM), nil
}
