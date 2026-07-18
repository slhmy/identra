package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/slhmy/identra/internal/config"
	"github.com/slhmy/identra/internal/security"
)

type tokenKeys struct {
	manager     *security.KeyManager
	tokenConfig security.TokenConfig
}

func buildTokenKeys(cfg config.AuthConfig) (tokenKeys, error) {
	km := &security.KeyManager{}
	if privateKeyPEM := strings.TrimSpace(cfg.RSAPrivateKey); privateKeyPEM != "" {
		if err := km.InitializeFromPEM(privateKeyPEM); err != nil {
			return tokenKeys{}, fmt.Errorf("failed to load RSA private key: %w", err)
		}
	} else {
		keyFile := strings.TrimSpace(cfg.RSAPrivateKeyFile)
		if keyFile == "" {
			return tokenKeys{}, errors.New("auth.rsa_private_key_file is required when auth.rsa_private_key is not set")
		}
		if err := initializeKeyManagerFromFile(km, keyFile); err != nil {
			return tokenKeys{}, err
		}
	}

	if !km.IsInitialized() {
		return tokenKeys{}, errors.New("token keys are not initialized")
	}

	tokenCfg := security.TokenConfig{
		PrivateKey:             km.GetPrivateKey(),
		PublicKey:              km.GetPublicKey(),
		KeyID:                  km.GetKeyID(),
		Issuer:                 cfg.Token.Issuer,
		AccessTokenExpiration:  cfg.Token.AccessTokenExpiration,
		RefreshTokenExpiration: cfg.Token.RefreshTokenExpiration,
		ServiceTokenExpiration: cfg.Token.ServiceTokenExpiration,
	}
	if tokenCfg.PrivateKey == nil || tokenCfg.PublicKey == nil {
		return tokenKeys{}, errors.New("token keys are not initialized")
	}

	return tokenKeys{manager: km, tokenConfig: tokenCfg}, nil
}

func initializeKeyManagerFromFile(km *security.KeyManager, keyFile string) error {
	privateKeyPEM, err := os.ReadFile(keyFile)
	if err == nil {
		if err := km.InitializeFromPEM(string(privateKeyPEM)); err != nil {
			return fmt.Errorf("load RSA private key %s: %w", keyFile, err)
		}
		return nil
	}
	if !os.IsNotExist(err) {
		return fmt.Errorf("read RSA private key %s: %w", keyFile, err)
	}

	if err := km.GenerateKeyPair(); err != nil {
		return fmt.Errorf("failed to generate RSA key pair: %w", err)
	}
	generatedPEM, err := km.ExportPrivateKeyPEM()
	if err != nil {
		return fmt.Errorf("export generated RSA private key: %w", err)
	}
	if err := writePrivateKeyFile(keyFile, []byte(generatedPEM)); err != nil {
		return err
	}
	return nil
}

func writePrivateKeyFile(path string, data []byte) (returnErr error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create RSA private key directory %s: %w", dir, err)
	}

	tempFile, err := os.CreateTemp(dir, ".identra-signing-key-*")
	if err != nil {
		return fmt.Errorf("create temporary RSA private key file: %w", err)
	}
	tempPath := tempFile.Name()
	defer func() {
		if err := os.Remove(tempPath); err != nil && !os.IsNotExist(err) && returnErr == nil {
			returnErr = fmt.Errorf("remove temporary RSA private key file: %w", err)
		}
	}()

	if err := tempFile.Chmod(0o600); err != nil {
		_ = tempFile.Close()
		return fmt.Errorf("set RSA private key permissions: %w", err)
	}
	if _, err := tempFile.Write(data); err != nil {
		_ = tempFile.Close()
		return fmt.Errorf("write RSA private key: %w", err)
	}
	if err := tempFile.Sync(); err != nil {
		_ = tempFile.Close()
		return fmt.Errorf("sync RSA private key: %w", err)
	}
	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("close RSA private key: %w", err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("persist RSA private key %s: %w", path, err)
	}
	return nil
}
