package identra

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log/slog"
	"sort"
	"strings"

	identra_v1_pb "github.com/slhmy/identra/gen/go/identra/v1"
	"github.com/slhmy/identra/internal/security"
	"golang.org/x/oauth2"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var supportedProviders = map[string]struct{}{
	"github": {},
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
	accessToken := accessTokenFromRequest(ctx, req.GetAccessToken())
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
	case errors.Is(err, ErrNotFound):
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
	if userInfo.ID == "" {
		slog.ErrorContext(ctx, "provider returned empty user id (bind)", "provider", stateData.Provider)
		return nil, status.Error(codes.Internal, "provider returned empty user id")
	}

	providerIdentity, err := s.externalIdentityStore.GetByProviderID(ctx, stateData.Provider, userInfo.ID)
	switch {
	case err == nil && providerIdentity.UserID != bindingUser.ID:
		return nil, status.Error(codes.AlreadyExists, "oauth account already linked to another user")
	case err == nil:
		// Already linked to this user; continue to refresh token pair.
	case errors.Is(err, ErrNotFound):
		// Not linked yet; create the external identity.
		identity := &ExternalIdentityModel{
			UserID:         bindingUser.ID,
			Provider:       stateData.Provider,
			ProviderUserID: userInfo.ID,
		}
		if createErr := s.externalIdentityStore.Create(ctx, identity); createErr != nil {
			if errors.Is(createErr, ErrAlreadyExists) {
				// A concurrent request may have created the same identity. Re-fetch
				// and treat as success if it is linked to this user.
				existing, refetchErr := s.externalIdentityStore.GetByProviderID(ctx, stateData.Provider, userInfo.ID)
				if refetchErr != nil {
					return nil, status.Error(codes.Internal, "failed to verify oauth link")
				}
				if existing.UserID != bindingUser.ID {
					return nil, status.Error(codes.AlreadyExists, "oauth account already linked to another user")
				}
				// Idempotent: identity exists and belongs to this user.
			} else {
				return nil, status.Error(codes.Internal, "failed to link oauth account")
			}
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

func (s *Service) ensureOAuthUser(ctx context.Context, info UserInfo) (*UserModel, error) {
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
	case errors.Is(err, ErrNotFound):
		// No existing external identity; try to link by email if available.
		if strings.TrimSpace(info.Email) != "" {
			byEmail, emailErr := s.userStore.GetByEmail(ctx, info.Email)
			switch {
			case emailErr == nil:
				// Merge: link the external identity to the existing user.
				identity := &ExternalIdentityModel{
					UserID:         byEmail.ID,
					Provider:       info.Provider,
					ProviderUserID: info.ID,
				}
				if createErr := s.externalIdentityStore.Create(ctx, identity); createErr != nil {
					if errors.Is(createErr, ErrAlreadyExists) {
						// A concurrent request may have created the same identity for
						// this user. Re-fetch and proceed if it belongs to the same user.
						existingIdentity, getErr := s.externalIdentityStore.GetByProviderID(ctx, info.Provider, info.ID)
						if getErr != nil {
							return nil, status.Error(codes.Internal, "failed to verify oauth account link")
						}
						if existingIdentity.UserID == byEmail.ID {
							return byEmail, nil
						}
						return nil, status.Error(codes.AlreadyExists, "oauth account already linked to another user")
					}
					return nil, status.Error(codes.Internal, "failed to link oauth account")
				}
				return byEmail, nil
			case errors.Is(emailErr, ErrNotFound):
				// Create new user and link external identity.
				userModel := &UserModel{Email: info.Email}
				if createErr := s.userStore.Create(ctx, userModel); createErr != nil {
					if errors.Is(createErr, ErrAlreadyExists) {
						return nil, status.Error(codes.AlreadyExists, "user already exists")
					}
					return nil, status.Error(codes.Internal, "failed to create user")
				}
				identity := &ExternalIdentityModel{
					UserID:         userModel.ID,
					Provider:       info.Provider,
					ProviderUserID: info.ID,
				}
				if createErr := s.externalIdentityStore.Create(ctx, identity); createErr != nil {
					// Determine the response code before attempting cleanup so that the
					// cleanup outcome does not affect the error returned to the caller.
					isConflict := errors.Is(createErr, ErrAlreadyExists)
					// Compensate: remove the newly created user to avoid orphaned records.
					if deleteErr := s.userStore.Delete(ctx, userModel.ID); deleteErr != nil {
						slog.ErrorContext(ctx, "failed to clean up orphaned user after identity create failure",
							"error", deleteErr, "user_id", userModel.ID)
					}
					if isConflict {
						return nil, status.Error(codes.AlreadyExists, "oauth account already linked")
					}
					return nil, status.Error(codes.Internal, "failed to create oauth identity")
				}
				return userModel, nil
			default:
				return nil, status.Error(codes.Internal, "failed to fetch user by email")
			}
		}
		// The current persistence layer enforces unique email values, so creating
		// a user without an email address can lead to duplicate empty-email
		// records and subsequent signup failures. Reject this flow until missing
		// emails are represented distinctly at the model/index level.
		return nil, status.Error(codes.FailedPrecondition, "oauth provider did not supply an email address")
	default:
		return nil, status.Error(codes.Internal, "failed to fetch user by provider id")
	}
}

func (s *Service) updateEmailIfNeeded(ctx context.Context, usr *UserModel, email string) (*UserModel, error) {
	if usr.Email == email || email == "" {
		return usr, nil
	}

	usr.Email = email
	if err := s.userStore.Update(ctx, usr); err != nil {
		return nil, status.Error(codes.Internal, "failed to update user email")
	}
	return usr, nil
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

func (s *Service) oauthConfigForRedirect(redirectURL string) *oauth2.Config {
	cfg := *s.githubOAuthConfig
	cfg.RedirectURL = redirectURL
	return &cfg
}

func generateOAuthState() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
