package cert

import (
	"crypto/tls"
	"fmt"
	"os"
)

// ReadOrCreatePem reads a PEM file from the specified path, or generates a new one at that path if it does not exist.
func ReadOrCreatePem(path string, commonName string) (tls.Certificate, error) {
	data, err := func() ([]byte, error) {
		pemFile, err := os.ReadFile(path)
		if err == nil {
			return pemFile, nil
		}
		if !os.IsNotExist(err) {
			return nil, err
		}

		pemFile, err = GenSelfSignedPem(commonName)
		if err != nil {
			return nil, err
		}

		if err := os.WriteFile(path, pemFile, 0o600); err != nil {
			return nil, err
		}
		return pemFile, nil
	}()
	if err != nil {
		return tls.Certificate{}, err
	}

	keyPair, err := tls.X509KeyPair(data, data)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("failed to parse self-signed key pair at %q: %v", path, err)
	}

	return keyPair, nil
}
