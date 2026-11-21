package api

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"strconv"
	"strings"
)

const (
	pinHashScheme     = "pbkdf2-sha256"
	minPINIterations  = 600000
	maxPINIterations  = 50_000_000
	pinSaltBytes      = 16
	pinKeyBytes       = 32
	minPINKeyBytes    = 16
	minPINSaltBytes   = 8
	pinDigitCount     = 6
	pinHashFieldCount = 4
)

// ErrMalformedPINHash carries no part of the value it rejected: the message ends
// up in a terminal and in a service manager's journal.
var ErrMalformedPINHash = errors.New("api: the PIN hash is not in the form pbkdf2-sha256$<iterations>$<salt>$<key>")

// PINHash is a parsed FINWISE_PIN_HASH.
type PINHash struct {
	iterations int
	salt       []byte
	key        []byte
}

// ValidPIN reports whether pin is exactly six ASCII digits.
func ValidPIN(pin string) bool {
	if len(pin) != pinDigitCount {
		return false
	}
	for index := 0; index < len(pin); index++ {
		if pin[index] < '0' || pin[index] > '9' {
			return false
		}
	}
	return true
}

// NewPINHash derives the stored form of pin with a fresh random salt.
func NewPINHash(pin string) (string, error) {
	if !ValidPIN(pin) {
		return "", errors.New("api: a PIN is exactly six digits")
	}
	salt := make([]byte, pinSaltBytes)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key, err := pbkdf2.Key(sha256.New, pin, salt, minPINIterations, pinKeyBytes)
	if err != nil {
		return "", err
	}
	return strings.Join([]string{
		pinHashScheme,
		strconv.Itoa(minPINIterations),
		base64.StdEncoding.EncodeToString(salt),
		base64.StdEncoding.EncodeToString(key),
	}, "$"), nil
}

// ParsePINHash reads the stored form back. Any deviation is one error.
func ParsePINHash(stored string) (PINHash, error) {
	fields := strings.Split(stored, "$")
	if len(fields) != pinHashFieldCount || fields[0] != pinHashScheme {
		return PINHash{}, ErrMalformedPINHash
	}
	iterations, err := strconv.Atoi(fields[1])
	if err != nil || iterations < minPINIterations || iterations > maxPINIterations {
		return PINHash{}, ErrMalformedPINHash
	}
	salt, saltErr := base64.StdEncoding.DecodeString(fields[2])
	key, keyErr := base64.StdEncoding.DecodeString(fields[3])
	if saltErr != nil || keyErr != nil || len(salt) < minPINSaltBytes || len(key) < minPINKeyBytes {
		return PINHash{}, ErrMalformedPINHash
	}
	return PINHash{iterations: iterations, salt: salt, key: key}, nil
}

// Verify derives a key from attempt and compares it in constant time.
func (hash PINHash) Verify(attempt string) bool {
	derived, err := pbkdf2.Key(sha256.New, attempt, hash.salt, hash.iterations, len(hash.key))
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare(derived, hash.key) == 1
}
