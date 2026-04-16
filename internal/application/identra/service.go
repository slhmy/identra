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
	"github.com/poly-workshop/identra/internal/infrastructure/mail"
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
	externalIdentityStore    domain.ExternalIdentityStore
	userStoreCleanup         func(context.Context) error
	keyManager               *security.KeyManager
	tokenCfg                 security.TokenConfig
	githubOAuthConfig        *oauth2.Config
	oauthFetchEmailIfMissing bool
	mailer                   mail.Sender

	// loginRateLimiter counts failed login attempts per email address and
	// blocks further attempts after the configured threshold.
	loginRateLimiter cache.RateLimiter
	// sendCodeRateLimiter limits how many email verification codes can be sent
	// to a single address within the configured window.
	sendCodeRateLimiter cache.RateLimiter
}

func NewService(ctx context.Context, cfg Config) (*Service, error) {
	mailerCfg := cfg.SmtpMailer
	var mailer mail.Sender

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

	userStore, externalIdentityStore, cleanup, storeErr := buildStores(ctx, cfg)
	if storeErr != nil {
		return nil, storeErr
	}

	githubCfg := &oauth2.Config{
		ClientID:     cfg.GithubClientID,
		ClientSecret: cfg.GithubClientSecret,
		Scopes:       []string{"read:user", "user:email"},
		Endpoint:     github.Endpoint,
	}

	rdb := redis.NewRDB(*cfg.RedisClient)

	emailStore, storeErr := cache.NewRedisEmailCodeStore(10*time.Minute, rdb)
	if storeErr != nil {
		return nil, fmt.Errorf("failed to initialize email code store: %w", storeErr)
	}

	oauthStore, storeErr := cache.NewRedisOAuthStateStore(stateTTL, rdb)
	if storeErr != nil {
		return nil, fmt.Errorf("failed to initialize oauth state store: %w", storeErr)
	}

	loginMaxAttempts := cfg.LoginMaxAttempts
	if loginMaxAttempts <= 0 {
		loginMaxAttempts = DefaultLoginMaxAttempts
	}
	loginLockoutDuration := cfg.LoginLockoutDuration
	if loginLockoutDuration <= 0 {
		loginLockoutDuration = DefaultLoginLockoutDuration
	}

	loginLimiter, loginLimiterErr := cache.NewRedisRateLimiter(
		rdb,
		"identra:rl:login:",
		loginMaxAttempts,
		loginLockoutDuration,
	)
	if loginLimiterErr != nil {
		return nil, fmt.Errorf("failed to initialize login rate limiter: %w", loginLimiterErr)
	}

	sendCodeMaxAttempts := cfg.SendCodeMaxAttempts
	if sendCodeMaxAttempts <= 0 {
		sendCodeMaxAttempts = DefaultSendCodeMaxAttempts
	}
	sendCodeWindow := cfg.SendCodeWindow
	if sendCodeWindow <= 0 {
		sendCodeWindow = DefaultSendCodeWindow
	}

	sendCodeLimiter, sendCodeLimiterErr := cache.NewRedisRateLimiter(
		rdb,
		"identra:rl:send_code:",
		sendCodeMaxAttempts,
		sendCodeWindow,
	)
	if sendCodeLimiterErr != nil {
		return nil, fmt.Errorf("failed to initialize send-code rate limiter: %w", sendCodeLimiterErr)
	}

	return &Service{
		userStore:                userStore,
		externalIdentityStore:    externalIdentityStore,
		keyManager:               km,
		tokenCfg:                 tokenCfg,
		oauthStateStore:          oauthStore,
		emailCodeStore:           emailStore,
		githubOAuthConfig:        githubCfg,
		oauthFetchEmailIfMissing: cfg.OAuthFetchEmailIfMissing,
		mailer:                   mailer,
		userStoreCleanup:         cleanup,
		loginRateLimiter:         loginLimiter,
		sendCodeRateLimiter:      sendCodeLimiter,
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
	if err := s.oauthStateStore.Add(ctx, state, provider, redirectURL); err != nil {
		slog.ErrorContext(ctx, "failed to store oauth state", "error", err)
		return nil, status.Error(codes.Internal, "failed to store oauth state")
	}

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

	stateData, ok, err := s.oauthStateStore.Consume(ctx, req.GetState())
	if err != nil {
		slog.ErrorContext(ctx, "failed to consume oauth state", "error", err)
		return nil, status.Error(codes.Internal, "failed to validate oauth state")
	}
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

	stateData, ok, err := s.oauthStateStore.Consume(ctx, req.GetState())
	if err != nil {
		slog.ErrorContext(ctx, "failed to consume oauth state", "error", err)
		return nil, status.Error(codes.Internal, "failed to validate oauth state")
	}
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

	providerIdentity, err := s.externalIdentityStore.GetByProviderID(ctx, stateData.Provider, userInfo.ID)
	switch {
	case err == nil && providerIdentity.UserID != bindingUser.ID:
		return nil, status.Error(codes.AlreadyExists, "oauth account already linked to another user")
	case err == nil:
		// Already linked to this user; continue to refresh token pair.
	case errors.Is(err, domain.ErrNotFound):
		// Not linked yet; create the external identity.
		identity := &domain.ExternalIdentityModel{
			UserID:         bindingUser.ID,
			Provider:       stateData.Provider,
			ProviderUserID: userInfo.ID,
		}
		if createErr := s.externalIdentityStore.Create(ctx, identity); createErr != nil {
			if errors.Is(createErr, domain.ErrAlreadyExists) {
				return nil, status.Error(codes.AlreadyExists, "oauth account already linked")
			}
			return nil, status.Error(codes.Internal, "failed to link oauth account")
		}
	default:
		return nil, status.Error(codes.Internal, "failed to check existing oauth link")
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

	if s.sendCodeRateLimiter != nil {
		allowed, rlErr := s.sendCodeRateLimiter.IsAllowed(ctx, email)
		if rlErr != nil {
			slog.ErrorContext(ctx, "send-code rate limiter error", "error", rlErr)
			// fail open — a limiter error must not prevent legitimate users
		} else if !allowed {
			return nil, status.Error(codes.ResourceExhausted, "too many verification code requests, please try again later")
		}
		if rlErr == nil {
			if recordErr := s.sendCodeRateLimiter.Record(ctx, email); recordErr != nil {
				slog.ErrorContext(ctx, "failed to record send-code attempt", "error", recordErr)
			}
		}
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

	if s.loginRateLimiter != nil {
		allowed, rlErr := s.loginRateLimiter.IsAllowed(ctx, email)
		if rlErr != nil {
			slog.ErrorContext(ctx, "login rate limiter error", "error", rlErr)
			// fail open
		} else if !allowed {
			return nil, status.Error(codes.ResourceExhausted, "too many failed attempts, please try again later")
		}
	}

	ok, err := s.emailCodeStore.Consume(ctx, email, code)
	if err != nil {
		slog.ErrorContext(ctx, "failed to validate verification code", "error", err)
		return nil, status.Error(codes.Internal, "failed to validate code")
	}
	if !ok {
		if s.loginRateLimiter != nil {
			if recordErr := s.loginRateLimiter.Record(ctx, email); recordErr != nil {
				slog.ErrorContext(ctx, "failed to record login failure", "error", recordErr)
			}
		}
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

	if s.loginRateLimiter != nil {
		if resetErr := s.loginRateLimiter.Reset(ctx, email); resetErr != nil {
			slog.ErrorContext(ctx, "failed to reset login rate limit", "error", resetErr)
		}
	}

	s.recordLogin(ctx, usr)
	tokenPair, err := security.NewTokenPair(usr.ID, s.tokenCfg)
	if err != nil {
		slog.ErrorContext(ctx, "failed to create token pair (email code)", "error", err)
		return nil, status.Error(codes.Internal, "failed to create token pair")
	}

	return &identra_v1_pb.LoginByEmailCodeResponse{Token: tokenPair}, nil
}

func (s *Service) RegisterByPassword(
	ctx context.Context,
	req *identra_v1_pb.RegisterByPasswordRequest,
) (*identra_v1_pb.RegisterByPasswordResponse, error) {
	email := strings.TrimSpace(req.GetEmail())
	password := req.GetPassword()
	if email == "" || password == "" {
		return nil, status.Error(codes.InvalidArgument, "email and password are required")
	}

	_, err := s.userStore.GetByEmail(ctx, email)
	switch {
	case err == nil:
		return nil, status.Error(codes.AlreadyExists, "user already exists")
	case errors.Is(err, domain.ErrNotFound):
		// expected — proceed to create
	default:
		return nil, status.Error(codes.Internal, "failed to fetch user")
	}

	hash, hashErr := security.HashPassword(password)
	if hashErr != nil {
		slog.ErrorContext(ctx, "failed to hash password", "error", hashErr)
		return nil, status.Error(codes.Internal, "failed to process password")
	}
	usr := &domain.UserModel{Email: email, HashedPassword: &hash}
	if createErr := s.userStore.Create(ctx, usr); createErr != nil {
		if errors.Is(createErr, domain.ErrAlreadyExists) {
			return nil, status.Error(codes.AlreadyExists, "user already exists")
		}
		slog.ErrorContext(ctx, "failed to create user", "error", createErr, "email", email)
		return nil, status.Error(codes.Internal, "failed to create user")
	}

	s.recordLogin(ctx, usr)
	tokenPair, err := security.NewTokenPair(usr.ID, s.tokenCfg)
	if err != nil {
		slog.ErrorContext(ctx, "failed to create token pair (register)", "error", err)
		return nil, status.Error(codes.Internal, "failed to create token pair")
	}

	return &identra_v1_pb.RegisterByPasswordResponse{Token: tokenPair}, nil
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

	if s.loginRateLimiter != nil {
		allowed, rlErr := s.loginRateLimiter.IsAllowed(ctx, email)
		if rlErr != nil {
			slog.ErrorContext(ctx, "login rate limiter error", "error", rlErr)
			// fail open
		} else if !allowed {
			return nil, status.Error(codes.ResourceExhausted, "too many failed attempts, please try again later")
		}
	}

	usr, err := s.userStore.GetByEmail(ctx, email)
	switch {
	case err == nil:
		// user found — verify password below
	case errors.Is(err, domain.ErrNotFound):
		return nil, status.Error(codes.NotFound, "user not found")
	default:
		return nil, status.Error(codes.Internal, "failed to fetch user")
	}

	if usr.HashedPassword == nil {
		return nil, status.Error(codes.FailedPrecondition, "password login not set up for this account")
	}

	valid, verifyErr := security.VerifyPassword(password, *usr.HashedPassword)
	if verifyErr != nil {
		slog.ErrorContext(ctx, "password verification failed", "error", verifyErr)
		return nil, status.Error(codes.Internal, "failed to verify password")
	}
	if !valid {
		if s.loginRateLimiter != nil {
			if recordErr := s.loginRateLimiter.Record(ctx, email); recordErr != nil {
				slog.ErrorContext(ctx, "failed to record login failure", "error", recordErr)
			}
		}
		return nil, status.Error(codes.Unauthenticated, "invalid credentials")
	}

	if s.loginRateLimiter != nil {
		if resetErr := s.loginRateLimiter.Reset(ctx, email); resetErr != nil {
			slog.ErrorContext(ctx, "failed to reset login rate limit", "error", resetErr)
		}
	}

	s.recordLogin(ctx, usr)
	tokenPair, err := security.NewTokenPair(usr.ID, s.tokenCfg)
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

	identities, err := s.externalIdentityStore.GetByUserID(ctx, usr.ID)
	if err != nil {
		slog.WarnContext(ctx, "failed to fetch external identities", "error", err, "user_id", usr.ID)
	} else {
		for _, identity := range identities {
			resp.OauthConnections = append(resp.OauthConnections, &identra_v1_pb.OAuthConnection{
				Provider:       identity.Provider,
				ProviderUserId: identity.ProviderUserID,
			})
			if identity.Provider == "github" {
				resp.GithubId = &identity.ProviderUserID
			}
		}
	}

	return resp, nil
}

func (s *Service) ensureOAuthUser(ctx context.Context, info UserInfo) (*domain.UserModel, error) {
	if info.ID == "" {
		return nil, status.Error(codes.Internal, "provider user id is empty")
	}
	if info.Provider == "" {
		return nil, status.Error(codes.Internal, "provider name is empty")
	}

	existing, err := s.externalIdentityStore.GetByProviderID(ctx, info.Provider, info.ID)
	switch {
	case err == nil:
		// External identity already exists; fetch and return the linked user.
		usr, userErr := s.userStore.GetByID(ctx, existing.UserID)
		if userErr != nil {
			return nil, status.Error(codes.Internal, "failed to fetch user linked to oauth identity")
		}
		return s.updateEmailIfNeeded(ctx, usr, info.Email)
	case errors.Is(err, domain.ErrNotFound):
		// No existing external identity; try to link by email if available.
		if strings.TrimSpace(info.Email) != "" {
			byEmail, emailErr := s.userStore.GetByEmail(ctx, info.Email)
			switch {
			case emailErr == nil:
				// Merge: link the external identity to the existing user.
				identity := &domain.ExternalIdentityModel{
					UserID:         byEmail.ID,
					Provider:       info.Provider,
					ProviderUserID: info.ID,
				}
				if createErr := s.externalIdentityStore.Create(ctx, identity); createErr != nil {
					return nil, status.Error(codes.Internal, "failed to link oauth account")
				}
				return byEmail, nil
			case errors.Is(emailErr, domain.ErrNotFound):
				// Create new user and link external identity.
				userModel := &domain.UserModel{Email: info.Email}
				if createErr := s.userStore.Create(ctx, userModel); createErr != nil {
					return nil, status.Error(codes.Internal, "failed to create user")
				}
				identity := &domain.ExternalIdentityModel{
					UserID:         userModel.ID,
					Provider:       info.Provider,
					ProviderUserID: info.ID,
				}
				if createErr := s.externalIdentityStore.Create(ctx, identity); createErr != nil {
					// Compensate: remove the newly created user to avoid orphaned records.
					if deleteErr := s.userStore.Delete(ctx, userModel.ID); deleteErr != nil {
						slog.ErrorContext(ctx, "failed to clean up orphaned user after identity create failure",
							"error", deleteErr, "user_id", userModel.ID)
					}
					return nil, status.Error(codes.Internal, "failed to create oauth identity")
				}
				return userModel, nil
			default:
				return nil, status.Error(codes.Internal, "failed to fetch user by email")
			}
		}
		// No email provided; create user with external identity only.
		userModel := &domain.UserModel{}
		if createErr := s.userStore.Create(ctx, userModel); createErr != nil {
			return nil, status.Error(codes.Internal, "failed to create user")
		}
		identity := &domain.ExternalIdentityModel{
			UserID:         userModel.ID,
			Provider:       info.Provider,
			ProviderUserID: info.ID,
		}
		if createErr := s.externalIdentityStore.Create(ctx, identity); createErr != nil {
			// Compensate: remove the newly created user to avoid orphaned records.
			if deleteErr := s.userStore.Delete(ctx, userModel.ID); deleteErr != nil {
				slog.ErrorContext(ctx, "failed to clean up orphaned user after identity create failure",
					"error", deleteErr, "user_id", userModel.ID)
			}
			return nil, status.Error(codes.Internal, "failed to create oauth identity")
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

func buildStores(ctx context.Context, cfg Config) (domain.UserStore, domain.ExternalIdentityStore, func(context.Context) error, error) {
	repoType := strings.ToLower(strings.TrimSpace(cfg.PersistenceType))
	switch repoType {
	case "mongo", "mongodb":
		mongoCfg := cfg.MongoClient
		if strings.TrimSpace(mongoCfg.URI) == "" {
			return nil, nil, nil, fmt.Errorf("mongo uri is required when using mongo user repository")
		}
		if strings.TrimSpace(mongoCfg.Database) == "" {
			return nil, nil, nil, fmt.Errorf("mongo database is required when using mongo user repository")
		}

		client, err := mongo.Connect(options.Client().ApplyURI(mongoCfg.URI))
		if err != nil {
			return nil, nil, nil, fmt.Errorf("failed to connect to mongo: %w", err)
		}

		userStore, repoErr := persistence.NewMongoUserStore(ctx, client, mongoCfg.Database, "users")
		if repoErr != nil {
			_ = client.Disconnect(ctx)
			return nil, nil, nil, repoErr
		}

		extStore, extErr := persistence.NewMongoExternalIdentityStore(ctx, client, mongoCfg.Database, "external_identities")
		if extErr != nil {
			_ = client.Disconnect(ctx)
			return nil, nil, nil, extErr
		}

		cleanup := func(cleanupCtx context.Context) error {
			return client.Disconnect(cleanupCtx)
		}
		return userStore, extStore, cleanup, nil
	case "", "gorm", "postgres", "mysql", "sqlite":
		db := gorm.NewDB(*cfg.GORMClient)
		if err := db.AutoMigrate(&domain.UserModel{}, &domain.ExternalIdentityModel{}); err != nil {
			slog.Error("failed to migrate database", "error", err)
		}
		userStore := persistence.NewGormUserStore(db)
		extStore := persistence.NewGormExternalIdentityStore(db)
		return userStore, extStore, func(context.Context) error { return nil }, nil
	default:
		return nil, nil, nil, fmt.Errorf("unsupported user repository type: %s", cfg.PersistenceType)
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
