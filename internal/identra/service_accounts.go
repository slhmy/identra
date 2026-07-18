package identra

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	identra_v1_pb "github.com/slhmy/identra/gen/go/identra/v1"
	"github.com/slhmy/identra/internal/security"
	"github.com/slhmy/identra/internal/serviceaccount"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const (
	ScopeAdmin                 = "identra.admin"
	ScopeServiceAccountsRead   = "identra.service_accounts.read"
	ScopeServiceAccountsManage = "identra.service_accounts.manage"
)

func (s *Service) ExchangeServiceToken(ctx context.Context, req *identra_v1_pb.ExchangeServiceTokenRequest) (*identra_v1_pb.ExchangeServiceTokenResponse, error) {
	account, err := serviceaccount.Authenticate(ctx, s.serviceAccountStore, req.GetClientId(), req.GetClientSecret())
	if err != nil {
		if errors.Is(err, serviceaccount.ErrInvalidCredential) || errors.Is(err, serviceaccount.ErrNotFound) {
			return nil, status.Error(codes.Unauthenticated, "invalid service-account credential")
		}
		slog.ErrorContext(ctx, "failed to authenticate service account", "error", err)
		return nil, status.Error(codes.Internal, "failed to authenticate service account")
	}
	token, err := security.NewServiceToken(account.ID, account.Scopes, s.tokenCfg)
	if err != nil {
		slog.ErrorContext(ctx, "failed to issue service token", "error", err, "client_id", account.ID)
		return nil, status.Error(codes.Internal, "failed to issue service token")
	}
	return &identra_v1_pb.ExchangeServiceTokenResponse{Token: token}, nil
}

func (s *Service) CreateServiceAccount(ctx context.Context, req *identra_v1_pb.CreateServiceAccountRequest) (*identra_v1_pb.CreateServiceAccountResponse, error) {
	if _, err := s.authorizeServiceAccount(ctx, ScopeServiceAccountsManage); err != nil {
		return nil, err
	}
	result, err := serviceaccount.Create(ctx, s.serviceAccountStore, req.GetName(), req.GetScopes())
	if err != nil {
		switch {
		case errors.Is(err, serviceaccount.ErrAlreadyExists):
			return nil, status.Error(codes.AlreadyExists, "service account already exists")
		case strings.Contains(err.Error(), "required"), strings.Contains(err.Error(), "invalid scope"), strings.Contains(err.Error(), "must not exceed"):
			return nil, status.Error(codes.InvalidArgument, err.Error())
		default:
			slog.ErrorContext(ctx, "failed to create service account", "error", err)
			return nil, status.Error(codes.Internal, "failed to create service account")
		}
	}
	return &identra_v1_pb.CreateServiceAccountResponse{
		ServiceAccount: serviceAccountProto(result.Account),
		Credential: &identra_v1_pb.ServiceAccountCredential{
			ClientId: result.ID, ClientSecret: result.ClientSecret,
		},
	}, nil
}

func (s *Service) ListServiceAccounts(ctx context.Context, _ *identra_v1_pb.ListServiceAccountsRequest) (*identra_v1_pb.ListServiceAccountsResponse, error) {
	if _, err := s.authorizeServiceAccount(ctx, ScopeServiceAccountsRead, ScopeServiceAccountsManage); err != nil {
		return nil, err
	}
	accounts, err := s.serviceAccountStore.List(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "failed to list service accounts", "error", err)
		return nil, status.Error(codes.Internal, "failed to list service accounts")
	}
	response := &identra_v1_pb.ListServiceAccountsResponse{ServiceAccounts: make([]*identra_v1_pb.ServiceAccount, 0, len(accounts))}
	for _, account := range accounts {
		response.ServiceAccounts = append(response.ServiceAccounts, serviceAccountProto(account))
	}
	return response, nil
}

