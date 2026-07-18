package security

import (
	"testing"
	"time"

	"github.com/google/uuid"
	identra_v1_pb "github.com/slhmy/identra/gen/go/identra/v1"
)

// Helper function to create test token config with generated keys
func createTestTokenConfig(t *testing.T) TokenConfig {
	t.Helper()

	km := &KeyManager{}
	if err := km.GenerateKeyPair(); err != nil {
		t.Fatalf("Failed to generate key pair: %v", err)
	}

	return TokenConfig{
		PrivateKey:             km.GetPrivateKey(),
		PublicKey:              km.GetPublicKey(),
		KeyID:                  km.GetKeyID(),
		Issuer:                 "test-issuer",
		AccessTokenExpiration:  15 * time.Minute,
		RefreshTokenExpiration: 7 * 24 * time.Hour,
		ServiceTokenExpiration: 15 * time.Minute,
	}
}

func TestServiceTokenClaims(t *testing.T) {
	config := createTestTokenConfig(t)
	token, err := NewServiceToken("isa_test", []string{"identra.users.read"}, config)
	if err != nil {
		t.Fatalf("new service token: %v", err)
	}
	claims, err := ValidateServiceToken(token.Value, config.PublicKey)
	if err != nil {
		t.Fatalf("validate service token: %v", err)
	}
	if claims.ServiceAccountID != "isa_test" || len(claims.Scopes) != 1 || claims.Scopes[0] != "identra.users.read" {
		t.Fatalf("unexpected claims: %+v", claims)
	}
	if _, err := ValidateAccessToken(token.Value, config.PublicKey); err == nil {
		t.Fatal("service token must not validate as a user access token")
	}
}

func TestNewTokenPair(t *testing.T) {
	userID := uuid.New().String()
	config := createTestTokenConfig(t)

	tokenPair, err := NewTokenPair(userID, config)
	if err != nil {
		t.Fatalf("Failed to create token pair: %v", err)
	}

	if tokenPair.AccessToken == nil || tokenPair.AccessToken.Value == "" {
		t.Error("Expected access_token.value to be non-empty")
	}
	if tokenPair.AccessToken == nil || tokenPair.AccessToken.ExpiresAt == nil {
		t.Error("Expected access_token.expires_at to be set")
	}

	if tokenPair.RefreshToken == nil || tokenPair.RefreshToken.Value == "" {
		t.Error("Expected refresh_token.value to be non-empty")
	}
	if tokenPair.RefreshToken == nil || tokenPair.RefreshToken.ExpiresAt == nil {
		t.Error("Expected refresh_token.expires_at to be set")
	}

	expectedAccessExp := time.Now().Add(config.AccessTokenExpiration)
	accessExp := tokenPair.AccessToken.ExpiresAt.AsTime()
	if accessExp.Before(expectedAccessExp.Add(-5*time.Second)) ||
		accessExp.After(expectedAccessExp.Add(5*time.Second)) {
		t.Errorf("Expected access token expiration around %v, got %v", expectedAccessExp, accessExp)
	}

	expectedRefreshExp := time.Now().Add(config.RefreshTokenExpiration)
	refreshExp := tokenPair.RefreshToken.ExpiresAt.AsTime()
	if refreshExp.Before(expectedRefreshExp.Add(-5*time.Second)) ||
		refreshExp.After(expectedRefreshExp.Add(5*time.Second)) {
		t.Errorf("Expected refresh token expiration around %v, got %v", expectedRefreshExp, refreshExp)
	}
}

func TestValidateAccessToken(t *testing.T) {
	userID := uuid.New().String()
	config := createTestTokenConfig(t)

	tokenPair, err := NewTokenPair(userID, config)
	if err != nil {
		t.Fatalf("Failed to create token pair: %v", err)
	}

	claims, err := ValidateAccessToken(tokenPair.AccessToken.Value, config.PublicKey)
	if err != nil {
		t.Fatalf("Failed to validate access token: %v", err)
	}

	if claims.UserID != userID {
		t.Errorf("Expected user ID %s, got %s", userID, claims.UserID)
	}
	if claims.TokenType != AccessTokenType {
		t.Errorf("Expected token type %s, got %s", AccessTokenType, claims.TokenType)
	}
	if claims.TokenID == "" {
		t.Error("Expected token ID to be set")
	}

	if _, err = ValidateRefreshToken(tokenPair.AccessToken.Value, config.PublicKey); err == nil {
		t.Error("Expected access token to fail validation as refresh token")
	}
}

