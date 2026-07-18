package identra

import (
	"context"
	"testing"
	"time"

	identra_v1_pb "github.com/slhmy/identra/gen/go/identra/v1"
	"github.com/slhmy/identra/internal/security"
	"github.com/slhmy/identra/internal/serviceaccount"
	"google.golang.org/grpc/codes"
)

type memoryAuditStore struct {
	events []AuditEvent
}

func (s *memoryAuditStore) Record(_ context.Context, event AuditEvent) error {
	s.events = append(s.events, event)
	return nil
}

func (s *memoryAuditStore) List(_ context.Context, offset, limit int) ([]AuditEvent, error) {
	if offset >= len(s.events) {
		return nil, nil
	}
	end := offset + limit
	if end > len(s.events) {
		end = len(s.events)
	}
	return append([]AuditEvent(nil), s.events[offset:end]...), nil
}

func TestListAuditEventsRequiresScopeAndPaginates(t *testing.T) {
	accounts := newMemoryServiceAccountStore()
	admin, err := serviceaccount.Bootstrap(context.Background(), accounts, serviceaccount.BootstrapRequest{
		Name: "auditor", Scopes: []string{ScopeAuditRead},
	})
	if err != nil {
		t.Fatalf("bootstrap auditor: %v", err)
	}
	tokenConfig := newTestTokenConfig(t)
	token, err := security.NewServiceToken(admin.ID, admin.Scopes, tokenConfig)
	if err != nil {
		t.Fatalf("issue service token: %v", err)
	}
	audits := &memoryAuditStore{events: []AuditEvent{
		{ID: "one", OccurredAt: time.Now(), ActorType: AuditActorSystem, Metadata: map[string]string{}},
		{ID: "two", OccurredAt: time.Now(), ActorType: AuditActorServiceAccount, Metadata: map[string]string{}},
	}}
	svc := &Service{serviceAccountStore: accounts, tokenCfg: tokenConfig, auditStore: audits}

	_, err = svc.ListAuditEvents(context.Background(), &identra_v1_pb.ListAuditEventsRequest{})
	requireCode(t, err, codes.Unauthenticated)
	response, err := svc.ListAuditEvents(serviceTokenContext(token.Value), &identra_v1_pb.ListAuditEventsRequest{PageSize: 1})
	if err != nil {
		t.Fatalf("list first page: %v", err)
	}
	if len(response.GetAuditEvents()) != 1 || response.GetNextPageToken() != "1" {
		t.Fatalf("unexpected first page: %+v", response)
	}
}
