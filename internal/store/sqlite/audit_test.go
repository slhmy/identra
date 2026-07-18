package sqlite

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/slhmy/identra/internal/identra"
)

func TestAuditStoreRecordsAndListsNewestFirst(t *testing.T) {
	db, err := Open(Config{Path: filepath.Join(t.TempDir(), "audit.db")})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store := NewAuditStore(db)
	now := time.Now().UTC().Round(time.Millisecond)
	for _, event := range []identra.AuditEvent{
		{ID: "older", OccurredAt: now.Add(-time.Minute), ActorType: identra.AuditActorSystem, ActorID: "bootstrap", Action: "bootstrap", ResourceType: "service_account", ResourceID: "one", Metadata: map[string]string{}},
		{ID: "newer", OccurredAt: now, ActorType: identra.AuditActorServiceAccount, ActorID: "admin", Action: "service_account.create", ResourceType: "service_account", ResourceID: "two", Metadata: map[string]string{"name": "worker"}},
	} {
		if err := store.Record(context.Background(), event); err != nil {
			t.Fatalf("record %s: %v", event.ID, err)
		}
	}
	events, err := store.List(context.Background(), 0, 10)
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(events) != 2 || events[0].ID != "newer" || events[0].Metadata["name"] != "worker" {
		t.Fatalf("unexpected events: %+v", events)
	}
}
