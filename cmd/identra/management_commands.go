package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	identra_v1_pb "github.com/slhmy/identra/gen/go/identra/v1"
	"google.golang.org/grpc/metadata"
)

func runAuditCommand(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 || args[0] != "list" {
		return errors.New("usage: identra audit list [--page-size N] [--page-token TOKEN]")
	}
	flags := flag.NewFlagSet("audit list", flag.ContinueOnError)
	flags.SetOutput(stderr)
	client := addGRPCClientFlags(flags)
	tokenFile := flags.String("token-file", "", "file containing a service token; defaults to IDENTRA_SERVICE_TOKEN")
	pageSize := flags.Uint("page-size", 50, "number of audit events to return (maximum 200)")
	pageToken := flags.String("page-token", "", "pagination token returned by a previous request")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected arguments: %s", strings.Join(flags.Args(), " "))
	}
	token, err := readSecret(*tokenFile, "IDENTRA_SERVICE_TOKEN")
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
	response, err := identra_v1_pb.NewAuditServiceClient(conn).ListAuditEvents(ctx, &identra_v1_pb.ListAuditEventsRequest{
		PageSize: uint32(*pageSize), PageToken: strings.TrimSpace(*pageToken),
	})
	if err != nil {
		return fmt.Errorf("list audit events: %w", err)
	}
	return writeProtoJSON(stdout, response)
}

func runServerInfoCommand(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("server-info", flag.ContinueOnError)
	flags.SetOutput(stderr)
	client := addGRPCClientFlags(flags)
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected arguments: %s", strings.Join(flags.Args(), " "))
	}
	conn, ctx, cancel, err := client.connect()
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()
	defer cancel()
	response, err := identra_v1_pb.NewSystemServiceClient(conn).GetServerInfo(ctx, &identra_v1_pb.GetServerInfoRequest{})
	if err != nil {
		return fmt.Errorf("get server info: %w", err)
	}
	return writeProtoJSON(stdout, response)
}
