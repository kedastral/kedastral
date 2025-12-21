// Package tls provides TLS configuration utilities for mutual TLS authentication.
//
// This package implements secure TLS 1.3 configurations for both server and client
// components with mutual authentication (mTLS). All configurations enforce:
//   - TLS 1.3 minimum version
//   - Secure cipher suites only (AES-GCM, ChaCha20-Poly1305)
//   - Mutual certificate verification
//   - Proper certificate validation
package tls

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"os"
)

// Config holds TLS certificate file paths for client or server configuration.
type Config struct {
	Enabled  bool
	CertFile string
	KeyFile  string
	CAFile   string
}

// Validate checks TLS configuration for security issues.
// Returns error if TLS is enabled but certificate files are missing or inaccessible.
func (c Config) Validate() error {
	if !c.Enabled {
		return nil
	}

	if c.CertFile == "" || c.KeyFile == "" || c.CAFile == "" {
		return errors.New("tls enabled but cert/key/ca files not specified")
	}

	for _, path := range []string{c.CertFile, c.KeyFile, c.CAFile} {
		if _, err := os.Stat(path); err != nil {
			return fmt.Errorf("tls file %q: %w", path, err)
		}
	}

	return nil
}

// NewServerTLSConfig creates a TLS configuration for HTTP/gRPC servers with mutual authentication.
// Requires client certificates to be verified against the provided CA certificate.
//
// Parameters:
//   - certFile: Server certificate file path (PEM format)
//   - keyFile: Server private key file path (PEM format)
//   - caFile: CA certificate file path for verifying client certificates (PEM format)
//
// Security features:
//   - TLS 1.3 minimum (rejects TLS 1.2 and below)
//   - Requires and verifies client certificates (mutual TLS)
//   - Secure cipher suites only
//   - Server cipher suite preference enabled
func NewServerTLSConfig(certFile, keyFile, caFile string) (*tls.Config, error) {
	if err := validateCertFiles(certFile, keyFile, caFile); err != nil {
		return nil, err
	}

	caCert, err := os.ReadFile(caFile)
	if err != nil {
		return nil, fmt.Errorf("read CA certificate: %w", err)
	}

	caCertPool := x509.NewCertPool()
	if !caCertPool.AppendCertsFromPEM(caCert) {
		return nil, errors.New("failed to parse CA certificate")
	}

	return &tls.Config{
		ClientCAs:  caCertPool,
		ClientAuth: tls.RequireAndVerifyClientCert,
		MinVersion: tls.VersionTLS13,
		CipherSuites: []uint16{
			tls.TLS_AES_128_GCM_SHA256,
			tls.TLS_AES_256_GCM_SHA384,
			tls.TLS_CHACHA20_POLY1305_SHA256,
		},
		PreferServerCipherSuites: true,
	}, nil
}

// NewClientTLSConfig creates a TLS configuration for HTTP/gRPC clients with mutual authentication.
// Presents client certificate and verifies server certificate against the provided CA.
//
// Parameters:
//   - certFile: Client certificate file path (PEM format)
//   - keyFile: Client private key file path (PEM format)
//   - caFile: CA certificate file path for verifying server certificate (PEM format)
//
// Security features:
//   - TLS 1.3 minimum (rejects TLS 1.2 and below)
//   - Presents client certificate for server verification
//   - Verifies server certificate against CA
//   - Secure cipher suites only
func NewClientTLSConfig(certFile, keyFile, caFile string) (*tls.Config, error) {
	if err := validateCertFiles(certFile, keyFile, caFile); err != nil {
		return nil, err
	}

	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, fmt.Errorf("load client certificate: %w", err)
	}

	caCert, err := os.ReadFile(caFile)
	if err != nil {
		return nil, fmt.Errorf("read CA certificate: %w", err)
	}

	caCertPool := x509.NewCertPool()
	if !caCertPool.AppendCertsFromPEM(caCert) {
		return nil, errors.New("failed to parse CA certificate")
	}

	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      caCertPool,
		MinVersion:   tls.VersionTLS13,
		CipherSuites: []uint16{
			tls.TLS_AES_128_GCM_SHA256,
			tls.TLS_AES_256_GCM_SHA384,
			tls.TLS_CHACHA20_POLY1305_SHA256,
		},
	}, nil
}

func validateCertFiles(certFile, keyFile, caFile string) error {
	if certFile == "" {
		return errors.New("certificate file path cannot be empty")
	}
	if keyFile == "" {
		return errors.New("key file path cannot be empty")
	}
	if caFile == "" {
		return errors.New("CA certificate file path cannot be empty")
	}

	for _, path := range []string{certFile, keyFile, caFile} {
		if _, err := os.Stat(path); err != nil {
			return fmt.Errorf("certificate file %q: %w", path, err)
		}
	}

	return nil
}
