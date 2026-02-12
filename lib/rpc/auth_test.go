package rpc

import (
	cryptotls "crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"testing"

	"github.com/wolves-fc/tasker/lib/tls"
)

func newCert(t *testing.T, cn string, org []string, ou []string) *x509.Certificate {
	t.Helper()

	return &x509.Certificate{
		Subject: pkix.Name{
			CommonName:         cn,
			Organization:       org,
			OrganizationalUnit: ou,
		},
	}
}

func TestValidatePeer(t *testing.T) {
	t.Parallel()

	t.Run("valid_admin", func(t *testing.T) {
		t.Parallel()

		state := cryptotls.ConnectionState{
			PeerCertificates: []*x509.Certificate{
				newCert(t, "wolf", []string{tls.Organization}, []string{string(tls.RoleAdmin)}),
			},
		}

		id, err := validatePeer(state)
		if err != nil {
			t.Fatalf("validatePeer: %v", err)
		}

		if id.Name != "wolf" {
			t.Errorf("Name (got=%q, want=%q)", id.Name, "wolf")
		}

		if id.Role != tls.RoleAdmin {
			t.Errorf("Role (got=%q, want=%q)", id.Role, tls.RoleAdmin)
		}
	})

	t.Run("valid_user", func(t *testing.T) {
		t.Parallel()

		state := cryptotls.ConnectionState{
			PeerCertificates: []*x509.Certificate{
				newCert(t, "wolfjr", []string{tls.Organization}, []string{string(tls.RoleUser)}),
			},
		}

		id, err := validatePeer(state)
		if err != nil {
			t.Fatalf("validatePeer: %v", err)
		}

		if id.Name != "wolfjr" {
			t.Errorf("Name (got=%q, want=%q)", id.Name, "wolfjr")
		}

		if id.Role != tls.RoleUser {
			t.Errorf("Role (got=%q, want=%q)", id.Role, tls.RoleUser)
		}
	})

	t.Run("valid_server", func(t *testing.T) {
		t.Parallel()

		state := cryptotls.ConnectionState{
			PeerCertificates: []*x509.Certificate{
				newCert(t, "wolfpack1", []string{tls.Organization}, []string{string(tls.RoleServer)}),
			},
		}

		id, err := validatePeer(state)
		if err != nil {
			t.Fatalf("validatePeer: %v", err)
		}

		if id.Name != "wolfpack1" {
			t.Errorf("Name (got=%q, want=%q)", id.Name, "wolfpack1")
		}

		if id.Role != tls.RoleServer {
			t.Errorf("Role (got=%q, want=%q)", id.Role, tls.RoleServer)
		}
	})

	t.Run("no_peer_certs", func(t *testing.T) {
		t.Parallel()

		state := cryptotls.ConnectionState{}

		if _, err := validatePeer(state); err == nil {
			t.Fatal("no peer certs (got=nil, want=error)")
		}
	})

	t.Run("wrong_organization", func(t *testing.T) {
		t.Parallel()

		state := cryptotls.ConnectionState{
			PeerCertificates: []*x509.Certificate{
				newCert(t, "wolf", []string{"Other"}, []string{string(tls.RoleAdmin)}),
			},
		}

		if _, err := validatePeer(state); err == nil {
			t.Fatal("wrong organization (got=nil, want=error)")
		}
	})

	t.Run("empty_organization", func(t *testing.T) {
		t.Parallel()

		state := cryptotls.ConnectionState{
			PeerCertificates: []*x509.Certificate{
				newCert(t, "wolf", nil, []string{string(tls.RoleAdmin)}),
			},
		}

		if _, err := validatePeer(state); err == nil {
			t.Fatal("empty organization (got=nil, want=error)")
		}
	})

	t.Run("empty_cn", func(t *testing.T) {
		t.Parallel()

		state := cryptotls.ConnectionState{
			PeerCertificates: []*x509.Certificate{
				newCert(t, "", []string{tls.Organization}, []string{string(tls.RoleAdmin)}),
			},
		}

		if _, err := validatePeer(state); err == nil {
			t.Fatal("empty CN (got=nil, want=error)")
		}
	})

	t.Run("empty_ou", func(t *testing.T) {
		t.Parallel()

		state := cryptotls.ConnectionState{
			PeerCertificates: []*x509.Certificate{
				newCert(t, "wolf", []string{tls.Organization}, nil),
			},
		}

		if _, err := validatePeer(state); err == nil {
			t.Fatal("empty OU (got=nil, want=error)")
		}
	})
}
