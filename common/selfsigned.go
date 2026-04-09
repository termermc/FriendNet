package common

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"time"
)

// GenSelfSignedPem generates a self-signed certificate and returns both the
// certificate and PKCS#8 private key as a single PEM file's bytes.
//
// If makeBrowserCompatible is true, it uses an algorithm that is broadly
// compatible with browsers (ECDSA P-256). If false, it uses Ed25519.
//
// The output can be converted to a tls.Certificate with tls.X509KeyPair(bytes, bytes).
func GenSelfSignedPem(
	commonName string,
	makeBrowserCompatible bool,
) ([]byte, error) {
	// Generate keypair.
	var pub crypto.PublicKey
	var priv crypto.PrivateKey
	var err error

	if makeBrowserCompatible {
		var ecdsaPriv *ecdsa.PrivateKey
		ecdsaPriv, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			return nil, err
		}
		priv = ecdsaPriv
		pub = ecdsaPriv.Public()
	} else {
		var edPriv ed25519.PrivateKey
		_, edPriv, err = ed25519.GenerateKey(rand.Reader)
		if err != nil {
			return nil, err
		}
		priv = edPriv
		pub = edPriv.Public()
	}

	// Random 128-bit serial
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, err
	}

	notBefore := time.Now().Add(-1 * time.Minute)
	notAfter := notBefore.Add(365 * 24 * time.Hour)

	tpl := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName: commonName,
		},
		NotBefore: notBefore,
		NotAfter:  notAfter,

		// For TLS server use; adjust as needed.
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  false,

		// Browsers expect SAN for host validation; without it you may get a
		// different cert error. Keep it minimal and let callers expand.
		DNSNames: []string{commonName},
	}

	// Self-sign.
	derCert, err := x509.CreateCertificate(rand.Reader, tpl, tpl, pub, priv)
	if err != nil {
		return nil, err
	}

	// Marshal private key as PKCS#8.
	pkcs8, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return nil, err
	}

	// Build PEM.
	var pemBuf []byte
	pemBuf = append(
		pemBuf,
		pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: derCert})...,
	)
	pemBuf = append(
		pemBuf,
		pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pkcs8})...,
	)

	return pemBuf, nil
}
