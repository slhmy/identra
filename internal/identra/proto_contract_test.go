package identra

import (
	"testing"

	identra_v1_pb "github.com/slhmy/identra/gen/go/identra/v1"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
)

var (
	_ identra_v1_pb.AuthServiceServer           = (*Service)(nil)
	_ identra_v1_pb.SessionServiceServer        = (*Service)(nil)
	_ identra_v1_pb.UserServiceServer           = (*Service)(nil)
	_ identra_v1_pb.KeyServiceServer            = (*Service)(nil)
	_ identra_v1_pb.ServiceAccountServiceServer = (*Service)(nil)
)

func TestPublicGRPCServices(t *testing.T) {
	want := map[protoreflect.FullName][]protoreflect.Name{
		"identra.v1.AuthService": {
			"RegisterWithPassword", "LoginWithPassword", "RequestEmailLoginCode",
			"LoginWithEmailCode", "ListOAuthProviders", "StartOAuthLogin", "LoginWithOAuth",
		},
		"identra.v1.SessionService": {"RefreshSession", "RevokeSession"},
		"identra.v1.UserService":    {"GetCurrentUser", "LinkOAuthAccount"},
		"identra.v1.KeyService":     {"ListSigningKeys"},
		"identra.v1.ServiceAccountService": {
			"ExchangeServiceToken", "CreateServiceAccount", "ListServiceAccounts",
			"DisableServiceAccount", "RotateServiceAccountSecret",
		},
	}

	for serviceName, wantMethods := range want {
		descriptor, err := protoregistry.GlobalFiles.FindDescriptorByName(serviceName)
		if err != nil {
			t.Fatalf("find %s: %v", serviceName, err)
		}
		service, ok := descriptor.(protoreflect.ServiceDescriptor)
		if !ok {
			t.Fatalf("%s is %T, not a service descriptor", serviceName, descriptor)
		}
		if service.Methods().Len() != len(wantMethods) {
			t.Fatalf("%s has %d methods, want %d", serviceName, service.Methods().Len(), len(wantMethods))
		}
		for i, wantMethod := range wantMethods {
			if got := service.Methods().Get(i).Name(); got != wantMethod {
				t.Errorf("%s method %d = %s, want %s", serviceName, i, got, wantMethod)
			}
		}
	}
}
