package tls

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	cryptotls "crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
)

// Organization is the expected Organization field in all Tasker certificates.
const Organization = "Tasker"

// Role represents a certificate role stored in the OrganizationalUnit field.
type Role string

const (
	RoleAdmin  Role = "admin"
	RoleUser   Role = "user"
	RoleServer Role = "server"
)

// GenerateCA creates a new CA certificate and private key in dir.
//
// CA cert/key will get written to the directory provided.
func GenerateCA(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create CA directory: %w", err)
	}

	certPath := filepath.Join(dir, "ca.crt")
	keyPath := filepath.Join(dir, "ca.key")

	if _, err := os.Stat(certPath); err == nil {
		return fmt.Errorf("CA cert already exists (cert=%s)", certPath)
	}

	if _, err := os.Stat(keyPath); err == nil {
		return fmt.Errorf("CA key already exists (key=%s)", keyPath)
	}

	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return fmt.Errorf("generate key: %w", err)
	}

	// 10 Years, CRLs probably don't matter here, no intermediate CA
	template := x509.Certificate{
		SerialNumber: generateSerialNumber(),
		Subject: pkix.Name{
			CommonName:   "Tasker CA",
			Organization: []string{Organization},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().AddDate(10, 0, 0),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return fmt.Errorf("create certificate: %w", err)
	}

	if err := writeCertAndKey(certPath, keyPath, certDER, privateKey); err != nil {
		return err
	}

	return nil
}

// loadCA reads the CA certificate and private key.
func loadCA(dir string) (*x509.Certificate, *ecdsa.PrivateKey, error) {
	certPath := filepath.Join(dir, "ca.crt")
	keyPath := filepath.Join(dir, "ca.key")

	tlsCert, err := cryptotls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return nil, nil, fmt.Errorf("load CA keypair: %w", err)
	}

	// tlsCert.Certificate[0] is the certDER
	caCert, err := x509.ParseCertificate(tlsCert.Certificate[0])
	if err != nil {
		return nil, nil, fmt.Errorf("parse ca.crt: %w", err)
	}

	caKey, matches := tlsCert.PrivateKey.(*ecdsa.PrivateKey)
	if !matches {
		return nil, nil, fmt.Errorf("ca.key is not an ECDSA key")
	}

	return caCert, caKey, nil
}

// writeCertAndKey writes a certificate and private key to .crt and .key files.
//
// In production, key files should be 0o600 to restrict access.
func writeCertAndKey(certPath, keyPath string, certDER []byte, key *ecdsa.PrivateKey) error {
	// Convert cert from DER to PEM
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	if err := os.WriteFile(certPath, certPEM, 0o644); err != nil {
		return fmt.Errorf("write cert: %w", err)
	}

	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		os.Remove(certPath)
		return fmt.Errorf("marshal PKCS8 key: %w", err)
	}

	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
	if err := os.WriteFile(keyPath, keyPEM, 0o644); err != nil {
		os.Remove(certPath)
		return fmt.Errorf("write key: %w", err)
	}

	return nil
}

// generateSerialNumber generates a 128 bit serial number from a UUIDv7.
func generateSerialNumber() *big.Int {
	id := uuid.Must(uuid.NewV7())
	return new(big.Int).SetBytes(id[:])
}
