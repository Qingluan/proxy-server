package prosss

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
	"io"

	"golang.org/x/crypto/chacha20poly1305"
)

const (
	// Key sizes for different ciphers
	KeySizeAes256Gcm    = 32
	KeySizeChacha20Poly = 32
	// Nonce sizes
	NonceSizeAes256Gcm    = 12
	NonceSizeChacha20Poly = 12
)

// Cipher represents a tcp-encrypt cipher
type Cipher interface {
	KeySize() int
	NonceSize() int
	Encrypt(plaintext []byte) ([]byte, error)
	Decrypt(ciphertext []byte) ([]byte, error)
}

// Aes256GcmCipher implements AES-256-GCM encryption
type Aes256GcmCipher struct {
	key        []byte
	aead       cipher.AEAD
	nonceBytes []byte
}

// NewAes256GcmCipher creates a new AES-256-GCM cipher
func NewAes256GcmCipher(key []byte) (*Aes256GcmCipher, error) {
	if len(key) != KeySizeAes256Gcm {
		return nil, fmt.Errorf("aes-256-gcm requires exactly %d bytes key", KeySizeAes256Gcm)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	return &Aes256GcmCipher{
		key:        key,
		aead:       aead,
		nonceBytes: make([]byte, NonceSizeAes256Gcm),
	}, nil
}

func (c *Aes256GcmCipher) KeySize() int {
	return KeySizeAes256Gcm
}

func (c *Aes256GcmCipher) NonceSize() int {
	return NonceSizeAes256Gcm
}

func (c *Aes256GcmCipher) Encrypt(plaintext []byte) ([]byte, error) {
	// Generate random nonce
	if _, err := io.ReadFull(rand.Reader, c.nonceBytes); err != nil {
		return nil, err
	}

	// Seal encrypts and authenticates plaintext
	ciphertext := c.aead.Seal(c.nonceBytes, c.nonceBytes, plaintext, nil)
	return ciphertext, nil
}

func (c *Aes256GcmCipher) Decrypt(ciphertext []byte) ([]byte, error) {
	if len(ciphertext) < c.NonceSize() {
		return nil, errors.New("ciphertext too short")
	}

	nonce := ciphertext[:c.NonceSize()]
	encrypted := ciphertext[c.NonceSize():]

	// Open decrypts and authenticates ciphertext
	plaintext, err := c.aead.Open(nil, nonce, encrypted, nil)
	if err != nil {
		return nil, err
	}

	return plaintext, nil
}

// Chacha20Poly1305Cipher implements ChaCha20-Poly1305 encryption
type Chacha20Poly1305Cipher struct {
	key        []byte
	aead       cipher.AEAD
	nonceBytes []byte
}

// NewChacha20Poly1305Cipher creates a new ChaCha20-Poly1305 cipher
func NewChacha20Poly1305Cipher(key []byte) (*Chacha20Poly1305Cipher, error) {
	if len(key) != KeySizeChacha20Poly {
		return nil, fmt.Errorf("chacha20-poly1305 requires exactly %d bytes key", KeySizeChacha20Poly)
	}

	aead, err := chacha20poly1305.New(key)
	if err != nil {
		return nil, err
	}

	return &Chacha20Poly1305Cipher{
		key:        key,
		aead:       aead,
		nonceBytes: make([]byte, NonceSizeChacha20Poly),
	}, nil
}

func (c *Chacha20Poly1305Cipher) KeySize() int {
	return KeySizeChacha20Poly
}

func (c *Chacha20Poly1305Cipher) NonceSize() int {
	return NonceSizeChacha20Poly
}

func (c *Chacha20Poly1305Cipher) Encrypt(plaintext []byte) ([]byte, error) {
	// Generate random nonce
	if _, err := io.ReadFull(rand.Reader, c.nonceBytes); err != nil {
		return nil, err
	}

	// Seal encrypts and authenticates plaintext
	ciphertext := c.aead.Seal(c.nonceBytes, c.nonceBytes, plaintext, nil)
	return ciphertext, nil
}

func (c *Chacha20Poly1305Cipher) Decrypt(ciphertext []byte) ([]byte, error) {
	if len(ciphertext) < c.NonceSize() {
		return nil, errors.New("ciphertext too short")
	}

	nonce := ciphertext[:c.NonceSize()]
	encrypted := ciphertext[c.NonceSize():]

	// Open decrypts and authenticates ciphertext
	plaintext, err := c.aead.Open(nil, nonce, encrypted, nil)
	if err != nil {
		return nil, err
	}

	return plaintext, nil
}

// CipherInfo contains information about supported ciphers
type CipherInfo struct {
	Name      string
	KeySize   int
	NewCipher func(key []byte) (Cipher, error)
}

// SupportedCiphers returns a map of supported cipher methods
var SupportedCiphers = map[string]CipherInfo{
	"aes-256-gcm": {
		Name:    "aes-256-gcm",
		KeySize: KeySizeAes256Gcm,
		NewCipher: func(key []byte) (Cipher, error) {
			return NewAes256GcmCipher(key)
		},
	},
	"chacha20-poly1305": {
		Name:    "chacha20-poly1305",
		KeySize: KeySizeChacha20Poly,
		NewCipher: func(key []byte) (Cipher, error) {
			return NewChacha20Poly1305Cipher(key)
		},
	},
}
