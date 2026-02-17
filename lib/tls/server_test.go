package tls_test

import (
	"crypto/x509"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/wolves-fc/tasker/lib/tls"
)

func TestGenerateServer(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		dir := generateCA(t)

		if err := tls.GenerateServer(dir, "wolfpack1", []string{"localhost", "127.0.0.1", "::1"}); err != nil {
			t.Fatalf("GenerateServer: %v", err)
		}

		certPath := filepath.Join(dir, "server", "wolfpack1.crt")
		keyPath := filepath.Join(dir, "server", "wolfpack1.key")

		if _, err := os.Stat(certPath); err != nil {
			t.Fatalf("server cert not found: %v", err)
		}

		if _, err := os.Stat(keyPath); err != nil {
			t.Fatalf("server key not found: %v", err)
		}

		cert := parseCert(t, certPath)

		if cert.Subject.CommonName != "wolfpack1" {
			t.Errorf("CN (got=%q, want=%q)", cert.Subject.CommonName, "wolfpack1")
		}

		if len(cert.Subject.Organization) == 0 || cert.Subject.Organization[0] != tls.Organization {
			t.Errorf("Organization (got=%v, want=[%s])", cert.Subject.Organization, tls.Organization)
		}

		if len(cert.Subject.OrganizationalUnit) == 0 || cert.Subject.OrganizationalUnit[0] != string(tls.RoleServer) {
			t.Errorf("OU (got=%v, want=[%s])", cert.Subject.OrganizationalUnit, tls.RoleServer)
		}

		if cert.IsCA {
			t.Error("IsCA (got=true, want=false)")
		}

		if cert.KeyUsage != x509.KeyUsageDigitalSignature {
			t.Errorf("KeyUsage (got=%v, want=%v)", cert.KeyUsage, x509.KeyUsageDigitalSignature)
		}

		if cert.ExtKeyUsage[0] != x509.ExtKeyUsageServerAuth {
			t.Errorf("ExtKeyUsage (got=%v, want=ServerAuth)", cert.ExtKeyUsage[0])
		}

		validYears := cert.NotAfter.Year() - cert.NotBefore.Year()
		if validYears != 1 {
			t.Errorf("validity (got=%d years, want=1)", validYears)
		}

		if len(cert.DNSNames) != 1 || cert.DNSNames[0] != "localhost" {
			t.Errorf("DNSNames (got=%v, want=[localhost])", cert.DNSNames)
		}

		wantIPs := []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")}
		if len(cert.IPAddresses) != len(wantIPs) {
			t.Fatalf("IPAddresses count (got=%d, want=%d)", len(cert.IPAddresses), len(wantIPs))
		}

		for i, ip := range cert.IPAddresses {
			if !ip.Equal(wantIPs[i]) {
				t.Errorf("IPAddresses[%d] (got=%v, want=%v)", i, ip, wantIPs[i])
			}
		}
	})

	t.Run("missing_ca", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()

		if err := tls.GenerateServer(dir, "wolfpack1", []string{"localhost"}); err == nil {
			t.Fatal("missing CA (got=nil, want=error)")
		}
	})

	t.Run("empty_name", func(t *testing.T) {
		t.Parallel()

		dir := generateCA(t)

		if err := tls.GenerateServer(dir, "", []string{"localhost"}); err == nil {
			t.Fatal("empty name (got=nil, want=error)")
		}
	})

	t.Run("no_hosts", func(t *testing.T) {
		t.Parallel()

		dir := generateCA(t)

		if err := tls.GenerateServer(dir, "wolfpack1", nil); err == nil {
			t.Fatal("no hosts (got=nil, want=error)")
		}
	})

	t.Run("overwrite", func(t *testing.T) {
		t.Parallel()

		dir := generateCA(t)

		if err := tls.GenerateServer(dir, "wolfpack1", []string{"localhost"}); err != nil {
			t.Fatalf("GenerateServer: %v", err)
		}

		if err := tls.GenerateServer(dir, "wolfpack1", []string{"localhost"}); err == nil {
			t.Fatal("overwrite (got=nil, want=error)")
		}
	})
}

func TestNewServerCfg(t *testing.T) {
	t.Parallel()

	dir := generateCA(t)
	if err := tls.GenerateServer(dir, "wolfpack1", []string{"localhost"}); err != nil {
		t.Fatalf("GenerateServer: %v", err)
	}

	cfg, err := tls.NewServerCfg(dir, "wolfpack1")
	if err != nil {
		t.Fatalf("NewTLSServerCfg: %v", err)
	}

	if len(cfg.Certificates) != 1 {
		t.Fatalf("Certificates count (got=%d, want=1)", len(cfg.Certificates))
	}

	if cfg.ClientCAs == nil {
		t.Fatal("ClientCAs (got=nil, want=non-nil)")
	}
}
