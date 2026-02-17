package tls_test

import (
	cryptotls "crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/wolves-fc/tasker/lib/tls"
)

func generateCA(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	if err := tls.GenerateCA(dir); err != nil {
		t.Fatalf("GenerateCA: %v", err)
	}

	return dir
}

func parseCert(t *testing.T, path string) *x509.Certificate {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read cert: %v", err)
	}

	block, _ := pem.Decode(data)
	if block == nil {
		t.Fatal("no PEM block found")
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("parse cert: %v", err)
	}

	return cert
}

func TestGenerateCA(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		dir := generateCA(t)

		certPath := filepath.Join(dir, "ca.crt")
		keyPath := filepath.Join(dir, "ca.key")

		if _, err := os.Stat(certPath); err != nil {
			t.Fatalf("ca.crt not found: %v", err)
		}

		if _, err := os.Stat(keyPath); err != nil {
			t.Fatalf("ca.key not found: %v", err)
		}

		cert := parseCert(t, certPath)

		if cert.Subject.CommonName != "Tasker CA" {
			t.Errorf("CN (got=%q, want=%q)", cert.Subject.CommonName, "Tasker CA")
		}

		if len(cert.Subject.Organization) == 0 || cert.Subject.Organization[0] != tls.Organization {
			t.Errorf("Organization (got=%v, want=[%s])", cert.Subject.Organization, tls.Organization)
		}

		if !cert.IsCA {
			t.Error("IsCA (got=false, want=true)")
		}

		wantKeyUsage := x509.KeyUsageCertSign | x509.KeyUsageCRLSign
		if cert.KeyUsage != wantKeyUsage {
			t.Errorf("KeyUsage (got=%v, want=%v)", cert.KeyUsage, wantKeyUsage)
		}

		validYears := cert.NotAfter.Year() - cert.NotBefore.Year()
		if validYears != 10 {
			t.Errorf("validity (got=%d years, want=10)", validYears)
		}
	})

	t.Run("overwrite", func(t *testing.T) {
		t.Parallel()

		dir := generateCA(t)

		if err := tls.GenerateCA(dir); err == nil {
			t.Fatal("overwrite (got=nil, want=error)")
		}
	})
}

func TestHandshake(t *testing.T) {
	t.Parallel()

	dir := generateCA(t)
	if err := tls.GenerateServer(dir, "wolfpack1", []string{"127.0.0.1"}); err != nil {
		t.Fatalf("GenerateServer: %v", err)
	}

	if err := tls.GenerateClient(dir, "wolf", tls.RoleAdmin); err != nil {
		t.Fatalf("GenerateClient: %v", err)
	}

	clientCfg, err := tls.NewClientCfg(dir, "wolf")
	if err != nil {
		t.Fatalf("NewTLSClientCfg: %v", err)
	}

	clientCfg.ServerName = "127.0.0.1"

	serverCfg, err := tls.NewServerCfg(dir, "wolfpack1")
	if err != nil {
		t.Fatalf("NewTLSServerCfg: %v", err)
	}

	// Ephemeral port
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	defer listener.Close()

	// Accept connection, handshake and send back err if there is one
	errChan := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			errChan <- err
			return
		}

		defer conn.Close()

		tlsConn := cryptotls.Server(conn, serverCfg)
		errChan <- tlsConn.Handshake()
	}()

	conn, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}

	defer conn.Close()

	tlsConn := cryptotls.Client(conn, clientCfg)
	if err := tlsConn.Handshake(); err != nil {
		t.Fatalf("client handshake: %v", err)
	}

	if err := <-errChan; err != nil {
		t.Fatalf("server handshake: %v", err)
	}
}
