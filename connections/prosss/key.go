package prosss

import (
	"crypto/sha256"
	"fmt"

	"golang.org/x/crypto/pbkdf2"
)

const (
	// KeyDerivationFunc is the key derivation function for tcp-encrypt AEAD ciphers
	// Using EVP_BytesToKey compatible derivation
	keyDerivationIterations = 1
)

// DeriveKey derives a key from the password using the specified method
func DeriveKey(password string, keySize int) []byte {
	// For AEAD ciphers, we use a simple key derivation
	// This is compatible with the tcp-encrypt AEAD key derivation
	key := pbkdf2.Key([]byte(password), []byte(password), keyDerivationIterations, keySize, sha256.New)
	return key
}

// DeriveKeyWithSalt derives a key from password and salt using PBKDF2
func DeriveKeyWithSalt(password, salt string, keySize int) []byte {
	key := pbkdf2.Key([]byte(password), []byte(salt), keyDerivationIterations, keySize, sha256.New)
	return key
}

// GetCipherInfo returns the cipher info for the given method
func GetCipherInfo(method string) (*CipherInfo, error) {
	if info, ok := SupportedCiphers[method]; ok {
		return &info, nil
	}
	return nil, fmt.Errorf("unsupported cipher method: %s", method)
}

// CreateCipher creates a cipher from password and method
func CreateCipher(password, method string) (Cipher, error) {
	info, err := GetCipherInfo(method)
	if err != nil {
		return nil, err
	}

	key := DeriveKey(password, info.KeySize)
	return info.NewCipher(key)
}
