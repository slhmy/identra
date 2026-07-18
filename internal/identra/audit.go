package identra

import (
	"context"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	identra_v1_pb "github.com/slhmy/identra/gen/go/identra/v1"
	"github.com/slhmy/identra/internal/serviceaccount"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const (
	ScopeAuditRead = "identra.audit.read"

	AuditActorSystem         = "system"
	AuditActorServiceAccount = "service_account"
)

func (s *Service) ListAuditEvents(ctx context.Context, req *identra_v1_pb.ListAuditEventsRequest) (*identra_v1_pb.ListAuditEventsResponse, error) {
	if _, err := s.authorizeServiceAccount(ctx, ScopeAuditRead); err != nil {
		return nil, err
	}
	pageSize := int(req.GetPageSize())
	if pageSize == 0 {
		pageSize = 50
	}
	if pageSize > 200 {
		return nil, status.Error(codes.InvalidArgument, "page size must not exceed 200")
	}
	offset := 0
	if token := strings.TrimSpace(req.GetPageToken()); token != "" {
		value, err := strconv.Atoi(token)
		if err != nil || value < 0 {
			return nil, status.Error(codes.InvalidArgument, "invalid page token")
		}
		offset = value
	}
	events, err := s.auditStore.List(ctx, offset, pageSize+1)
	if err != nil {
		slog.ErrorContext(ctx, "failed to list audit events", "error", err)
		return nil, status.Error(codes.Internal, "failed to list audit events")
	}
	response := &identra_v1_pb.ListAuditEventsResponse{}
	if len(events) > pageSize {
		events = events[:pageSize]
		response.NextPageToken = strconv.Itoa(offset + pageSize)
	}
	for _, event := range events {
		response.AuditEvents = append(response.AuditEvents, auditEventProto(event))
	}
	return response, nil
}

func (s *Service) recordManagementAudit(ctx context.Context, actor serviceaccount.Account, action, resourceType, resourceID string) {
	if s.auditStore == nil {
		return
	}
	event := AuditEvent{
		ID:           uuid.NewString(),
		OccurredAt:   time.Now().UTC(),
		ActorType:    AuditActorServiceAccount,
		ActorID:      actor.ID,
		Action:       action,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		Metadata:     map[string]string{},
	}
	if err := s.auditStore.Record(ctx, event); err != nil {
		slog.ErrorContext(ctx, "failed to record management audit event", "error", err, "action", action, "resource_id", resourceID)
	}
}

func auditEventProto(event AuditEvent) *identra_v1_pb.AuditEvent {
	actorType := identra_v1_pb.AuditActorType_AUDIT_ACTOR_TYPE_UNSPECIFIED
	switch event.ActorType {
	case AuditActorSystem:
		actorType = identra_v1_pb.AuditActorType_AUDIT_ACTOR_TYPE_SYSTEM
	case AuditActorServiceAccount:
		actorType = identra_v1_pb.AuditActorType_AUDIT_ACTOR_TYPE_SERVICE_ACCOUNT
	}
	return &identra_v1_pb.AuditEvent{
		Id:           event.ID,
		OccurredAt:   timestamppb.New(event.OccurredAt),
		ActorType:    actorType,
		ActorId:      event.ActorID,
		Action:       event.Action,
		ResourceType: event.ResourceType,
		ResourceId:   event.ResourceID,
		Metadata:     event.Metadata,
	}
}
