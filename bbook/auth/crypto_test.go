package auth

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testSecret      = "test-secret-0123456789abcdef-0123456789abcdef"
	otherTestSecret = "other-secret-0123456789abcdef-0123456789abcdef"
)

// setTestSecret installs a session secret and re-derives the AEAD, restoring the previous state after the test.
func setTestSecret(t *testing.T, s string) {
	t.Helper()
	oldSecret, oldAead := secret, aead
	t.Cleanup(func() { secret, aead = oldSecret, oldAead })
	secret = []byte(s)
	require.NoError(t, initCrypto())
}

func TestEncryptDecryptRoundTrip(t *testing.T) {
	setTestSecret(t, testSecret)
	pt := []byte("refresh-token-value")
	ct, err := encrypt(pt)
	require.NoError(t, err)
	assert.NotEqual(t, pt, ct, "ciphertext should differ from plaintext")
	got, err := decrypt(ct)
	require.NoError(t, err)
	assert.Equal(t, pt, got)
}

func TestDecryptRejectsTamperedCiphertext(t *testing.T) {
	setTestSecret(t, testSecret)
	ct, err := encrypt([]byte("payload"))
	require.NoError(t, err)
	ct[len(ct)-1] ^= 0x01
	_, err = decrypt(ct)
	assert.Error(t, err)
}

func TestDecryptRejectsTruncatedInput(t *testing.T) {
	setTestSecret(t, testSecret)
	_, err := decrypt(make([]byte, aead.NonceSize()-1))
	assert.Error(t, err)
}

func TestDecryptRejectsDifferentKey(t *testing.T) {
	setTestSecret(t, testSecret)
	ct, err := encrypt([]byte("payload"))
	require.NoError(t, err)
	setTestSecret(t, otherTestSecret)
	_, err = decrypt(ct)
	assert.Error(t, err)
}
