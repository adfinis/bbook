package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
)

var aead cipher.AEAD


func initCrypto() error {
	key := sha256.Sum256(secret)
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return fmt.Errorf("initializing crypto: cipher: %w", err)
	}
	a, err := cipher.NewGCM(block)
	if err != nil {
		return fmt.Errorf("initializing crypto: gcm: %w", err)
	}
	aead = a
	return nil
}

// Encrypt using SESSION SECRET
func encrypt(plaintext []byte) ([]byte, error) {
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("encrypting: nonce: %w", err)
	}
	return aead.Seal(nonce, nonce, plaintext, nil), nil
}

// Decrypt using SESSION SECRET
func decrypt(ciphertext []byte) ([]byte, error) {
	if len(ciphertext) < aead.NonceSize() {
		return nil, errors.New("decrypting: ciphertext too short")
	}
	nonce, sealed := ciphertext[:aead.NonceSize()], ciphertext[aead.NonceSize():]
	pt, err := aead.Open(nil, nonce, sealed, nil)
	if err != nil {
		return nil, fmt.Errorf("decrypting: open: %w", err)
	}
	return pt, nil
}
