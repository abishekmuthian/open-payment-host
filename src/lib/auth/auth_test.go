package auth

import (
	"bytes"
	"compress/flate"
	"testing"
)

// TestEncryptDecryptGCM tests encrypt.go
func TestEncryptDecryptGCM(t *testing.T) {
	key := RandomToken(32)
	plaintext := []byte("Hello, world!")

	ciphertext, err := Encrypt(plaintext, key)
	if err != nil {
		t.Fatal(err)
	}

	decrypted, err := Decrypt(ciphertext, key)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(decrypted, plaintext) {
		t.Errorf("plaintexts don't match")
	}

	ciphertext[0] ^= 0xff
	plaintext, err = Decrypt(ciphertext, key)
	if err == nil {
		t.Errorf("gcmOpen should not have worked, but did")
	}

}

// TestAuthenticityToken tests our authenticity token is set
func TestAuthenticityToken(t *testing.T) {
	secret := RandomToken(32)
	token := AuthenticityTokenWithSecret(secret)
	err := CheckAuthenticityTokenWithSecret(token, secret)
	if err != nil {
		t.Error(err)
	}
	err = CheckAuthenticityTokenWithSecret(token, []byte("bogus"))
	if err == nil {
		t.Fatalf("matched authenticity token against bogus secret")
	}
}

// TestRandom tests random token generation
func TestRandom(t *testing.T) {
	lens := []int{65, 144, 32}

	// Test lengths generated
	for _, l := range lens {
		b := RandomToken(l)
		if len(b) != l {
			t.Fatalf("random bytes wrong length expected:%d got:%d", l, len(b))
		}
	}

	// Test compression
	b := RandomToken(1e5)
	var z bytes.Buffer
	f, _ := flate.NewWriter(&z, 5)
	f.Write(b)
	f.Close()
	// Any compression is a bad sign, we expect longer
	//if z.Len() < len(b)*99/100 {
	if z.Len() < len(b) {
		t.Fatalf("Random data compressed too much %d -> %d", len(b), z.Len())
	}

}

// TestHashPassword tests password hashing with bcrypt
func TestHashPassword(t *testing.T) {
	password := "Hunter2"
	hash := "$2a$10$2IUzpI/yH0Xc.qs9Z5UUL.3f9bqi0ThvbKs6Q91UOlyCEGY8hdBw6"

	newHash, err := HashPassword(password)
	if err != nil {
		t.Fatal(err)
	}

	if err = CheckPassword(password, newHash); err != nil {
		t.Fatal(err)
	}

	if err = CheckPassword(password, hash); err != nil {
		t.Fatal(err)
	}

	if err = CheckPassword(password+"2", newHash); err == nil {
		t.Fatal("error unmatched hashes match")
	}

}
