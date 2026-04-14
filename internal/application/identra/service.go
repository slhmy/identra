package identra

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"sort"
	"strings"
	"time"

	"github.com/poly-workshop/identra/internal/infrastructure/cache/redis"
	"github.com/poly-workshop/identra/internal/infrastructure/notification/smtp"
	"github.com/poly-workshop/identra/internal/infrastructure/persistence/gorm"
	identra_v1_pb "github.com/poly-workshop/identra/gen/go/identra/v1"
	"github.com/poly-workshop/identra/internal/domain"
	"github.com/poly-workshop/identra/internal/infrastructure/cache"
	"github.com/poly-workshop/identra/internal/infrastructure/oauth"
	"github.com/poly-workshop/identra/internal/infrastructure/persistence"
	"github.com/poly-workshop/identra/internal/infrastructure/security"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/github"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

var supportedProviders = map[string]struct{}{
	"github": {},
}

// Service implements identra.v1.IdentraService.
type Service struct {
	identra_v1_pb.UnimplementedIdentraServiceServer

	emailCodeStore           cache.EmailCodeStore
	oauthStateStore          oauth.StateStore
	userStore                domain.UserStore
	userStoreCleanup         func(context.Context) error
	keyManager               *security.KeyManager
	tokenCfg                 security.TokenConfig
	githubOAuthConfig        *oauth2.Config
	oauthFetchEmailIfMissing bool
	mailer                   *smtp.Mailer
}

func NewService(ctx context.Context, cfg Config) (*Service, error) {
	mailerCfg := cfg.SmtpMailer
	var mailer *smtp.Mailer

	if strings.TrimSpace(mailerCfg.Host) != "" {
		if err := validateMailerConfig(mailerCfg); err != nil {
			return nil, fmt.Errorf("invalid mailer config: %w", err)
		}

		mailer = smtp.NewMailer(mailerCfg)
	}

	km := security.GetKeyManager()
	if cfg.RSAPrivateKey != "" {
		if err := km.InitializeFromPEM(cfg.RSAPrivateKey); err != nil {
			return nil, fmt.Errorf("failed to load RSA private key: %w", err)
		}
	}
	if !km.IsInitialized() {
		if err := km.GenerateKeyPair(); err != nil {
			return nil, fmt.Errorf("failed to generate RSA key pair: %w", err)
		}
	}

	tokenCfg := security.TokenConfig{
		PrivateKey:             km.GetPrivateKey(),
		PublicKey:              km.GetPublicKey(),
		KeyID:                  km.GetKeyID(),
		Issuer:                 cfg.TokenIssuer,
		AccessTokenExpiration:  cfg.AccessTokenExpirationDuration,
		RefreshTokenExpiration: cfg.RefreshTokenExpirationDuration,
	}
	if tokenCfg.PrivateKey == nil || tokenCfg.PublicKey == nil {
		return nil, errors.New("token keys are not initialized")
	}

	stateTTL := cfg.OAuthStateExpirationDuration
	if stateTTL <= 0 {
		stateTTL = DefaultOAuthStateExpiration
	}

	userStore, cleanup, storeErr := buildUserStore(ctx, cfg)
	if storeErr != nil {
		return nil, storeErr
	}

	githubCfg := &oauth2.Config{
		ClientID:     cfg.GithubClientID,
		ClientSecret: cfg.GithubClientSecret,
		Scopes:       []string{"read:user", "user:email"},
		Endpoint:     github.Endpoint,
	}

	emailStore, storeErr := cache.NewRedisEmailCodeStore(10*time.Minute, redis.NewRDB(*cfg.RedisClient))
	if storeErr != nil {
		return nil, fmt.Errorf("failed to initialize email code store: %w", storeErr)
	}

	return &Service{
		userStore:                userStore,
		keyManager:               km,
		tokenCfg:                 tokenCfg,
		oauthStateStore:          oauth.NewInMemoryStateStore(stateTTL),
		emailCodeStore:           emailStore,
		githubOAuthConfig:        githubCfg,
		oauthFetchEmailIfMissing: cfg.OAuthFetchEmailIfMissing,
		mailer:                   mailer,
		userStoreCleanup:         cleanup,
	}, nil
}

// Close releases resources owned by the service.
func (s *Service) Close(ctx context.Context) error {
	if s.userStoreCleanup != nil {
		return s.userStoreCleanup(ctx)
	}
	return nil
}

