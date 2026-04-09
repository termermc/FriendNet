package cert

import (
	"crypto/tls"
	"fmt"
	"os"

	"friendnet.org/common"
)

// ReadFullChainPem reads a PEM file from the specified path.
// It treats it as a full-chain PEM, aka one containing a X509 keypair.
func ReadFullChainPem(path string) (tls.Certificate, error) {
	pemFile, err := os.ReadFile(path)
	if err != nil {
		return tls.Certificate{}, err
	}

	keyPair, err := tls.X509KeyPair(pemFile, pemFile)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("failed to parse key pair from PEM file at %q: %v", path, err)
	}
	return keyPair, nil
}

// ReadOrCreatePem reads a PEM file from the specified path, or generates a new one at that path if it does not exist.
//
// If makeBrowserCompatible is true, it uses an algorithm that is broadly
// compatible with browsers (ECDSA P-256). If false, it uses Ed25519.
func ReadOrCreatePem(path string, commonName string, makeBrowserCompatible bool) (tls.Certificate, error) {
	cert, err := ReadFullChainPem(path)
	if err == nil {
		return cert, nil
	}
	if !os.IsNotExist(err) {
		return tls.Certificate{}, fmt.Errorf("failed to read PEM file at %q: %w", path, err)
	}

	raw, err := common.GenSelfSignedPem(commonName, makeBrowserCompatible)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("failed to generate self-signed certificate: %w", err)
	}

	if err = os.WriteFile(path, raw, 0o600); err != nil {
		return tls.Certificate{}, err
	}

	return tls.X509KeyPair(raw, raw)
}
