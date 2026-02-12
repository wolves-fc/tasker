package rpc

import (
	"context"
	"fmt"
	"net"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/tap"

	taskerpb "github.com/wolves-fc/tasker/gen/proto/tasker"
	"github.com/wolves-fc/tasker/lib/tls"
)

// Serve starts a gRPC server with mutual TLS on the given address.
func Serve(svc taskerpb.TaskerServiceServer, certDir, name, addr string) error {
	tlsCfg, err := tls.NewServerCfg(certDir, name)
	if err != nil {
		return fmt.Errorf("load TLS server config: %w", err)
	}

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}

	// NOTE: Using tap.InTapHandle instead of unary/stream interceptors with a wrappedStream.
	// The tap package is experimental but avoids the boilerplate of wrapping ServerStream just to override Context().
	srv := grpc.NewServer(
		grpc.Creds(credentials.NewTLS(tlsCfg)),
		grpc.InTapHandle(authTapHandle),
	)

	taskerpb.RegisterTaskerServiceServer(srv, svc)

	return srv.Serve(listener)
}

// authTapHandle validates the peer's identity and injects it into the context before each RPC is handled.
func authTapHandle(ctx context.Context, _ *tap.Info) (context.Context, error) {
	p, exists := peer.FromContext(ctx)
	if !exists {
		return nil, status.Error(codes.Unauthenticated, "no peer info")
	}

	tlsInfo, exists := p.AuthInfo.(credentials.TLSInfo)
	if !exists {
		return nil, status.Error(codes.Unauthenticated, "missing TLS info")
	}

	identity, err := validatePeer(tlsInfo.State)
	if err != nil {
		return nil, status.Errorf(codes.Unauthenticated, "validate peer: %v", err)
	}

	if identity.Role != tls.RoleAdmin && identity.Role != tls.RoleUser {
		return nil, status.Errorf(codes.Unauthenticated, "invalid role (%s)", identity.Role)
	}

	return ContextWithIdentity(ctx, identity), nil
}