func (s *Service) GetJWKS(ctx context.Context, _ *identra_v1_pb.GetJWKSRequest) (*identra_v1_pb.GetJWKSResponse, error) {
	response := s.keyManager.GetJWKS()

	// Generate ETag based on hash of key IDs in the response
	// This allows clients to efficiently check if keys have changed
	etag := generateJWKSETag(response)

	// Set HTTP cache headers via gRPC metadata
	// Cache-Control: public, max-age=3600 (1 hour)
	// This allows clients to cache the JWKS and reduces load on the server
	md := metadata.Pairs(
		"Cache-Control", "public, max-age=3600",
		"ETag", etag,
	)
	if err := grpc.SetHeader(ctx, md); err != nil {
		// Log error but don't fail the request
		slog.Warn("failed to set JWKS cache headers", "error", err)
	}

	return response, nil
}

// generateJWKSETag creates an ETag based on the key IDs in the JWKS response.
// This allows clients to efficiently check if the key set has changed.
func generateJWKSETag(jwks *identra_v1_pb.GetJWKSResponse) string {
	if jwks == nil || len(jwks.Keys) == 0 {
		return `"empty"`
	}

	// Join all key IDs with a delimiter to avoid ambiguous concatenations, then hash them
	keyIDs := make([]string, 0, len(jwks.Keys))
	for _, key := range jwks.Keys {
		keyIDs = append(keyIDs, key.Kid)
	}

	hash := sha256.Sum256([]byte(strings.Join(keyIDs, ",")))
	// Use the full 32 bytes (256 bits) of the SHA-256 hash to minimize collision risk
	// Quoted per HTTP ETag specification (RFC 7232)
	return fmt.Sprintf(`"%x"`, hash[:])
}

func (s *Service) ListOAuthProviders(
	_ context.Context,
	_ *identra_v1_pb.ListOAuthProvidersRequest,
) (*identra_v1_pb.ListOAuthProvidersResponse, error) {
	names := make([]string, 0, len(supportedProviders))
	for name := range supportedProviders {
		names = append(names, name)
	}
	sort.Strings(names)

	providers := make([]*identra_v1_pb.OAuthProviderStatus, 0, len(names))
	for _, name := range names {
		ps := &identra_v1_pb.OAuthProviderStatus{Name: name}

		switch name {
		case "github":
			if s.githubOAuthConfig.ClientID == "" {
				reason := "missing_client_id"
				ps.Reason = &reason
			} else if s.githubOAuthConfig.ClientSecret == "" {
				reason := "missing_client_secret"
				ps.Reason = &reason
			} else {
				ps.Enabled = true
			}
		}

		providers = append(providers, ps)
	}

	return &identra_v1_pb.ListOAuthProvidersResponse{Providers: providers}, nil
}

func (s *Service) GetOAuthAuthorizationURL(
	ctx context.Context,
	req *identra_v1_pb.GetOAuthAuthorizationURLRequest,
) (*identra_v1_pb.GetOAuthAuthorizationURLResponse, error) {
	provider := strings.ToLower(strings.TrimSpace(req.GetProvider()))
	if provider == "" {
		return nil, status.Error(codes.InvalidArgument, "provider is required")
	}
	if _, ok := supportedProviders[provider]; !ok {
		return nil, status.Errorf(codes.InvalidArgument, "provider %s not supported", provider)
	}

	redirectURL := strings.TrimSpace(req.GetRedirectUrl())
	if redirectURL == "" {
		return nil, status.Error(codes.InvalidArgument, "redirect URL is required")
	}
	if s.githubOAuthConfig.ClientID == "" || s.githubOAuthConfig.ClientSecret == "" {
		return nil, status.Error(codes.FailedPrecondition, "GitHub OAuth is not configured")
	}

	oauthCfg := s.oauthConfigForRedirect(redirectURL)
	state, err := generateOAuthState()
	if err != nil {
		slog.ErrorContext(ctx, "failed to generate oauth state", "error", err)
		return nil, status.Error(codes.Internal, "failed to generate oauth state")
	}
	s.oauthStateStore.Add(state, provider, redirectURL)

	authURL := oauthCfg.AuthCodeURL(state, oauth2.AccessTypeOffline)
	return &identra_v1_pb.GetOAuthAuthorizationURLResponse{Url: authURL, State: state}, nil
}

