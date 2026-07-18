package identra

import (
	"context"
	"sort"
	"testing"
	"time"

	identra_v1_pb "github.com/slhmy/identra/gen/go/identra/v1"
	"github.com/slhmy/identra/internal/serviceaccount"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
)

func TestServiceAccountManagementFlow(t *testing.T) {
	store := newMemoryServiceAccountStore()
	admin, err := serviceaccount.Bootstrap(context.Background(), store, serviceaccount.BootstrapRequest{
		Name: "platform-admin", Scopes: []string{ScopeAdmin},
	})
	if err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	svc := &Service{serviceAccountStore: store, tokenCfg: newTestTokenConfig(t)}

	exchanged, err := svc.ExchangeServiceToken(context.Background(), &identra_v1_pb.ExchangeServiceTokenRequest{
		ClientId: admin.ID, ClientSecret: admin.ClientSecret,
	})
	if err != nil || exchanged.GetToken().GetValue() == "" {
		t.Fatalf("exchange result=%+v error=%v", exchanged, err)
	}
	adminCtx := serviceTokenContext(exchanged.Token.Value)
	created, err := svc.CreateServiceAccount(adminCtx, &identra_v1_pb.CreateServiceAccountRequest{
		Name: "reporting-worker", Scopes: []string{ScopeServiceAccountsRead},
	})
	if err != nil {
		t.Fatalf("create service account: %v", err)
	}
	workerID := created.GetCredential().GetClientId()
	workerSecret := created.GetCredential().GetClientSecret()
	if workerID == "" || workerSecret == "" {
		t.Fatalf("missing one-time credential: %+v", created)
	}

	listed, err := svc.ListServiceAccounts(adminCtx, &identra_v1_pb.ListServiceAccountsRequest{})
	if err != nil || len(listed.GetServiceAccounts()) != 2 {
		t.Fatalf("list result=%+v error=%v", listed, err)
	}
	workerToken, err := svc.ExchangeServiceToken(context.Background(), &identra_v1_pb.ExchangeServiceTokenRequest{
		ClientId: workerID, ClientSecret: workerSecret,
	})
	if err != nil {
		t.Fatalf("exchange worker token: %v", err)
	}
	_, err = svc.CreateServiceAccount(serviceTokenContext(workerToken.Token.Value), &identra_v1_pb.CreateServiceAccountRequest{
		Name: "forbidden", Scopes: []string{ScopeServiceAccountsRead},
	})
	requireCode(t, err, codes.PermissionDenied)

	rotated, err := svc.RotateServiceAccountSecret(adminCtx, &identra_v1_pb.RotateServiceAccountSecretRequest{ClientId: workerID})
	if err != nil || rotated.GetCredential().GetClientSecret() == "" {
		t.Fatalf("rotate result=%+v error=%v", rotated, err)
	}
	_, err = svc.ExchangeServiceToken(context.Background(), &identra_v1_pb.ExchangeServiceTokenRequest{
		ClientId: workerID, ClientSecret: workerSecret,
	})
	requireCode(t, err, codes.Unauthenticated)

	disabled, err := svc.DisableServiceAccount(adminCtx, &identra_v1_pb.DisableServiceAccountRequest{ClientId: workerID})
	if err != nil || disabled.GetServiceAccount().GetDisabledAt() == nil {
		t.Fatalf("disable result=%+v error=%v", disabled, err)
	}
	_, err = svc.ListServiceAccounts(serviceTokenContext(workerToken.Token.Value), &identra_v1_pb.ListServiceAccountsRequest{})
	requireCode(t, err, codes.Unauthenticated)
	_, err = svc.DisableServiceAccount(adminCtx, &identra_v1_pb.DisableServiceAccountRequest{ClientId: admin.ID})
	requireCode(t, err, codes.FailedPrecondition)
}

