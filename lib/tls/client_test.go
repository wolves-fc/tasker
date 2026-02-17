package tls_test

import (
	"crypto/x509"
	"os"
	"path/filepath"
	"testing"

	"github.com/wolves-fc/tasker/lib/tls"
)

func TestGenerateClient(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		dir := generateCA(t)

		if err := tls.GenerateClient(dir, "wolf", tls.RoleAdmin); err != nil {
			t.Fatalf("GenerateClient: %v", err)
		}

		certPath := filepath.Join(dir, "client", "wolf.crt")
		keyPath := filepath.Join(dir, "client", "wolf.key")

		if _, err := os.Stat(certPath); err != nil {
			t.Fatalf("client cert not found: %v", err)
		}

		if _, err := os.Stat(keyPath); err != nil {
			t.Fatalf("client key not found: %v", err)
		}

		cert := parseCert(t, certPath)

		if cert.Subject.CommonName != "wolf" {
			t.Errorf("CN (got=%q, want=%q)", cert.Subject.CommonName, "wolf")
		}

		if len(cert.Subject.Organization) == 0 || cert.Subject.Organization[0] != tls.Organization {
			t.Errorf("Organization (got=%v, want=[%s])", cert.Subject.Organization, tls.Organization)
		}

		if len(cert.Subject.OrganizationalUnit) == 0 || cert.Subject.OrganizationalUnit[0] != string(tls.RoleAdmin) {
			t.Errorf("OU (got=%v, want=[%s])", cert.Subject.OrganizationalUnit, tls.RoleAdmin)
		}

		if cert.IsCA {
			t.Error("IsCA (got=true, want=false)")
		}

		if cert.KeyUsage != x509.KeyUsageDigitalSignature {
			t.Errorf("KeyUsage (got=%v, want=%v)", cert.KeyUsage, x509.KeyUsageDigitalSignature)
		}

		if cert.ExtKeyUsage[0] != x509.ExtKeyUsageClientAuth {
			t.Errorf("ExtKeyUsage (got=%v, want=ClientAuth)", cert.ExtKeyUsage[0])
		}

		validYears := cert.NotAfter.Year() - cert.NotBefore.Year()
		if validYears != 1 {
			t.Errorf("validity (got=%d years, want=1)", validYears)
		}
	})

	t.Run("invalid_role", func(t *testing.T) {
		t.Parallel()

		dir := generateCA(t)

		if err := tls.GenerateClient(dir, "wolf", tls.RoleServer); err == nil {
			t.Fatal("invalid role (got=nil, want=error)")
		}
	})

	t.Run("missing_ca", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()

		if err := tls.GenerateClient(dir, "wolf", tls.RoleAdmin); err == nil {
			t.Fatal("missing CA (got=nil, want=error)")
		}
	})

	t.Run("empty_name", func(t *testing.T) {
		t.Parallel()

		dir := generateCA(t)

		if err := tls.GenerateClient(dir, "", tls.RoleAdmin); err == nil {
			t.Fatal("empty name (got=nil, want=error)")
		}
	})

	t.Run("overwrite", func(t *testing.T) {
		t.Parallel()

		dir := generateCA(t)

		if err := tls.GenerateClient(dir, "wolf", tls.RoleAdmin); err != nil {
			t.Fatalf("GenerateClient: %v", err)
		}

		if err := tls.GenerateClient(dir, "wolf", tls.RoleAdmin); err == nil {
			t.Fatal("overwrite (got=nil, want=error)")
		}
	})
}

func TestNewClientCfg(t *testing.T) {
	t.Parallel()

	dir := generateCA(t)
	if err := tls.GenerateClient(dir, "wolf", tls.RoleAdmin); err != nil {
		t.Fatalf("GenerateClient: %v", err)
	}

	cfg, err := tls.NewClientCfg(dir, "wolf")
	if err != nil {
		t.Fatalf("NewClientCfg: %v", err)
	}

	if len(cfg.Certificates) != 1 {
		t.Fatalf("Certificates count (got=%d, want=1)", len(cfg.Certificates))
	}

	if cfg.RootCAs == nil {
		t.Fatal("RootCAs (got=nil, want=non-nil)")
	}
}