func TestValidateRefreshToken(t *testing.T) {
	userID := uuid.New().String()
	config := createTestTokenConfig(t)

	tokenPair, err := NewTokenPair(userID, config)
	if err != nil {
		t.Fatalf("Failed to create token pair: %v", err)
	}

	claims, err := ValidateRefreshToken(tokenPair.RefreshToken.Value, config.PublicKey)
	if err != nil {
		t.Fatalf("Failed to validate refresh token: %v", err)
	}

	if claims.UserID != userID {
		t.Errorf("Expected user ID %s, got %s", userID, claims.UserID)
	}
	if claims.TokenType != RefreshTokenType {
		t.Errorf("Expected token type %s, got %s", RefreshTokenType, claims.TokenType)
	}

	if _, err = ValidateAccessToken(tokenPair.RefreshToken.Value, config.PublicKey); err == nil {
		t.Error("Expected refresh token to fail validation as access token")
	}
}

func TestRefreshTokenPair(t *testing.T) {
	userID := uuid.New().String()
	config := createTestTokenConfig(t)

	originalPair, err := NewTokenPair(userID, config)
	if err != nil {
		t.Fatalf("Failed to create initial token pair: %v", err)
	}

	newPair, err := RefreshTokenPair(originalPair.RefreshToken.Value, config)
	if err != nil {
		t.Fatalf("Failed to refresh token pair: %v", err)
	}

	if newPair.AccessToken.Value == originalPair.AccessToken.Value {
		t.Error("Expected new access token to be different from original")
	}
	if newPair.RefreshToken.Value == originalPair.RefreshToken.Value {
		t.Error("Expected new refresh token to be different from original")
	}

	claims, err := ValidateAccessToken(newPair.AccessToken.Value, config.PublicKey)
	if err != nil {
		t.Fatalf("Failed to validate new access token: %v", err)
	}
	if claims.UserID != userID {
		t.Errorf("Expected user ID %s in refreshed token, got %s", userID, claims.UserID)
	}
}

func TestStandardClaims(t *testing.T) {
	userID := uuid.New().String()
	issuer := "test-issuer"
	expiresAt := time.Now().Add(1 * time.Hour)

	claims, err := NewStandardClaims(userID, AccessTokenType, issuer, expiresAt)
	if err != nil {
		t.Fatalf("Failed to create standard claims: %v", err)
	}

	if claims.UserID != userID {
		t.Errorf("Expected user ID %s, got %s", userID, claims.UserID)
	}
	if claims.TokenType != AccessTokenType {
		t.Errorf("Expected token type %s, got %s", AccessTokenType, claims.TokenType)
	}
	if claims.TokenID == "" {
		t.Error("Expected token ID (jti) to be set")
	}

	if claims.Issuer != issuer {
		t.Errorf("Expected issuer %s, got %s", issuer, claims.Issuer)
	}
	if claims.Subject != userID {
		t.Errorf("Expected subject %s, got %s", userID, claims.Subject)
	}
	if claims.ID != claims.TokenID {
		t.Errorf("Expected ID and TokenID to match")
	}
}

// Legacy compatibility tests

func TestLegacyUserTokenClaimsWithExpiration(t *testing.T) {
	userID := uuid.New().String()
	customExpiration := time.Now().Add(3 * time.Hour)

	claims := NewUserTokenClaimsWithExpiration(userID, customExpiration)

	claimsUserID, ok := claims.MapClaims["user_id"].(string)
	if !ok {
		t.Error("Expected user_id in claims")
	}
	if claimsUserID != userID {
		t.Errorf("Expected user ID %s, got %s", userID, claimsUserID)
	}

	expUnix, ok := claims.MapClaims["exp"].(int64)
	if !ok {
		t.Error("Expected exp in claims")
	}

	claimsExpiration := time.Unix(expUnix, 0)
	if claimsExpiration.Before(customExpiration.Add(-5*time.Second)) ||
		claimsExpiration.After(customExpiration.Add(5*time.Second)) {
		t.Errorf("Expected expiration around %v, got %v", customExpiration, claimsExpiration)
	}
}

