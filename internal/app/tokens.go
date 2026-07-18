package app

import (
	"errors"
	"fmt"

	"github.com/slhmy/identra/internal/config"
	"github.com/slhmy/identra/internal/security"
)

type tokenKeys struct {
	manager     *security.KeyManager
	tokenConfig security.TokenConfig
}

func buildTokenKeys(cfg config.AuthConfig) (tokenKeys, error) {
	km := security.GetKeyManager()
	if cfg.RSAPrivateKey != "" {
		if err := km.InitializeFromPEM(cfg.RSAPrivateKey); err != nil {
			return tokenKeys{}, fmt.Errorf("failed to load RSA private key: %w", err)
		}
	}
	if !km.IsInitialized() {
		if err := km.GenerateKeyPair(); err != nil {
			return tokenKeys{}, fmt.Errorf("failed to generate RSA key pair: %w", err)
		}
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