func (s *Service) LoginByOAuth(
	ctx context.Context,
	req *identra_v1_pb.LoginByOAuthRequest,
) (*identra_v1_pb.LoginByOAuthResponse, error) {
	if strings.TrimSpace(req.GetCode()) == "" {
		return nil, status.Error(codes.InvalidArgument, "code is required")
	}
	if strings.TrimSpace(req.GetState()) == "" {
		return nil, status.Error(codes.InvalidArgument, "state is required")
	}

	stateData, ok := s.oauthStateStore.Consume(req.GetState())
	if !ok {
		return nil, status.Error(codes.InvalidArgument, "invalid or expired state")
	}

	oauthCfg := s.oauthConfigForRedirect(stateData.RedirectURL)
	token, err := oauthCfg.Exchange(ctx, req.GetCode())
	if err != nil {
		slog.ErrorContext(ctx, "oauth code exchange failed", "error", err)
		return nil, status.Error(codes.Unauthenticated, "failed to exchange authorization code")
	}

	userProvider, err := GetUserProvider(stateData.Provider)
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to get user provider")
	}

	userInfo, err := userProvider.GetUserInfo(ctx, token.AccessToken)
	if err != nil {
		slog.ErrorContext(ctx, "failed to fetch user info", "error", err)
		return nil, status.Error(codes.Unauthenticated, "failed to fetch user info")
	}
	s.maybeFillOAuthEmail(ctx, userProvider, token.AccessToken, &userInfo)

	authUser, err := s.ensureOAuthUser(ctx, userInfo)
	if err != nil {
		return nil, err
	}
	s.recordLogin(ctx, authUser)

	tokenPair, err := security.NewTokenPair(authUser.ID, s.tokenCfg)
	if err != nil {
		slog.ErrorContext(ctx, "failed to create token pair", "error", err)
		return nil, status.Error(codes.Internal, "failed to create token pair")
	}

	return &identra_v1_pb.LoginByOAuthResponse{
		Token:     tokenPair,
		Username:  userInfo.Username,
		AvatarUrl: userInfo.AvatarURL,
		Email:     userInfo.Email,
	}, nil
}

func (s *Service) BindUserByOAuth(
	ctx context.Context,
	req *identra_v1_pb.BindUserByOAuthRequest,
) (*identra_v1_pb.BindUserByOAuthResponse, error) {
	accessToken := strings.TrimSpace(req.GetAccessToken())
	if accessToken == "" {
		return nil, status.Error(codes.InvalidArgument, "access token is required")
	}
	if strings.TrimSpace(req.GetCode()) == "" {
		return nil, status.Error(codes.InvalidArgument, "code is required")
	}
	if strings.TrimSpace(req.GetState()) == "" {
		return nil, status.Error(codes.InvalidArgument, "state is required")
	}

	claims, err := security.ValidateAccessToken(accessToken, s.tokenCfg.PublicKey)
	if err != nil {
		slog.WarnContext(ctx, "invalid access token for bind", "error", err)
		return nil, status.Error(codes.Unauthenticated, "invalid access token")
	}

	bindingUser, err := s.userStore.GetByID(ctx, claims.UserID)
	switch {
	case err == nil:
	case errors.Is(err, domain.ErrNotFound):
		return nil, status.Error(codes.NotFound, "user not found")
	default:
		return nil, status.Error(codes.Internal, "failed to fetch user")
	}

	stateData, ok := s.oauthStateStore.Consume(req.GetState())
	if !ok {
		return nil, status.Error(codes.InvalidArgument, "invalid or expired state")
	}

	oauthCfg := s.oauthConfigForRedirect(stateData.RedirectURL)
	token, err := oauthCfg.Exchange(ctx, req.GetCode())
	if err != nil {
		slog.ErrorContext(ctx, "oauth code exchange failed (bind)", "error", err)
		return nil, status.Error(codes.Unauthenticated, "failed to exchange authorization code")
	}

	userProvider, err := GetUserProvider(stateData.Provider)
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to get user provider")
	}

	userInfo, err := userProvider.GetUserInfo(ctx, token.AccessToken)
	if err != nil {
		slog.ErrorContext(ctx, "failed to fetch user info (bind)", "error", err)
		return nil, status.Error(codes.Unauthenticated, "failed to fetch user info")
	}
	s.maybeFillOAuthEmail(ctx, userProvider, token.AccessToken, &userInfo)

	providerUser, err := s.userStore.GetByGithubID(ctx, userInfo.ID)
	switch {
	case err == nil && providerUser.ID != bindingUser.ID:
		return nil, status.Error(codes.AlreadyExists, "oauth account already linked to another user")
	case err == nil:
		// Already linked to this user; continue to refresh token pair and email if needed.
	case errors.Is(err, domain.ErrNotFound):
		// Not linked yet.
	default:
		return nil, status.Error(codes.Internal, "failed to check existing oauth link")
	}

	if bindingUser.GithubID == nil || *bindingUser.GithubID == "" {
		bindingUser.GithubID = &userInfo.ID
	} else if *bindingUser.GithubID != userInfo.ID {
		return nil, status.Error(codes.FailedPrecondition, "user already linked to another oauth account")
	}

	if _, err := s.updateEmailIfNeeded(ctx, bindingUser, userInfo.Email); err != nil {
		return nil, err
	}

	if err := s.userStore.Update(ctx, bindingUser); err != nil {
		return nil, status.Error(codes.Internal, "failed to link oauth account")
	}

	s.recordLogin(ctx, bindingUser)
	tokenPair, err := security.NewTokenPair(bindingUser.ID, s.tokenCfg)
	if err != nil {
		slog.ErrorContext(ctx, "failed to create token pair (bind)", "error", err)
		return nil, status.Error(codes.Internal, "failed to create token pair")
	}

	return &identra_v1_pb.BindUserByOAuthResponse{
		Token:     tokenPair,
		Username:  userInfo.Username,
		AvatarUrl: userInfo.AvatarURL,
	}, nil
}