func TestInvalidTokenValidation(t *testing.T) {
	config := createTestTokenConfig(t)

	if _, err := ValidateAccessToken("invalid-token", config.PublicKey); err == nil {
		t.Error("Expected error for invalid token")
	}

	userID := uuid.New().String()
	tokenPair, err := NewTokenPair(userID, config)
	if err != nil {
		t.Fatalf("Failed to create token pair: %v", err)
	}

	wrongKm := &KeyManager{}
	if err := wrongKm.GenerateKeyPair(); err != nil {
		t.Fatalf("Failed to generate wrong key pair: %v", err)
	}

	if _, err = ValidateAccessToken(tokenPair.AccessToken.Value, wrongKm.GetPublicKey()); err == nil {
		t.Error("Expected error for wrong public key")
	}
}

func TestKeyManager(t *testing.T) {
	km := &KeyManager{}

	if km.IsInitialized() {
		t.Error("Expected key manager to be uninitialized")
	}

	if err := km.GenerateKeyPair(); err != nil {
		t.Fatalf("Failed to generate key pair: %v", err)
	}

	if !km.IsInitialized() {
		t.Error("Expected key manager to be initialized")
	}

	if km.GetKeyID() == "" {
		t.Error("Expected key ID to be set")
	}

	if km.GetPublicKey() == nil {
		t.Error("Expected public key to be available")
	}

	if km.GetPrivateKey() == nil {
		t.Error("Expected private key to be available")
	}

	keyListResponse := km.ListSigningKeys()
	if len(keyListResponse.Keys) != 1 {
		t.Errorf("Expected 1 key in signing key list, got %d", len(keyListResponse.Keys))
	}
	key := keyListResponse.Keys[0]
	if key.GetRsa() == nil || len(key.GetRsa().GetModulus()) == 0 || key.GetRsa().GetExponent() == 0 {
		t.Fatal("Expected a complete RSA public key")
	}
	if key.Algorithm != identra_v1_pb.SigningAlgorithm_SIGNING_ALGORITHM_RS256 {
		t.Errorf("Expected algorithm RS256, got %s", key.Algorithm)
	}

	privatePEM, err := km.ExportPrivateKeyPEM()
	if err != nil {
		t.Fatalf("Failed to export private key PEM: %v", err)
	}
	if privatePEM == "" {
		t.Error("Expected private key PEM to be non-empty")
	}

	publicPEM, err := km.ExportPublicKeyPEM()
	if err != nil {
		t.Fatalf("Failed to export public key PEM: %v", err)
	}
	if publicPEM == "" {
		t.Error("Expected public key PEM to be non-empty")
	}

	newKm := &KeyManager{}
	if err := newKm.InitializeFromPEM(privatePEM); err != nil {
		t.Fatalf("Failed to initialize from PEM: %v", err)
	}
	if !newKm.IsInitialized() {
		t.Error("Expected new key manager to be initialized from PEM")
	}
}