func (s *Service) DisableServiceAccount(ctx context.Context, req *identra_v1_pb.DisableServiceAccountRequest) (*identra_v1_pb.DisableServiceAccountResponse, error) {
	actor, err := s.authorizeServiceAccount(ctx, ScopeServiceAccountsManage)
	if err != nil {
		return nil, err
	}
	clientID := strings.TrimSpace(req.GetClientId())
	if clientID == "" {
		return nil, status.Error(codes.InvalidArgument, "client ID is required")
	}
	if clientID == actor.ID {
		return nil, status.Error(codes.FailedPrecondition, "a service account cannot disable itself")
	}
	account, err := s.serviceAccountStore.Disable(ctx, clientID, time.Now().UTC())
	if errors.Is(err, serviceaccount.ErrNotFound) {
		return nil, status.Error(codes.NotFound, "active service account not found")
	}
	if err != nil {
		slog.ErrorContext(ctx, "failed to disable service account", "error", err, "client_id", clientID)
		return nil, status.Error(codes.Internal, "failed to disable service account")
	}
	return &identra_v1_pb.DisableServiceAccountResponse{ServiceAccount: serviceAccountProto(account)}, nil
}

func (s *Service) RotateServiceAccountSecret(ctx context.Context, req *identra_v1_pb.RotateServiceAccountSecretRequest) (*identra_v1_pb.RotateServiceAccountSecretResponse, error) {
	if _, err := s.authorizeServiceAccount(ctx, ScopeServiceAccountsManage); err != nil {
		return nil, err
	}
	clientID := strings.TrimSpace(req.GetClientId())
	if clientID == "" {
		return nil, status.Error(codes.InvalidArgument, "client ID is required")
	}
	secret, err := serviceaccount.RotateCredential(ctx, s.serviceAccountStore, clientID)
	if errors.Is(err, serviceaccount.ErrNotFound) {
		return nil, status.Error(codes.NotFound, "active service account not found")
	}
	if err != nil {
		slog.ErrorContext(ctx, "failed to rotate service-account secret", "error", err, "client_id", clientID)
		return nil, status.Error(codes.Internal, "failed to rotate service-account secret")
	}
	return &identra_v1_pb.RotateServiceAccountSecretResponse{
		Credential: &identra_v1_pb.ServiceAccountCredential{ClientId: clientID, ClientSecret: secret},
	}, nil
}

func (s *Service) authorizeServiceAccount(ctx context.Context, requiredScopes ...string) (serviceaccount.Account, error) {
	token := accessTokenFromMetadata(ctx)
	if token == "" {
		return serviceaccount.Account{}, status.Error(codes.Unauthenticated, "service token is required")
	}
	claims, err := security.ValidateServiceToken(token, s.tokenCfg.PublicKey)
	if err != nil {
		return serviceaccount.Account{}, status.Error(codes.Unauthenticated, "invalid service token")
	}
	account, err := s.serviceAccountStore.GetByID(ctx, claims.ServiceAccountID)
	if err != nil || account.DisabledAt != nil {
		return serviceaccount.Account{}, status.Error(codes.Unauthenticated, "service account is inactive")
	}
	for _, required := range requiredScopes {
		if hasServiceScope(account.Scopes, required) {
			return account, nil
		}
	}
	return serviceaccount.Account{}, status.Error(codes.PermissionDenied, "service account lacks the required scope")
}

func hasServiceScope(scopes []string, required string) bool {
	for _, scope := range scopes {
		if scope == ScopeAdmin || scope == required {
			return true
		}
		if strings.HasSuffix(scope, "*") && strings.HasPrefix(required, strings.TrimSuffix(scope, "*")) {
			return true
		}
	}
	return false
}

func serviceAccountProto(account serviceaccount.Account) *identra_v1_pb.ServiceAccount {
	result := &identra_v1_pb.ServiceAccount{
		ClientId:  account.ID,
		Name:      account.Name,
		Scopes:    append([]string(nil), account.Scopes...),
		CreatedAt: timestamppb.New(account.CreatedAt),
	}
	if account.DisabledAt != nil {
		result.DisabledAt = timestamppb.New(*account.DisabledAt)
	}
	return result
}
