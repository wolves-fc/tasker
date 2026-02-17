package rpc

import (
	"context"
	cryptotls "crypto/tls"
	"fmt"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/wolves-fc/tasker/lib/tls"
)

// Identity holds the authenticated peer's name and role.
type Identity struct {
	Name string
	Role tls.Role
}

type identityKey struct{}

// ContextWithIdentity returns a new context carrying the given Identity.
func ContextWithIdentity(ctx context.Context, identity Identity) context.Context {
	return context.WithValue(ctx, identityKey{}, identity)
}

// IdentityFromContext extracts the Identity from the context.
func IdentityFromContext(ctx context.Context) (Identity, error) {
	identity, exists := ctx.Value(identityKey{}).(Identity)
	if !exists {
		return Identity{}, status.Error(codes.Unauthenticated, "no identity in context")
	}

	return identity, nil
}

// validatePeer extracts and verifies the peer's Identity from a TLS connection.
func validatePeer(state cryptotls.ConnectionState) (Identity, error) {
	if len(state.PeerCertificates) == 0 {
		return Identity{}, fmt.Errorf("missing peer certificate")
	}

	cert := state.PeerCertificates[0]

	if len(cert.Subject.Organization) == 0 || cert.Subject.Organization[0] != tls.Organization {
		return Identity{}, fmt.Errorf("invalid organization (%v)", cert.Subject.Organization)
	}

	if cert.Subject.CommonName == "" {
		return Identity{}, fmt.Errorf("missing CN")
	}

	if len(cert.Subject.OrganizationalUnit) == 0 {
		return Identity{}, fmt.Errorf("missing role")
	}

	return Identity{
		Name: cert.Subject.CommonName,
		Role: tls.Role(cert.Subject.OrganizationalUnit[0]),
	}, nil
}