func (s *Service) SendLoginEmailCode(
	ctx context.Context,
	req *identra_v1_pb.SendLoginEmailCodeRequest,
) (*identra_v1_pb.SendLoginEmailCodeResponse, error) {
	if s.mailer == nil {
		return nil, status.Error(codes.FailedPrecondition, "mail service is disabled")
	}

	email := strings.TrimSpace(req.GetEmail())
	if email == "" {
		return nil, status.Error(codes.InvalidArgument, "email is required")
	}

	code, err := generateEmailCode()
	if err != nil {
		slog.ErrorContext(ctx, "failed to generate email code", "error", err)
		return nil, status.Error(codes.Internal, "failed to generate code")
	}

	const expiryMinutes = 10
	if err := s.emailCodeStore.Set(ctx, email, code); err != nil {
		slog.ErrorContext(ctx, "failed to store verification code", "error", err)
		return nil, status.Error(codes.Internal, "failed to store verification code")
	}

	if err := s.sendVerificationCode(email, code, expiryMinutes, req.GetUseHtml()); err != nil {
		slog.ErrorContext(ctx, "failed to send verification email", "error", err)
		return nil, status.Error(codes.Internal, "failed to send verification email")
	}

	return &identra_v1_pb.SendLoginEmailCodeResponse{}, nil
}

func (s *Service) sendVerificationCode(to string, code string, expiryMinutes int, useHTML bool) error {
	subject := "Your Verification Code"
	if useHTML {
		htmlBody := fmt.Sprintf(`
<!DOCTYPE html>
<html>
<head>
	<meta charset="UTF-8">
	<style>
		body { font-family: Arial, sans-serif; line-height: 1.6; color: #333; }
		.container { max-width: 600px; margin: 0 auto; padding: 20px; }
		.header { background-color: #4CAF50; color: white; padding: 20px; text-align: center; }
		.content { background-color: #f9f9f9; padding: 30px; border-radius: 5px; margin-top: 20px; }
		.code { font-size: 32px; font-weight: bold; color: #4CAF50; text-align: center; letter-spacing: 5px; padding: 20px; background-color: #fff; border-radius: 5px; margin: 20px 0; }
		.footer { text-align: center; margin-top: 20px; color: #666; font-size: 12px; }
	</style>
</head>
<body>
	<div class="container">
		<div class="header">
			<h1>Verification Code</h1>
		</div>
		<div class="content">
			<p>Hello,</p>
			<p>You have requested a verification code. Please use the code below to complete your verification:</p>
			<div class="code">%s</div>
			<p>This code will expire in <strong>%d minutes</strong>.</p>
			<p>If you did not request this code, please ignore this email.</p>
		</div>
		<div class="footer">
			<p>This is an automated message, please do not reply.</p>
		</div>
	</div>
</body>
</html>
`, code, expiryMinutes)

		return s.mailer.SendEmail(smtp.Message{
			ToEmails: []string{to},
			Subject:  subject,
			Body:     htmlBody,
			IsHTML:   true,
		})
	}

	body := fmt.Sprintf("Your verification code is: %s (valid for %d minutes)", code, expiryMinutes)
	return s.mailer.SendEmail(smtp.Message{
		ToEmails: []string{to},
		Subject:  subject,
		Body:     body,
		IsHTML:   false,
	})
}