func TestKeyRotation(t *testing.T) {
	km := &KeyManager{}

	// Initialize with first key
	if err := km.GenerateKeyPair(); err != nil {
		t.Fatalf("Failed to generate initial key pair: %v", err)
	}

	firstKeyID := km.GetKeyID()
	if firstKeyID == "" {
		t.Error("Expected initial key ID to be set")
	}

	// Verify only one key in signing key list
	keyList := km.ListSigningKeys()
	if len(keyList.Keys) != 1 {
		t.Errorf("Expected 1 key in signing key list, got %d", len(keyList.Keys))
	}
	if keyList.Keys[0].KeyId != firstKeyID {
		t.Errorf("Expected key ID %s, got %s", firstKeyID, keyList.Keys[0].KeyId)
	}

	// Add a new key in PASSIVE state
	secondKeyID, err := km.AddKeyPassive()
	if err != nil {
		t.Fatalf("Failed to add passive key: %v", err)
	}
	if secondKeyID == "" {
		t.Error("Expected second key ID to be set")
	}
	if secondKeyID == firstKeyID {
		t.Error("Expected second key ID to be different from first")
	}

	// Verify both keys are in signing key list
	keyList = km.ListSigningKeys()
	if len(keyList.Keys) != 2 {
		t.Errorf("Expected 2 keys in signing key list after adding passive key, got %d", len(keyList.Keys))
	}

	// Verify active key is still the first one
	if km.GetKeyID() != firstKeyID {
		t.Errorf("Expected active key to still be %s, got %s", firstKeyID, km.GetKeyID())
	}

	// Promote the second key to ACTIVE
	if err := km.PromoteKey(secondKeyID); err != nil {
		t.Fatalf("Failed to promote key: %v", err)
	}

	// Verify active key is now the second one
	if km.GetKeyID() != secondKeyID {
		t.Errorf("Expected active key to be %s after promotion, got %s", secondKeyID, km.GetKeyID())
	}

	// Verify both keys are still in signing key list (old key should be PASSIVE now)
	keyList = km.ListSigningKeys()
	if len(keyList.Keys) != 2 {
		t.Errorf("Expected 2 keys in signing key list after promotion, got %d", len(keyList.Keys))
	}

	// Retire the first key
	if err := km.RetireKey(firstKeyID); err != nil {
		t.Fatalf("Failed to retire key: %v", err)
	}

	// Verify only the second key is in signing key list
	keyList = km.ListSigningKeys()
	if len(keyList.Keys) != 1 {
		t.Errorf("Expected 1 key in signing key list after retiring first key, got %d", len(keyList.Keys))
	}
	if keyList.Keys[0].KeyId != secondKeyID {
		t.Errorf("Expected remaining key to be %s, got %s", secondKeyID, keyList.Keys[0].KeyId)
	}
}

func TestKeyRotationWithTokenValidation(t *testing.T) {
	km := &KeyManager{}

	// Initialize with first key
	if err := km.GenerateKeyPair(); err != nil {
		t.Fatalf("Failed to generate initial key pair: %v", err)
	}

	// Create a token with the first key
	userID := uuid.New().String()
	config := TokenConfig{
		PrivateKey:             km.GetPrivateKey(),
		PublicKey:              km.GetPublicKey(),
		KeyID:                  km.GetKeyID(),
		Issuer:                 "test-issuer",
		AccessTokenExpiration:  15 * time.Minute,
		RefreshTokenExpiration: 7 * 24 * time.Hour,
	}

	tokenPair, err := NewTokenPair(userID, config)
	if err != nil {
		t.Fatalf("Failed to create token pair: %v", err)
	}

	// Verify token can be validated with first key
	claims, err := ValidateAccessToken(tokenPair.AccessToken.Value, km.GetPublicKey())
	if err != nil {
		t.Fatalf("Failed to validate token with first key: %v", err)
	}
	if claims.UserID != userID {
		t.Errorf("Expected user ID %s, got %s", userID, claims.UserID)
	}

	// Add a new key in PASSIVE state
	secondKeyID, err := km.AddKeyPassive()
	if err != nil {
		t.Fatalf("Failed to add passive key: %v", err)
	}

	// Promote the second key to ACTIVE
	if err := km.PromoteKey(secondKeyID); err != nil {
		t.Fatalf("Failed to promote key: %v", err)
	}

	// Token signed with first key should still be valid (key is now PASSIVE)
	// In a real scenario, we would extract kid from JWT header and use it to look up the key
	// For this test, we just verify that both keys are still in signing key list
	keyList := km.ListSigningKeys()
	if len(keyList.Keys) != 2 {
		t.Errorf("Expected 2 keys in signing key list after promotion, got %d", len(keyList.Keys))
	}

	// Create a new token with the second (now active) key
	config.PrivateKey = km.GetPrivateKey()
	config.PublicKey = km.GetPublicKey()
	config.KeyID = km.GetKeyID()

	newTokenPair, err := NewTokenPair(userID, config)
	if err != nil {
		t.Fatalf("Failed to create token pair with second key: %v", err)
	}

	// New token should be valid with second key
	newClaims, err := ValidateAccessToken(newTokenPair.AccessToken.Value, km.GetPublicKey())
	if err != nil {
		t.Fatalf("Failed to validate token with second key: %v", err)
	}
	if newClaims.UserID != userID {
		t.Errorf("Expected user ID %s in new token, got %s", userID, newClaims.UserID)
	}
}

