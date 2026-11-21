package api

import (
	"os"
	"strings"
	"sync"
	"testing"
)

const testPIN = "123456"

var (
	sharedHashOnce sync.Once
	sharedHash     string
)

// testPINHash derives the hash at run time, once: nothing resembling a stored
// secret is committed, and the 600000 iterations are paid once per test binary.
func testPINHash(t *testing.T) PINHash {
	t.Helper()
	sharedHashOnce.Do(func() {
		stored, err := NewPINHash(testPIN)
		if err != nil {
			t.Fatalf("NewPINHash: %v", err)
		}
		sharedHash = stored
	})
	hash, err := ParsePINHash(sharedHash)
	if err != nil {
		t.Fatalf("ParsePINHash: %v", err)
	}
	return hash
}

func TestPINHashRoundTrip(t *testing.T) {
	hash := testPINHash(t)
	if !hash.Verify(testPIN) {
		t.Error("the PIN it was made from does not verify")
	}
	for _, wrong := range []string{"123457", "000000", "", "1234567", "12345"} {
		if hash.Verify(wrong) {
			t.Errorf("%q verified against the hash of another PIN", wrong)
		}
	}
}

func TestNewPINHashUsesAFreshSalt(t *testing.T) {
	first, err := NewPINHash(testPIN)
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewPINHash(testPIN)
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Error("two hashes of one PIN are identical, the salt is not random")
	}
}

func TestNewPINHashRefusesAnythingButSixDigits(t *testing.T) {
	for _, bad := range []string{"12345", "abcdef", "1234567", "", "12 456", "١٢٣٤٥٦"} {
		if _, err := NewPINHash(bad); err == nil {
			t.Errorf("%q was hashed", bad)
		}
	}
}

func TestParsePINHashRefusesMalformedValuesWithoutEchoingThem(t *testing.T) {
	testPINHash(t) // fills sharedHash
	fields := strings.Split(sharedHash, "$")
	rebuild := func(index int, value string) string {
		changed := append([]string(nil), fields...)
		changed[index] = value
		return strings.Join(changed, "$")
	}
	cases := map[string]string{
		"empty":            "",
		"not a hash":       "not-a-hash",
		"other scheme":     rebuild(0, "pbkdf2-sha1"),
		"too few rounds":   rebuild(1, "599999"),
		"not a number":     rebuild(1, "many"),
		"absurd rounds":    rebuild(1, "99999999999"),
		"salt not base64":  rebuild(2, "***"),
		"key not base64":   rebuild(3, "***"),
		"salt too short":   rebuild(2, "AAAA"),
		"key too short":    rebuild(3, "AAAA"),
		"extra field":      sharedHash + "$x",
		"missing key":      strings.Join(fields[:3], "$"),
		"surrounding text": " " + sharedHash,
	}
	for name, stored := range cases {
		_, err := ParsePINHash(stored)
		if err == nil {
			t.Errorf("%s: accepted", name)
			continue
		}
		if stored != "" && strings.Contains(err.Error(), stored) {
			t.Errorf("%s: the error repeats the value", name)
		}
	}
}

func TestValidPIN(t *testing.T) {
	cases := map[string]bool{"123456": true, "000000": true, "12345": false, "abcdef": false, "1234 6": false}
	for pin, want := range cases {
		if ValidPIN(pin) != want {
			t.Errorf("ValidPIN(%q) = %v, want %v", pin, !want, want)
		}
	}
}

// Timing cannot be asserted from a test; what can be pinned is that the one
// place a derived key is compared uses the constant-time primitive.
func TestPINComparisonIsConstantTime(t *testing.T) {
	source, err := os.ReadFile("pinhash.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)
	if !strings.Contains(text, "subtle.ConstantTimeCompare(") {
		t.Error("pinhash.go no longer compares with subtle.ConstantTimeCompare")
	}
	if strings.Contains(text, "bytes.Equal(") || strings.Contains(text, "derived ==") {
		t.Error("pinhash.go compares keys with a variable-time comparison")
	}
}
