package crypto

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"os"
)

type ED25519Signer ed25519.PrivateKey

func (p ED25519Signer) Sign(rand io.Reader, message []byte, _ crypto.SignerOpts) ([]byte, error) {
	return ed25519.Sign(ed25519.PrivateKey(p), message), nil
}

func LoadPrivateKey(pkeyPEM []byte) (crypto.Signer, error) {
	var err error
	block, _ := pem.Decode(pkeyPEM)
	var pkey crypto.PrivateKey
	switch block.Type {
	case "PRIVATE KEY":
		pkey, err = x509.ParsePKCS8PrivateKey(block.Bytes)
	case "EC PRIVATE KEY":
		pkey, err = x509.ParseECPrivateKey(block.Bytes)
	case "RSA PRIVATE KEY":
		pkey, err = x509.ParsePKCS1PrivateKey(block.Bytes)
	}
	if err != nil {
		return nil, err
	}
	// ed25519 does not use SHA256 digest
	if eddie, ok := pkey.(ed25519.PrivateKey); ok {
		pkey = ED25519Signer(eddie)
	}
	privateKey, ok := pkey.(crypto.Signer)
	if !ok {
		return nil, fmt.Errorf("invalid private key type")
	}
	return privateKey, nil
}