func TestExchangeServiceTokenRejectsInvalidCredential(t *testing.T) {
	limiter := newMockRateLimiter(true)
	svc := &Service{serviceAccountStore: &memoryServiceAccountStore{}, tokenCfg: newTestTokenConfig(t), serviceTokenRateLimiter: limiter}
	_, err := svc.ExchangeServiceToken(context.Background(), &identra_v1_pb.ExchangeServiceTokenRequest{})
	requireCode(t, err, codes.Unauthenticated)
	if limiter.recorded != 1 {
		t.Fatalf("rate-limit records = %d, want 1", limiter.recorded)
	}
}

func TestExchangeServiceTokenRateLimitBlocksBeforeAuthentication(t *testing.T) {
	svc := &Service{
		serviceAccountStore:     &memoryServiceAccountStore{},
		tokenCfg:                newTestTokenConfig(t),
		serviceTokenRateLimiter: newMockRateLimiter(false),
	}
	_, err := svc.ExchangeServiceToken(context.Background(), &identra_v1_pb.ExchangeServiceTokenRequest{ClientId: "blocked"})
	requireCode(t, err, codes.ResourceExhausted)
}

func serviceTokenContext(token string) context.Context {
	return metadata.NewIncomingContext(context.Background(), metadata.Pairs("authorization", "Bearer "+token))
}

type memoryServiceAccountStore struct {
	accounts map[string]serviceaccount.Account
	hashes   map[string]string
}

func newMemoryServiceAccountStore() *memoryServiceAccountStore {
	return &memoryServiceAccountStore{
		accounts: make(map[string]serviceaccount.Account),
		hashes:   make(map[string]string),
	}
}

func (s *memoryServiceAccountStore) Bootstrap(ctx context.Context, record serviceaccount.BootstrapRecord, _, _ bool) (serviceaccount.Account, bool, error) {
	if err := s.Create(ctx, record); err != nil {
		return serviceaccount.Account{}, false, err
	}
	return record.Account, true, nil
}
func (s *memoryServiceAccountStore) Create(_ context.Context, record serviceaccount.BootstrapRecord) error {
	for _, account := range s.accounts {
		if account.Name == record.Account.Name {
			return serviceaccount.ErrAlreadyExists
		}
	}
	s.accounts[record.Account.ID] = record.Account
	s.hashes[record.Account.ID] = record.SecretHash
	return nil
}
func (s *memoryServiceAccountStore) Authenticate(_ context.Context, clientID, hash string, _ time.Time) (serviceaccount.Account, error) {
	account, ok := s.accounts[clientID]
	if !ok || account.DisabledAt != nil || s.hashes[clientID] != hash {
		return serviceaccount.Account{}, serviceaccount.ErrInvalidCredential
	}
	return account, nil
}
func (s *memoryServiceAccountStore) GetByID(_ context.Context, clientID string) (serviceaccount.Account, error) {
	account, ok := s.accounts[clientID]
	if !ok {
		return serviceaccount.Account{}, serviceaccount.ErrNotFound
	}
	return account, nil
}
func (s *memoryServiceAccountStore) List(context.Context) ([]serviceaccount.Account, error) {
	accounts := make([]serviceaccount.Account, 0, len(s.accounts))
	for _, account := range s.accounts {
		accounts = append(accounts, account)
	}
	sort.Slice(accounts, func(i, j int) bool { return accounts[i].ID < accounts[j].ID })
	return accounts, nil
}
func (s *memoryServiceAccountStore) Disable(_ context.Context, clientID string, now time.Time) (serviceaccount.Account, error) {
	account, ok := s.accounts[clientID]
	if !ok || account.DisabledAt != nil {
		return serviceaccount.Account{}, serviceaccount.ErrNotFound
	}
	account.DisabledAt = &now
	s.accounts[clientID] = account
	return account, nil
}
func (s *memoryServiceAccountStore) RotateCredential(_ context.Context, clientID, _, hash string, _ time.Time) error {
	account, ok := s.accounts[clientID]
	if !ok || account.DisabledAt != nil {
		return serviceaccount.ErrNotFound
	}
	s.hashes[clientID] = hash
	return nil
}