func TestKeyLifecycleErrors(t *testing.T) {
	km := &KeyManager{}

	// Try to promote a non-existent key
	if err := km.PromoteKey("nonexistent"); err == nil {
		t.Error("Expected error when promoting non-existent key")
	}

	// Try to retire a non-existent key
	if err := km.RetireKey("nonexistent"); err == nil {
		t.Error("Expected error when retiring non-existent key")
	}

	// Generate initial key
	if err := km.GenerateKeyPair(); err != nil {
		t.Fatalf("Failed to generate key pair: %v", err)
	}
	activeKeyID := km.GetKeyID()

	// Try to promote an already ACTIVE key
	if err := km.PromoteKey(activeKeyID); err == nil {
		t.Error("Expected error when promoting already ACTIVE key")
	}

	// Try to retire an ACTIVE key
	if err := km.RetireKey(activeKeyID); err == nil {
		t.Error("Expected error when retiring ACTIVE key")
	}

	// Add passive key
	passiveKeyID, err := km.AddKeyPassive()
	if err != nil {
		t.Fatalf("Failed to add passive key: %v", err)
	}

	// Retire passive key should succeed
	if err := km.RetireKey(passiveKeyID); err != nil {
		t.Errorf("Failed to retire passive key: %v", err)
	}
}

func TestListKeys(t *testing.T) {
	km := &KeyManager{}

	// Initially empty
	keys := km.ListKeys()
	if len(keys) != 0 {
		t.Errorf("Expected 0 keys initially, got %d", len(keys))
	}

	// Add first key
	if err := km.GenerateKeyPair(); err != nil {
		t.Fatalf("Failed to generate key pair: %v", err)
	}

	keys = km.ListKeys()
	if len(keys) != 1 {
		t.Errorf("Expected 1 key after generation, got %d", len(keys))
	}
	if keys[0].State != KeyStateActive {
		t.Errorf("Expected first key to be ACTIVE, got %s", keys[0].State)
	}

	// Add passive key
	passiveKeyID, err := km.AddKeyPassive()
	if err != nil {
		t.Fatalf("Failed to add passive key: %v", err)
	}

	keys = km.ListKeys()
	if len(keys) != 2 {
		t.Errorf("Expected 2 keys after adding passive key, got %d", len(keys))
	}

	// Verify states
	activeCount := 0
	passiveCount := 0
	for _, k := range keys {
		switch k.State {
		case KeyStateActive:
			activeCount++
		case KeyStatePassive:
			passiveCount++
		}
	}
	if activeCount != 1 {
		t.Errorf("Expected 1 ACTIVE key, got %d", activeCount)
	}
	if passiveCount != 1 {
		t.Errorf("Expected 1 PASSIVE key, got %d", passiveCount)
	}

	// Promote passive key
	if err := km.PromoteKey(passiveKeyID); err != nil {
		t.Fatalf("Failed to promote key: %v", err)
	}

	keys = km.ListKeys()
	activeCount = 0
	passiveCount = 0
	for _, k := range keys {
		switch k.State {
		case KeyStateActive:
			activeCount++
		case KeyStatePassive:
			passiveCount++
		}
	}
	if activeCount != 1 {
		t.Errorf("Expected 1 ACTIVE key after promotion, got %d", activeCount)
	}
	if passiveCount != 1 {
		t.Errorf("Expected 1 PASSIVE key after promotion, got %d", passiveCount)
	}
}

func TestMultipleActiveKeyPrevention(t *testing.T) {
	km := &KeyManager{}

	// Generate first key
	if err := km.GenerateKeyPair(); err != nil {
		t.Fatalf("Failed to generate first key: %v", err)
	}
	firstKeyID := km.GetKeyID()

	// Generate second key - should demote first
	if err := km.GenerateKeyPair(); err != nil {
		t.Fatalf("Failed to generate second key: %v", err)
	}
	secondKeyID := km.GetKeyID()

	// Verify only one ACTIVE key
	keys := km.ListKeys()
	activeCount := 0
	for _, k := range keys {
		if k.State == KeyStateActive {
			activeCount++
			if k.KeyID != secondKeyID {
				t.Errorf("Expected ACTIVE key to be %s, got %s", secondKeyID, k.KeyID)
			}
		}
	}
	if activeCount != 1 {
		t.Errorf("Expected exactly 1 ACTIVE key, got %d", activeCount)
	}

	// Verify first key was demoted to PASSIVE
	found := false
	for _, k := range keys {
		if k.KeyID == firstKeyID {
			found = true
			if k.State != KeyStatePassive {
				t.Errorf("Expected first key to be PASSIVE, got %s", k.State)
			}
		}
	}
	if !found {
		t.Error("First key not found in key ring")
	}
}

