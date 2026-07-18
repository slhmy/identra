package identra

import (
	"context"

	identra_v1_pb "github.com/slhmy/identra/gen/go/identra/v1"
)

func (s *Service) ListSigningKeys(
	_ context.Context,
	_ *identra_v1_pb.ListSigningKeysRequest,
) (*identra_v1_pb.ListSigningKeysResponse, error) {
	return s.keyManager.ListSigningKeys(), nil
}
