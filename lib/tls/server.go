package tls

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"
)

// GenerateServer creates a new server certificate signed by the CA.
//
//	CA cert/key must be located in <dir>.
//	Server cert/key will get written to <dir>/server/.
func GenerateServer(dir, name string, hosts []string) error {
	if name == "" {
		return fmt.Errorf("name is required")
	}

	if len(hosts) == 0 {
		return fmt.Errorf("at least one host is required")
	}

	caCert, caKey, err := loadCA(dir)
	if err != nil {
		return fmt.Errorf("load CA: %w", err)
	}

	serverDir := filepath.Join(dir, "server")
	if err := os.MkdirAll(serverDir, 0o755); err != nil {
		return fmt.Errorf("create server directory: %w", err)
	}

	certPath := filepath.Join(serverDir, name+".crt")
	keyPath := filepath.Join(serverDir, name+".key")

	if _, err := os.Stat(certPath); err == nil {
		return fmt.Errorf("server cert already exists (cert=%s)", certPath)
	}

	if _, err := os.Stat(keyPath); err == nil {
		return fmt.Errorf("server key already exists (key=%s)", keyPath)
	}

	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return fmt.Errorf("generate key: %w", err)
	}

	template := x509.Certificate{
		SerialNumber: generateSerialNumber(),
		Subject: pkix.Name{
			CommonName:         name,
			Organization:       []string{Organization},
			OrganizationalUnit: []string{string(RoleServer)},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().AddDate(1, 0, 0),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	// Filter IPs and hostnames
	for _, host := range hosts {
		if ip := net.ParseIP(host); ip != nil {
			template.IPAddresses = append(template.IPAddresses, ip)
		} else {
			template.DNSNames = append(template.DNSNames, host)
		}
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

// NewServerCfg builds a mutual TLS server config.
//
//	CA cert must be located in <dir>.
//	Server cert/key must be located in <dir>/server/.
func NewServerCfg(dir, name string) (*tls.Config, error) {
	caCertPath := filepath.Join(dir, "ca.crt")
	serverCertPath := filepath.Join(dir, "server", name+".crt")
	serverKeyPath := filepath.Join(dir, "server", name+".key")

	cert, err := tls.LoadX509KeyPair(serverCertPath, serverKeyPath)
	if err != nil {
		return nil, fmt.Errorf("load server cert: %w", err)
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
		ClientCAs:    caPool,
		ClientAuth:   tls.RequireAndVerifyClientCert,
		MinVersion:   tls.VersionTLS13,
	}, nil
}
