package main

import (
	"context"
	"crypto/tls"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	identra_v1_pb "github.com/slhmy/identra/gen/go/identra/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

const defaultEndpoint = "localhost:50051"

type grpcClientFlags struct {
	endpoint *string
	tls      *bool
	timeout  *time.Duration
}

func addGRPCClientFlags(flags *flag.FlagSet) grpcClientFlags {
	endpointDefault := strings.TrimSpace(os.Getenv("IDENTRA_ENDPOINT"))
	if endpointDefault == "" {
		endpointDefault = defaultEndpoint
	}
	return grpcClientFlags{
		endpoint: flags.String("endpoint", endpointDefault, "Identra gRPC endpoint"),
		tls:      flags.Bool("tls", false, "use TLS for the gRPC connection"),
		timeout:  flags.Duration("timeout", 15*time.Second, "RPC timeout"),
	}
}

func (f grpcClientFlags) connect() (*grpc.ClientConn, context.Context, context.CancelFunc, error) {
	if strings.TrimSpace(*f.endpoint) == "" {
		return nil, nil, nil, errors.New("endpoint is required")
	}
	var transport credentials.TransportCredentials
	if *f.tls {
		transport = credentials.NewTLS(&tls.Config{MinVersion: tls.VersionTLS12})
	} else {
		transport = insecure.NewCredentials()
	}
	conn, err := grpc.NewClient(*f.endpoint, grpc.WithTransportCredentials(transport))
	if err != nil {
		return nil, nil, nil, fmt.Errorf("create gRPC client: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), *f.timeout)
	return conn, ctx, cancel, nil
}

func runTokenCommand(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 || args[0] != "service" {
		return errors.New("usage: identra token service --client-id ID [--client-secret-file FILE]")
	}
	flags := flag.NewFlagSet("token service", flag.ContinueOnError)
	flags.SetOutput(stderr)
	client := addGRPCClientFlags(flags)
	clientIDDefault := strings.TrimSpace(os.Getenv("IDENTRA_CLIENT_ID"))
	clientID := flags.String("client-id", clientIDDefault, "service-account client ID")
	secretFile := flags.String("client-secret-file", "", "file containing the client secret; defaults to IDENTRA_CLIENT_SECRET")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected arguments: %s", strings.Join(flags.Args(), " "))
	}
	secret, err := readSecret(*secretFile, "IDENTRA_CLIENT_SECRET")
	if err != nil {
		return err
	}
	if strings.TrimSpace(*clientID) == "" {
		return errors.New("client ID is required")
	}
	conn, ctx, cancel, err := client.connect()
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()
	defer cancel()
	response, err := identra_v1_pb.NewServiceAccountServiceClient(conn).ExchangeServiceToken(ctx, &identra_v1_pb.ExchangeServiceTokenRequest{
		ClientId: *clientID, ClientSecret: secret,
	})
	if err != nil {
		return fmt.Errorf("exchange service token: %w", err)
	}
	return writeProtoJSON(stdout, response)
}

func runServiceAccountCommand(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		return errors.New("usage: identra service-account create|list|disable|rotate")
	}
	switch args[0] {
	case "create":
		return runServiceAccountCreate(args[1:], stdout, stderr)
	case "list":
		return runServiceAccountList(args[1:], stdout, stderr)
	case "disable":
		return runServiceAccountDisable(args[1:], stdout, stderr)
	case "rotate":
		return runServiceAccountRotate(args[1:], stdout, stderr)
	default:
		return fmt.Errorf("unknown service-account command %q", args[0])
	}
}

func runServiceAccountCreate(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("service-account create", flag.ContinueOnError)
	flags.SetOutput(stderr)
	client := addGRPCClientFlags(flags)
	name := flags.String("name", "", "service-account name")
	tokenFile := flags.String("token-file", "", "file containing a service token; defaults to IDENTRA_SERVICE_TOKEN")
	var scopes stringList
	flags.Var(&scopes, "scope", "granted scope; may be repeated")
	if err := flags.Parse(args); err != nil {
		return err
	}
	response := &identra_v1_pb.CreateServiceAccountResponse{}
	err := callServiceAccountRPC(flags, client, *tokenFile, func(ctx context.Context, api identra_v1_pb.ServiceAccountServiceClient) error {
		var err error
		response, err = api.CreateServiceAccount(ctx, &identra_v1_pb.CreateServiceAccountRequest{Name: *name, Scopes: scopes})
		return err
	})
	if err != nil {
		return err
	}
	return writeProtoJSON(stdout, response)
}

func runServiceAccountList(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("service-account list", flag.ContinueOnError)
	flags.SetOutput(stderr)
	client := addGRPCClientFlags(flags)
	tokenFile := flags.String("token-file", "", "file containing a service token; defaults to IDENTRA_SERVICE_TOKEN")
	if err := flags.Parse(args); err != nil {
		return err
	}
	response := &identra_v1_pb.ListServiceAccountsResponse{}
	err := callServiceAccountRPC(flags, client, *tokenFile, func(ctx context.Context, api identra_v1_pb.ServiceAccountServiceClient) error {
		var err error
		response, err = api.ListServiceAccounts(ctx, &identra_v1_pb.ListServiceAccountsRequest{})
		return err
	})
	if err != nil {
		return err
	}
	return writeProtoJSON(stdout, response)
}

func runServiceAccountDisable(args []string, stdout, stderr io.Writer) error {
	return runServiceAccountByID("disable", args, stdout, stderr, func(ctx context.Context, api identra_v1_pb.ServiceAccountServiceClient, clientID string) (proto.Message, error) {
		return api.DisableServiceAccount(ctx, &identra_v1_pb.DisableServiceAccountRequest{ClientId: clientID})
	})
}

func runServiceAccountRotate(args []string, stdout, stderr io.Writer) error {
	return runServiceAccountByID("rotate", args, stdout, stderr, func(ctx context.Context, api identra_v1_pb.ServiceAccountServiceClient, clientID string) (proto.Message, error) {
		return api.RotateServiceAccountSecret(ctx, &identra_v1_pb.RotateServiceAccountSecretRequest{ClientId: clientID})
	})
}

func runServiceAccountByID(command string, args []string, stdout, stderr io.Writer, invoke func(context.Context, identra_v1_pb.ServiceAccountServiceClient, string) (proto.Message, error)) error {
	flags := flag.NewFlagSet("service-account "+command, flag.ContinueOnError)
	flags.SetOutput(stderr)
	client := addGRPCClientFlags(flags)
	clientID := flags.String("client-id", "", "target service-account client ID")
	tokenFile := flags.String("token-file", "", "file containing a service token; defaults to IDENTRA_SERVICE_TOKEN")
	if err := flags.Parse(args); err != nil {
		return err
	}
	var response proto.Message
	err := callServiceAccountRPC(flags, client, *tokenFile, func(ctx context.Context, api identra_v1_pb.ServiceAccountServiceClient) error {
		var err error
		response, err = invoke(ctx, api, strings.TrimSpace(*clientID))
		return err
	})
	if err != nil {
		return err
	}
	return writeProtoJSON(stdout, response)
}

func callServiceAccountRPC(flags *flag.FlagSet, client grpcClientFlags, tokenFile string, invoke func(context.Context, identra_v1_pb.ServiceAccountServiceClient) error) error {
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected arguments: %s", strings.Join(flags.Args(), " "))
	}
	token, err := readSecret(tokenFile, "IDENTRA_SERVICE_TOKEN")
	if err != nil {
		return err
	}
	conn, ctx, cancel, err := client.connect()
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()
	defer cancel()
	ctx = metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+token)
	if err := invoke(ctx, identra_v1_pb.NewServiceAccountServiceClient(conn)); err != nil {
		return fmt.Errorf("service-account RPC: %w", err)
	}
	return nil
}

func readSecret(path, environmentName string) (string, error) {
	if strings.TrimSpace(path) != "" {
		value, err := os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("read secret file: %w", err)
		}
		if secret := strings.TrimSpace(string(value)); secret != "" {
			return secret, nil
		}
		return "", errors.New("secret file is empty")
	}
	if secret := strings.TrimSpace(os.Getenv(environmentName)); secret != "" {
		return secret, nil
	}
	return "", fmt.Errorf("secret is required through --token-file/--client-secret-file or %s", environmentName)
}

func writeProtoJSON(w io.Writer, message proto.Message) error {
	value, err := (protojson.MarshalOptions{Indent: "  ", UseProtoNames: true}).Marshal(message)
	if err != nil {
		return fmt.Errorf("encode response: %w", err)
	}
	value = append(value, '\n')
	_, err = w.Write(value)
	return err
}