func validateMailerConfig(cfg smtp.Config) error {
	if strings.TrimSpace(cfg.Host) == "" {
		return nil
	}

	switch {
	case cfg.Port == 0:
		return errors.New("smtp port is required")
	case strings.TrimSpace(cfg.Username) == "":
		return errors.New("smtp username is required")
	case strings.TrimSpace(cfg.Password) == "":
		return errors.New("smtp password is required")
	case strings.TrimSpace(cfg.FromEmail) == "":
		return errors.New("smtp from email is required")
	default:
		return nil
	}
}

func (s *Service) LoginByEmailCode(
	ctx context.Context,
	req *identra_v1_pb.LoginByEmailCodeRequest,
) (*identra_v1_pb.LoginByEmailCodeResponse, error) {
	email := strings.TrimSpace(req.GetEmail())
	code := strings.TrimSpace(req.GetCode())
	if email == "" || code == "" {
		return nil, status.Error(codes.InvalidArgument, "email and code are required")
	}

	ok, err := s.emailCodeStore.Consume(ctx, email, code)
	if err != nil {
		slog.ErrorContext(ctx, "failed to validate verification code", "error", err)
		return nil, status.Error(codes.Internal, "failed to validate code")
	}
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "invalid or expired code")
	}

	usr, err := s.userStore.GetByEmail(ctx, email)
	switch {
	case err == nil:
	case errors.Is(err, domain.ErrNotFound):
		usr = &domain.UserModel{Email: email}
		if createErr := s.userStore.Create(ctx, usr); createErr != nil {
			return nil, status.Error(codes.Internal, "failed to create user")
		}
	default:
		return nil, status.Error(codes.Internal, "failed to fetch user")
	}

	s.recordLogin(ctx, usr)
	tokenPair, err := security.NewTokenPair(usr.ID, s.tokenCfg)
	if err != nil {
		slog.ErrorContext(ctx, "failed to create token pair (email code)", "error", err)
		return nil, status.Error(codes.Internal, "failed to create token pair")
	}

	return &identra_v1_pb.LoginByEmailCodeResponse{Token: tokenPair}, nil
}

func (s *Service) LoginByPassword(
	ctx context.Context,
	req *identra_v1_pb.LoginByPasswordRequest,
) (*identra_v1_pb.LoginByPasswordResponse, error) {
	email := strings.TrimSpace(req.GetEmail())
	password := req.GetPassword()
	if email == "" || password == "" {
		return nil, status.Error(codes.InvalidArgument, "email and password are required")
	}

	existing, err := s.userStore.GetByEmail(ctx, email)
	switch {
	case err == nil:
		if existing.HashedPassword == nil {
			hash, hashErr := security.HashPassword(password)
			if hashErr != nil {
				slog.ErrorContext(ctx, "failed to hash password", "error", hashErr)
				return nil, status.Error(codes.Internal, "failed to process password")
			}
			existing.HashedPassword = &hash
			if updateErr := s.userStore.Update(ctx, existing); updateErr != nil {
				return nil, status.Error(codes.Internal, "failed to persist password")
			}
		} else {
			valid, verifyErr := security.VerifyPassword(password, *existing.HashedPassword)
			if verifyErr != nil {
				slog.ErrorContext(ctx, "password verification failed", "error", verifyErr)
				return nil, status.Error(codes.Internal, "failed to verify password")
			}
			if !valid {
				return nil, status.Error(codes.Unauthenticated, "invalid credentials")
			}
		}

	case errors.Is(err, domain.ErrNotFound):
		hash, hashErr := security.HashPassword(password)
		if hashErr != nil {
			slog.ErrorContext(ctx, "failed to hash password", "error", hashErr)
			return nil, status.Error(codes.Internal, "failed to process password")
		}
		existing = &domain.UserModel{Email: email, HashedPassword: &hash}
		if createErr := s.userStore.Create(ctx, existing); createErr != nil {
			return nil, status.Error(codes.Internal, "failed to create user")
		}
	default:
		return nil, status.Error(codes.Internal, "failed to fetch user")
	}

	s.recordLogin(ctx, existing)
	tokenPair, err := security.NewTokenPair(existing.ID, s.tokenCfg)
	if err != nil {
		slog.ErrorContext(ctx, "failed to create token pair", "error", err)
		return nil, status.Error(codes.Internal, "failed to create token pair")
	}

	return &identra_v1_pb.LoginByPasswordResponse{Token: tokenPair}, nil
}

