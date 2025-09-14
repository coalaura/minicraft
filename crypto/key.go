package crypto

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
)

type KeyPair struct {
	Private *rsa.PrivateKey
	Public  []byte
}

func CreateKeyPair() (*KeyPair, error) {
	key, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		return nil, err
	}

	pub, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		return nil, err
	}

	return &KeyPair{
		Private: key,
		Public:  pub,
	}, nil
}