func TestInitializeFromPEMWithExistingKey(t *testing.T) {
	km := &KeyManager{}

	// Generate initial key
	if err := km.GenerateKeyPair(); err != nil {
		t.Fatalf("Failed to generate initial key: %v", err)
	}
	firstKeyID := km.GetKeyID()

	// Export and re-import (simulating loading from config)
	pem, err := km.ExportPrivateKeyPEM()
	if err != nil {
		t.Fatalf("Failed to export PEM: %v", err)
	}

	// Create a new key and then initialize from PEM
	newKm := &KeyManager{}
	if err := newKm.GenerateKeyPair(); err != nil {
		t.Fatalf("Failed to generate key in new manager: %v", err)
	}
	tempKeyID := newKm.GetKeyID()

	// Initialize from PEM - should demote the temp key
	if err := newKm.InitializeFromPEM(pem); err != nil {
		t.Fatalf("Failed to initialize from PEM: %v", err)
	}

	// Verify the PEM key is now ACTIVE
	if newKm.GetKeyID() != firstKeyID {
		t.Errorf("Expected ACTIVE key to be from PEM (%s), got %s", firstKeyID, newKm.GetKeyID())
	}

	// Verify temp key was demoted
	keys := newKm.ListKeys()
	activeCount := 0
	for _, k := range keys {
		if k.State == KeyStateActive {
			activeCount++
		}
		if k.KeyID == tempKeyID && k.State != KeyStatePassive {
			t.Errorf("Expected temp key to be PASSIVE, got %s", k.State)
		}
	}
	if activeCount != 1 {
		t.Errorf("Expected exactly 1 ACTIVE key after PEM init, got %d", activeCount)
	}
}

func TestSigningKeysDeterministicOrder(t *testing.T) {
	km := &KeyManager{}

	// Add multiple keys in random order
	if err := km.GenerateKeyPair(); err != nil {
		t.Fatalf("Failed to generate first key: %v", err)
	}

	keyID2, err := km.AddKeyPassive()
	if err != nil {
		t.Fatalf("Failed to add second passive key: %v", err)
	}

	keyID3, err := km.AddKeyPassive()
	if err != nil {
		t.Fatalf("Failed to add third passive key: %v", err)
	}

	// Get signing key list multiple times and verify order is consistent
	keyList1 := km.ListSigningKeys()
	keyList2 := km.ListSigningKeys()
	keyList3 := km.ListSigningKeys()

	// Verify all responses have the same number of keys
	if len(keyList1.Keys) != len(keyList2.Keys) || len(keyList1.Keys) != len(keyList3.Keys) {
		t.Errorf("Inconsistent number of keys across signing key list responses")
	}

	// Verify the order is the same in all responses
	for i := range keyList1.Keys {
		if keyList1.Keys[i].KeyId != keyList2.Keys[i].KeyId {
			t.Errorf("Key order differs between responses 1 and 2 at index %d: %s vs %s",
				i, keyList1.Keys[i].KeyId, keyList2.Keys[i].KeyId)
		}
		if keyList1.Keys[i].KeyId != keyList3.Keys[i].KeyId {
			t.Errorf("Key order differs between responses 1 and 3 at index %d: %s vs %s",
				i, keyList1.Keys[i].KeyId, keyList3.Keys[i].KeyId)
		}
	}

	// Verify keys are sorted (alphanumeric order)
	for i := 1; i < len(keyList1.Keys); i++ {
		if keyList1.Keys[i-1].KeyId > keyList1.Keys[i].KeyId {
			t.Errorf("Keys are not sorted: %s > %s", keyList1.Keys[i-1].KeyId, keyList1.Keys[i].KeyId)
		}
	}

	// Clean up - retire the passive keys
	if err := km.RetireKey(keyID2); err != nil {
		t.Fatalf("Failed to retire key: %v", err)
	}
	if err := km.RetireKey(keyID3); err != nil {
		t.Fatalf("Failed to retire key: %v", err)
	}
}
