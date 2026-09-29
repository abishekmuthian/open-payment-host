package subscriptions

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"github.com/abishekmuthian/open-payment-host/src/lib/server/config"
	"io"
)

// Capabilities needed for webhook delivery are encrypted at rest. Only their
// hashes are used for authorization. The server secret must survive restarts.
func capabilityCipher() (cipher.AEAD, error) {
	secret := config.Get("secret_key")
	if len(secret) < 32 {
		return nil, errors.New("secret_key must contain at least 32 characters")
	}
	key := sha256.Sum256([]byte("oph-payment-capabilities-v1:" + secret))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}
func sealCapability(value string) (string, error) {
	c, err := capabilityCipher()
	if err != nil {
		return "", err
	}
	nonce := make([]byte, c.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(c.Seal(nonce, nonce, []byte(value), nil)), nil
}
func openCapability(value string) (string, error) {
	c, err := capabilityCipher()
	if err != nil {
		return "", err
	}
	b, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil || len(b) < c.NonceSize() {
		return "", errors.New("invalid encrypted capability")
	}
	b, err = c.Open(nil, b[:c.NonceSize()], b[c.NonceSize():], nil)
	return string(b), err
}