func (s *Service) RefreshToken(
	ctx context.Context,
	req *identra_v1_pb.RefreshTokenRequest,
) (*identra_v1_pb.RefreshTokenResponse, error) {
	if strings.TrimSpace(req.GetRefreshToken()) == "" {
		return nil, status.Error(codes.InvalidArgument, "refresh token is required")
	}

	tokenPair, err := security.RefreshTokenPair(req.GetRefreshToken(), s.tokenCfg)
	if err != nil {
		slog.ErrorContext(ctx, "failed to refresh token pair", "error", err)
		return nil, status.Error(codes.Unauthenticated, "invalid refresh token")
	}

	return &identra_v1_pb.RefreshTokenResponse{Token: tokenPair}, nil
}

func (s *Service) GetCurrentUserLoginInfo(
	ctx context.Context,
	req *identra_v1_pb.GetCurrentUserLoginInfoRequest,
) (*identra_v1_pb.GetCurrentUserLoginInfoResponse, error) {
	accessToken := strings.TrimSpace(req.GetAccessToken())
	if accessToken == "" {
		return nil, status.Error(codes.InvalidArgument, "access token is required")
	}

	claims, err := security.ValidateAccessToken(accessToken, s.tokenCfg.PublicKey)
	if err != nil {
		slog.WarnContext(ctx, "invalid access token for get login info", "error", err)
		return nil, status.Error(codes.Unauthenticated, "invalid access token")
	}

	usr, err := s.userStore.GetByID(ctx, claims.UserID)
	switch {
	case err == nil:
	case errors.Is(err, domain.ErrNotFound):
		return nil, status.Error(codes.NotFound, "user not found")
	default:
		return nil, status.Error(codes.Internal, "failed to fetch user")
	}

	resp := &identra_v1_pb.GetCurrentUserLoginInfoResponse{
		UserId:          usr.ID,
		Email:           usr.Email,
		PasswordEnabled: usr.HashedPassword != nil && strings.TrimSpace(*usr.HashedPassword) != "",
	}

	if usr.GithubID != nil && strings.TrimSpace(*usr.GithubID) != "" {
		resp.GithubId = usr.GithubID
		resp.OauthConnections = append(resp.OauthConnections, &identra_v1_pb.OAuthConnection{
			Provider:       "github",
			ProviderUserId: *usr.GithubID,
		})
	}

	return resp, nil
}

func (s *Service) ensureOAuthUser(ctx context.Context, info UserInfo) (*domain.UserModel, error) {
	if info.ID == "" {
		return nil, status.Error(codes.Internal, "provider user id is empty")
	}

	existing, err := s.userStore.GetByGithubID(ctx, info.ID)
	switch {
	case err == nil:
		return s.updateEmailIfNeeded(ctx, existing, info.Email)
	case errors.Is(err, domain.ErrNotFound):
		// If user is not linked by provider id, try to link by email if available.
		// (Email may be missing for some OAuth users, depending on provider settings / privacy.)
		if strings.TrimSpace(info.Email) != "" {
			byEmail, emailErr := s.userStore.GetByEmail(ctx, info.Email)
			switch {
			case emailErr == nil:
				byEmail.GithubID = &info.ID
				if updateErr := s.userStore.Update(ctx, byEmail); updateErr != nil {
					return nil, status.Error(codes.Internal, "failed to link github account")
				}
				return byEmail, nil
			case errors.Is(emailErr, domain.ErrNotFound):
				// Email is provided but no existing user, create a new one
				userModel := &domain.UserModel{Email: info.Email, GithubID: &info.ID}
				if createErr := s.userStore.Create(ctx, userModel); createErr != nil {
					return nil, status.Error(codes.Internal, "failed to create user")
				}
				return userModel, nil
			default:
				return nil, status.Error(codes.Internal, "failed to fetch user by email")
			}
		}
		// No email provided from OAuth provider, create user with GitHub ID only.
		// Email is intentionally left as empty string (default value).
		userModel := &domain.UserModel{GithubID: &info.ID}
		if createErr := s.userStore.Create(ctx, userModel); createErr != nil {
			return nil, status.Error(codes.Internal, "failed to create user")
		}
		return userModel, nil
	default:
		return nil, status.Error(codes.Internal, "failed to fetch user by provider id")
	}
}

