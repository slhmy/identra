package identra

import (
	"context"

	identra_v1_pb "github.com/slhmy/identra/gen/go/identra/v1"
)

type ServerInfo struct {
	Version       string
	Commit        string
	BuildDate     string
	SchemaVersion uint32
	Capabilities  []string
}

func (s *Service) GetServerInfo(context.Context, *identra_v1_pb.GetServerInfoRequest) (*identra_v1_pb.GetServerInfoResponse, error) {
	return &identra_v1_pb.GetServerInfoResponse{
		Version:       s.serverInfo.Version,
		Commit:        s.serverInfo.Commit,
		BuildDate:     s.serverInfo.BuildDate,
		SchemaVersion: s.serverInfo.SchemaVersion,
		Capabilities:  append([]string(nil), s.serverInfo.Capabilities...),
	}, nil
}
