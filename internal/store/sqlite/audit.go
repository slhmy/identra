package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/slhmy/identra/internal/identra"
)

type AuditStore struct {
	db *sql.DB
}

var _ identra.AuditStore = (*AuditStore)(nil)

func NewAuditStore(db *sql.DB) *AuditStore {
	return &AuditStore{db: db}
}

func (s *AuditStore) Record(ctx context.Context, event identra.AuditEvent) error {
	metadata, err := json.Marshal(event.Metadata)
	if err != nil {
		return fmt.Errorf("encode audit metadata: %w", err)
	}
	_, err = s.db.ExecContext(ctx, `
INSERT INTO audit_events
  (id, occurred_at, actor_type, actor_id, action, resource_type, resource_id, metadata)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, event.ID, event.OccurredAt, event.ActorType, event.ActorID, event.Action, event.ResourceType, event.ResourceID, string(metadata))
	if err != nil {
		return fmt.Errorf("record audit event: %w", err)
	}
	return nil
}

func (s *AuditStore) List(ctx context.Context, offset, limit int) ([]identra.AuditEvent, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, occurred_at, actor_type, actor_id, action, resource_type, resource_id, metadata
FROM audit_events
ORDER BY occurred_at DESC, id DESC
LIMIT ? OFFSET ?`, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list audit events: %w", err)
	}
	defer func() { _ = rows.Close() }()

	events := make([]identra.AuditEvent, 0)
	for rows.Next() {
		var event identra.AuditEvent
		var metadata string
		if err := rows.Scan(&event.ID, &event.OccurredAt, &event.ActorType, &event.ActorID, &event.Action, &event.ResourceType, &event.ResourceID, &metadata); err != nil {
			return nil, fmt.Errorf("scan audit event: %w", err)
		}
		if err := json.Unmarshal([]byte(metadata), &event.Metadata); err != nil {
			return nil, fmt.Errorf("decode audit metadata: %w", err)
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate audit events: %w", err)
	}
	return events, nil
}
