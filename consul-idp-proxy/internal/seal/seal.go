// Package seal encrypts and authenticates cookie values with AES-256-GCM, so the proxy keeps no
// state: everything it needs between requests travels in the browser cookies.
package seal

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"time"
)

// ErrInvalid means the value was not produced by this key, was tampered with or has expired.
var ErrInvalid = errors.New("invalid or expired sealed value")

// Sealer seals values bound to a name (the cookie name), so a value of one cookie cannot be
// replayed as another one.
type Sealer struct {
	aead cipher.AEAD
	now  func() time.Time
}

// New derives the encryption key from the secret.
func New(secret []byte) (*Sealer, error) {
	key, err := hkdf.Key(sha256.New, secret, nil, "consul-idp-proxy cookie v1", 32)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Sealer{aead: aead, now: time.Now}, nil
}

type envelope struct {
	Exp int64           `json:"e"`
	V   json.RawMessage `json:"v"`
}

// Seal encodes v as JSON, adds the expiry and encrypts it.
func (s *Sealer) Seal(name string, v any, expires time.Time) (string, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	plain, _ := json.Marshal(envelope{Exp: expires.Unix(), V: raw})
	nonce := make([]byte, s.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	out := s.aead.Seal(nonce, nonce, plain, []byte(name))
	return base64.RawURLEncoding.EncodeToString(out), nil
}

// Open decrypts a value sealed under name into v.
func (s *Sealer) Open(name, value string, v any) error {
	b, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil || len(b) < s.aead.NonceSize() {
		return ErrInvalid
	}
	n := s.aead.NonceSize()
	plain, err := s.aead.Open(nil, b[:n], b[n:], []byte(name))
	if err != nil {
		return ErrInvalid
	}
	var env envelope
	if err := json.Unmarshal(plain, &env); err != nil || s.now().Unix() >= env.Exp {
		return ErrInvalid
	}
	if err := json.Unmarshal(env.V, v); err != nil {
		return ErrInvalid
	}
	return nil
}