func (s *Service) updateEmailIfNeeded(ctx context.Context, usr *domain.UserModel, email string) (*domain.UserModel, error) {
	if usr.Email == email || email == "" {
		return usr, nil
	}

	usr.Email = email
	if err := s.userStore.Update(ctx, usr); err != nil {
		return nil, status.Error(codes.Internal, "failed to update user email")
	}
	return usr, nil
}

func (s *Service) recordLogin(ctx context.Context, usr *domain.UserModel) {
	now := time.Now()
	usr.LastLoginAt = &now
	if err := s.userStore.Update(ctx, usr); err != nil {
		slog.WarnContext(ctx, "failed to record last login", "error", err, "user_id", usr.ID)
	}
}

func (s *Service) maybeFillOAuthEmail(ctx context.Context, provider UserInfoProvider, accessToken string, info *UserInfo) {
	if info == nil || info.Email != "" || !s.oauthFetchEmailIfMissing {
		return
	}
	if strings.TrimSpace(accessToken) == "" {
		return
	}

	emailProvider, ok := provider.(EmailProvider)
	if !ok {
		return
	}

	email, err := emailProvider.GetEmail(ctx, accessToken)
	if err != nil || strings.TrimSpace(email) == "" {
		return
	}
	info.Email = strings.TrimSpace(email)
}

func buildUserStore(ctx context.Context, cfg Config) (domain.UserStore, func(context.Context) error, error) {
	repoType := strings.ToLower(strings.TrimSpace(cfg.PersistenceType))
	switch repoType {
	case "mongo", "mongodb":
		mongoCfg := cfg.MongoClient
		if strings.TrimSpace(mongoCfg.URI) == "" {
			return nil, nil, fmt.Errorf("mongo uri is required when using mongo user repository")
		}
		if strings.TrimSpace(mongoCfg.Database) == "" {
			return nil, nil, fmt.Errorf("mongo database is required when using mongo user repository")
		}

		client, err := mongo.Connect(options.Client().ApplyURI(mongoCfg.URI))
		if err != nil {
			return nil, nil, fmt.Errorf("failed to connect to mongo: %w", err)
		}

		repo, repoErr := persistence.NewMongoUserStore(ctx, client, mongoCfg.Database, "users")
		if repoErr != nil {
			_ = client.Disconnect(ctx)
			return nil, nil, repoErr
		}

		cleanup := func(cleanupCtx context.Context) error {
			return client.Disconnect(cleanupCtx)
		}
		return repo, cleanup, nil
	case "", "gorm", "postgres", "mysql", "sqlite":
		db := gorm.NewDB(*cfg.GORMClient)
		if err := db.AutoMigrate(&domain.UserModel{}); err != nil {
			slog.Error("failed to migrate database", "error", err)
		}
		return persistence.NewGormUserStore(db), func(context.Context) error { return nil }, nil
	default:
		return nil, nil, fmt.Errorf("unsupported user repository type: %s", cfg.PersistenceType)
	}
}

func (s *Service) oauthConfigForRedirect(redirectURL string) *oauth2.Config {
	cfg := *s.githubOAuthConfig
	cfg.RedirectURL = redirectURL
	return &cfg
}

func generateEmailCode() (string, error) {
	num, err := rand.Int(rand.Reader, big.NewInt(1000000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", num.Int64()), nil
}

func generateOAuthState() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
