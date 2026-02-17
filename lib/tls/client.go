package tls

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// GenerateClient creates a new client certificate signed by the CA.
//
//	CA cert/key must be located in <dir>.
//	Client cert/key will get written to <dir>/client/.
func GenerateClient(dir, name string, role Role) error {
	if name == "" {
		return fmt.Errorf("name is required")
	}

	if role != RoleAdmin && role != RoleUser {
		return fmt.Errorf("role must be 'admin' or 'user' (role=%s)", role)
	}

	caCert, caKey, err := loadCA(dir)
	if err != nil {
		return fmt.Errorf("load CA: %w", err)
	}

	clientDir := filepath.Join(dir, "client")
	if err := os.MkdirAll(clientDir, 0o755); err != nil {
		return fmt.Errorf("create client directory: %w", err)
	}

	certPath := filepath.Join(clientDir, name+".crt")
	keyPath := filepath.Join(clientDir, name+".key")

	if _, err := os.Stat(certPath); err == nil {
		return fmt.Errorf("client cert already exists (cert=%s)", certPath)
	}

	if _, err := os.Stat(keyPath); err == nil {
		return fmt.Errorf("client key already exists (key=%s)", keyPath)
	}

	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return fmt.Errorf("generate key: %w", err)
	}

	// 1 Year, added in role for targeted responses
	template := x509.Certificate{
		SerialNumber: generateSerialNumber(),
		Subject: pkix.Name{
			CommonName:         name,
			Organization:       []string{Organization},
			OrganizationalUnit: []string{string(role)},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().AddDate(1, 0, 0),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, &template, caCert, &privateKey.PublicKey, caKey)
	if err != nil {
		return fmt.Errorf("create certificate: %w", err)
	}

	if err := writeCertAndKey(certPath, keyPath, certDER, privateKey); err != nil {
		return err
	}

	return nil
}

// NewClientCfg builds a mutual TLS client config.
//
//	CA cert must be located in <dir>.
//	Client cert/key must be located in <dir>/client/.
func NewClientCfg(dir, user string) (*tls.Config, error) {
	caCertPath := filepath.Join(dir, "ca.crt")
	clientCertPath := filepath.Join(dir, "client", user+".crt")
	clientKeyPath := filepath.Join(dir, "client", user+".key")

	cert, err := tls.LoadX509KeyPair(clientCertPath, clientKeyPath)
	if err != nil {
		return nil, fmt.Errorf("load client cert: %w", err)
	}

	caCert, err := os.ReadFile(caCertPath)
	if err != nil {
		return nil, fmt.Errorf("read CA cert: %w", err)
	}

	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM(caCert) {
		return nil, fmt.Errorf("failed to parse CA cert")
	}

	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      caPool,
		MinVersion:   tls.VersionTLS13,
	}, nil
}
