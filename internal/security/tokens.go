package security

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	identra_v1_pb "github.com/slhmy/identra/gen/go/identra/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// TokenType represents the type of JWT token.
type TokenType string

const (
	AccessTokenType  TokenType = "access"
	RefreshTokenType TokenType = "refresh"
)

// StandardClaims represents JWKS-compliant JWT claims.
type StandardClaims struct {
	jwt.RegisteredClaims
	UserID    string    `json:"uid"`
	TokenType TokenType `json:"typ"`
	TokenID   string    `json:"jti"`
}

// TokenConfig holds configuration for token generation.
type TokenConfig struct {
	PrivateKey             *rsa.PrivateKey
	PublicKey              *rsa.PublicKey
	KeyID                  string
	Issuer                 string
	AccessTokenExpiration  time.Duration
	RefreshTokenExpiration time.Duration
}

// NewStandardClaims creates JWKS-compliant claims for a token.
func NewStandardClaims(
	userID string,
	tokenType TokenType,
	issuer string,
	expiresAt time.Time,
) (*StandardClaims, error) {
	tokenIDBytes := make([]byte, 16)
	if _, err := rand.Read(tokenIDBytes); err != nil {
		return nil, err
	}
	tokenID := hex.EncodeToString(tokenIDBytes)

	now := time.Now()
	return &StandardClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    issuer,
			Subject:   userID,
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ID:        tokenID,
		},
		UserID:    userID,
		TokenType: tokenType,
		TokenID:   tokenID,
	}, nil
}

// GenerateToken creates a signed JWT token from claims using RS256.
func GenerateToken(claims *StandardClaims, privateKey *rsa.PrivateKey, keyID string) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = keyID
	return token.SignedString(privateKey)
}

// NewTokenPair creates a new access/refresh token pair following JWKS standards.
func NewTokenPair(
	userID string,
	config TokenConfig,
) (*identra_v1_pb.TokenPair, error) {
	now := time.Now()
	accessExpiresAt := now.Add(config.AccessTokenExpiration)
	refreshExpiresAt := now.Add(config.RefreshTokenExpiration)

	accessClaims, err := NewStandardClaims(userID, AccessTokenType, config.Issuer, accessExpiresAt)
	if err != nil {
		return nil, err
	}
	accessToken, err := GenerateToken(accessClaims, config.PrivateKey, config.KeyID)
	if err != nil {
		return nil, err
	}

	refreshClaims, err := NewStandardClaims(userID, RefreshTokenType, config.Issuer, refreshExpiresAt)
	if err != nil {
		return nil, err
	}
	refreshToken, err := GenerateToken(refreshClaims, config.PrivateKey, config.KeyID)
	if err != nil {
		return nil, err
	}

	return &identra_v1_pb.TokenPair{
		AccessToken: &identra_v1_pb.Token{
			Value:     accessToken,
			ExpiresAt: timestamppb.New(accessExpiresAt),
		},
		RefreshToken: &identra_v1_pb.Token{
			Value:     refreshToken,
			ExpiresAt: timestamppb.New(refreshExpiresAt),
		},
	}, nil
}

// ValidateToken validates a JWT token and returns the claims.
func ValidateToken(tokenString string, publicKey *rsa.PublicKey) (*StandardClaims, error) {
	token, err := jwt.ParseWithClaims(
		tokenString,
		&StandardClaims{},
		func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
			}
			return publicKey, nil
		},
	)
	if err != nil {
		return nil, err
	}

	if claims, ok := token.Claims.(*StandardClaims); ok && token.Valid {
		return claims, nil
	}

	return nil, jwt.ErrTokenInvalidClaims
}

// ValidateAccessToken validates an access token specifically.
func ValidateAccessToken(tokenString string, publicKey *rsa.PublicKey) (*StandardClaims, error) {
	claims, err := ValidateToken(tokenString, publicKey)
	if err != nil {
		return nil, err
	}

	if claims.TokenType != AccessTokenType {
		return nil, jwt.ErrTokenInvalidClaims
	}

	return claims, nil
}

// ValidateRefreshToken validates a refresh token specifically.
func ValidateRefreshToken(tokenString string, publicKey *rsa.PublicKey) (*StandardClaims, error) {
	claims, err := ValidateToken(tokenString, publicKey)
	if err != nil {
		return nil, err
	}

	if claims.TokenType != RefreshTokenType {
		return nil, jwt.ErrTokenInvalidClaims
	}

	return claims, nil
}

// RefreshTokenPair creates a new token pair using a valid refresh token.
func RefreshTokenPair(
	refreshTokenString string,
	config TokenConfig,
) (*identra_v1_pb.TokenPair, error) {
	claims, err := ValidateRefreshToken(refreshTokenString, config.PublicKey)
	if err != nil {
		return nil, err
	}

	return NewTokenPair(claims.UserID, config)
}

// Deprecated types and functions for backwards compatibility.
// These should be removed after migration is complete.

type UserTokenClaims struct {
	jwt.MapClaims
}

// NewUserTokenClaims creates a new UserTokenClaims with a default 24-hour expiration.
// Deprecated: Use NewTokenPair instead for JWKS-compliant tokens.
func NewUserTokenClaims(userID string) UserTokenClaims {
	return NewUserTokenClaimsWithExpiration(userID, time.Now().Add(24*time.Hour))
}

// NewUserTokenClaimsWithExpiration creates a new UserTokenClaims with a custom expiration time.
// Deprecated: Use NewTokenPair instead for JWKS-compliant tokens.
func NewUserTokenClaimsWithExpiration(
	userID string,
	expiresAt time.Time,
) UserTokenClaims {
	return UserTokenClaims{
		MapClaims: jwt.MapClaims{
			"user_id": userID,
			"exp":     expiresAt.Unix(),
		},
	}
}

func (c UserTokenClaims) GetExpirationTime() (*jwt.NumericDate, error) {
	return c.MapClaims.GetExpirationTime()
}

func (c UserTokenClaims) GetNotBefore() (*jwt.NumericDate, error) {
	return c.MapClaims.GetNotBefore()
}

func (c UserTokenClaims) GetIssuedAt() (*jwt.NumericDate, error) {
	return c.MapClaims.GetIssuedAt()
}

func (c UserTokenClaims) GetAudience() (jwt.ClaimStrings, error) {
	return c.MapClaims.GetAudience()
}

func (c UserTokenClaims) GetIssuer() (string, error) {
	return c.MapClaims.GetIssuer()
}

func (c UserTokenClaims) GetSubject() (string, error) {
	return c.MapClaims.GetSubject()
}

// ValidateUserToken validates a legacy user token.
// Deprecated: Use ValidateAccessToken instead for JWKS-compliant tokens.
func ValidateUserToken(tokenString, secret string) (*UserTokenClaims, error) {
	token, err := jwt.ParseWithClaims(
		tokenString,
		&UserTokenClaims{},
		func(token *jwt.Token) (interface{}, error) {
			return []byte(secret), nil
		},
	)
	if err != nil {
		return nil, err
	}

	if claims, ok := token.Claims.(*UserTokenClaims); ok && token.Valid {
		return claims, nil
	}

	return nil, jwt.ErrTokenInvalidClaims
}
