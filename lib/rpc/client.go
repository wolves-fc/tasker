package rpc

import (
	"context"
	"fmt"
	"net"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	taskerpb "github.com/wolves-fc/tasker/gen/proto/tasker"
	"github.com/wolves-fc/tasker/lib/tls"
)

// Client wraps a gRPC connection and exposes the TaskerService client.
type Client struct {
	*grpc.ClientConn
	peerCreds *peerCredentials
	Tasker    taskerpb.TaskerServiceClient
}

// PeerIdentity returns the peer's Identity.
func (c *Client) PeerIdentity() Identity {
	return c.peerCreds.identity
}

// Dial creates a new connection to the given server address with mutual TLS.
func Dial(certDir, user, addr string) (*Client, error) {
	tlsCfg, err := tls.NewClientCfg(certDir, user)
	if err != nil {
		return nil, fmt.Errorf("load TLS client config: %w", err)
	}

	peerCreds := &peerCredentials{
		TransportCredentials: credentials.NewTLS(tlsCfg),
	}

	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(peerCreds))
	if err != nil {
		return nil, fmt.Errorf("dial %s: %w", addr, err)
	}

	return &Client{
		ClientConn: conn,
		peerCreds:  peerCreds,
		Tasker:     taskerpb.NewTaskerServiceClient(conn),
	}, nil
}

// peerCredentials wraps TransportCredentials to validate and extract peer identity during the TLS handshake.
type peerCredentials struct {
	credentials.TransportCredentials
	identity Identity
}

// ClientHandshake extracts Identity from the peer during the TLS handshake.
func (creds *peerCredentials) ClientHandshake(
	ctx context.Context,
	authority string,
	rawConn net.Conn,
) (net.Conn, credentials.AuthInfo, error) {
	conn, auth, err := creds.TransportCredentials.ClientHandshake(ctx, authority, rawConn)
	if err != nil {
		return conn, auth, err
	}

	tlsInfo, ok := auth.(credentials.TLSInfo)
	if !ok {
		return nil, nil, fmt.Errorf("missing TLS info")
	}

	identity, err := validatePeer(tlsInfo.State)
	if err != nil {
		return nil, nil, fmt.Errorf("validate peer: %w", err)
	}

	if identity.Role != tls.RoleServer {
		return nil, nil, fmt.Errorf("invalid role (%s)", identity.Role)
	}

	creds.identity = identity
	return conn, auth, nil
}
