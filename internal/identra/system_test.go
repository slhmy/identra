package identra

import (
	"context"
	"testing"

	identra_v1_pb "github.com/slhmy/identra/gen/go/identra/v1"
)

func TestGetServerInfo(t *testing.T) {
	svc := &Service{serverInfo: ServerInfo{
		Version: "v0.2.0-rc.1", Commit: "abc123", BuildDate: "2026-07-18",
		SchemaVersion: 3, Capabilities: []string{"service_accounts", "audit_events"},
	}}
	response, err := svc.GetServerInfo(context.Background(), &identra_v1_pb.GetServerInfoRequest{})
	if err != nil {
		t.Fatalf("get server info: %v", err)
	}
	if response.GetVersion() != "v0.2.0-rc.1" || response.GetSchemaVersion() != 3 {
		t.Fatalf("unexpected server info: %+v", response)
	}
}
