package api

import (
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

const urlSafeAlphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_"

func newTokenAndHash(t testing.TB) (string, AgentTokenHash) {
	t.Helper()
	token, stored, err := NewAgentToken()
	if err != nil {
		t.Fatal(err)
	}
	hash, err := ParseAgentTokenHash(stored)
	if err != nil {
		t.Fatalf("the hash a fresh token comes with does not parse: %v", err)
	}
	return token, hash
}

func TestAFreshTokenIs32RandomBytesInUnpaddedBase64URL(t *testing.T) {
	seen := map[string]bool{}
	for range 20 {
		token, _, err := NewAgentToken()
		if err != nil {
			t.Fatal(err)
		}
		if len(token) != 43 {
			t.Errorf("a token of %d characters, want 43", len(token))
		}
		if bad := strings.Trim(token, urlSafeAlphabet); bad != "" {
			t.Errorf("the token holds characters outside the base64url alphabet")
		}
		raw, err := base64.RawURLEncoding.DecodeString(token)
		if err != nil || len(raw) != agentTokenBytes {
			t.Errorf("the token does not decode to %d bytes: %v", agentTokenBytes, err)
		}
		if seen[token] {
			t.Error("two tokens were the same")
		}
		seen[token] = true
	}
}

func TestTheStoredFormIsTheSchemeAndTheSHA256DigestOfTheTokenInBase64(t *testing.T) {
	token, stored, err := NewAgentToken()
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte(token))
	want := "sha256$" + base64.StdEncoding.EncodeToString(sum[:])
	if stored != want {
		t.Errorf("the stored form is not the scheme and the digest of the token")
	}
	if strings.Contains(stored, token) {
		t.Error("the stored form holds the token")
	}
}

func TestTheStoredHashVerifiesItsTokenAndRefusesAnythingElse(t *testing.T) {
	token, hash := newTokenAndHash(t)
	if !hash.Verify(token) {
		t.Fatal("the hash does not verify its own token")
	}
	other, _ := newTokenAndHash(t)
	refused := map[string]string{
		"another token": other, "empty": "", "a prefix": token[:len(token)-1], "a suffix": token + "a",
		"a space around": " " + token, "padding": token + "=", "upper case": strings.ToUpper(token),
		"the stored form": "sha256$" + base64.StdEncoding.EncodeToString(sha256Of(token)),
		"a long one":      strings.Repeat(token, 1000),
	}
	for label, attempt := range refused {
		if attempt != token && hash.Verify(attempt) {
			t.Errorf("%s was verified", label)
		}
	}
	for position := range len(token) {
		changed := []byte(token)
		changed[position] = differentCharacter(changed[position])
		if hash.Verify(string(changed)) {
			t.Errorf("the token with character %d changed was verified", position)
		}
	}
}

func sha256Of(value string) []byte {
	sum := sha256.Sum256([]byte(value))
	return sum[:]
}

func differentCharacter(character byte) byte {
	if character == 'A' {
		return 'B'
	}
	return 'A'
}

func TestAHashThatIsNotInTheStoredFormDoesNotParseAndItsErrorNamesNoPartOfIt(t *testing.T) {
	_, stored, err := NewAgentToken()
	if err != nil {
		t.Fatal(err)
	}
	digest := strings.TrimPrefix(stored, "sha256$")
	short := base64.StdEncoding.EncodeToString(make([]byte, 31))
	long := base64.StdEncoding.EncodeToString(make([]byte, 33))
	cases := map[string]string{
		"empty":            "",
		"no scheme":        digest,
		"another scheme":   "sha512$" + digest,
		"a scheme in case": "SHA256$" + digest,
		"no digest":        "sha256$",
		"an extra field":   stored + "$more",
		"not base64":       "sha256$not-base64-at-all!",
		"a short digest":   "sha256$" + short,
		"a long digest":    "sha256$" + long,
		"the url alphabet": "sha256$" + strings.NewReplacer("+", "-", "/", "_").Replace(digest) + "x",
		"a space":          "sha256$" + digest + " ",
		"a newline":        stored + "\n",
		"a line break":     "sha256$" + digest[:20] + "\r\n" + digest[20:],
		"the PIN's form":   "pbkdf2-sha256$600000$c2FsdHNhbHQ=$" + digest,
	}
	for label, value := range cases {
		_, err := ParseAgentTokenHash(value)
		if !errors.Is(err, ErrMalformedAgentTokenHash) {
			t.Errorf("%s: parsed or failed with another error: %v", label, err)
			continue
		}
		if strings.Contains(err.Error(), digest) {
			t.Errorf("%s: the error repeats the value", label)
		}
	}
	if _, err := ParseAgentTokenHash(stored); err != nil {
		t.Errorf("the stored form of a fresh token did not parse: %v", err)
	}
}

// The comparison of two digests is a property of the code and not of an
// answer, so the source is read: the stored digest is compared through
// crypto/subtle and no other way.
func TestTheDigestsAreComparedWithTheConstantTimeCompareAndNothingElse(t *testing.T) {
	parsed, err := parser.ParseFile(token.NewFileSet(), "agent_token.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	constantTime, equalities := 0, 0
	ast.Inspect(parsed, func(node ast.Node) bool {
		switch typed := node.(type) {
		case *ast.CallExpr:
			if selector, ok := typed.Fun.(*ast.SelectorExpr); ok {
				switch name := selector.Sel.Name; {
				case name == "ConstantTimeCompare":
					constantTime++
				case name == "Equal" || name == "EqualFold" && !isTheSchemeCheck(typed):
					t.Errorf("agent_token.go calls %s, which stops at the first difference", name)
				}
			}
		case *ast.BinaryExpr:
			if (typed.Op == token.EQL || typed.Op == token.NEQ) && !againstALiteral(typed) && mentionsSecret(typed) {
				equalities++
			}
		}
		return true
	})
	if constantTime != 1 {
		t.Errorf("agent_token.go calls ConstantTimeCompare %d times, want once", constantTime)
	}
	if equalities != 0 {
		t.Errorf("agent_token.go compares a digest or a token with == or != in %d places", equalities)
	}
}

// isTheSchemeCheck is the one EqualFold of the file: it reads the word Bearer,
// which is not a secret.
func isTheSchemeCheck(call *ast.CallExpr) bool {
	for _, argument := range call.Args {
		if identifier, ok := argument.(*ast.Ident); ok && identifier.Name == "bearerScheme" {
			return true
		}
	}
	return false
}

// againstALiteral is a comparison with a constant, such as an empty check,
// which says nothing about where a secret differs from another.
func againstALiteral(expression *ast.BinaryExpr) bool {
	_, left := expression.X.(*ast.BasicLit)
	_, right := expression.Y.(*ast.BasicLit)
	return left || right
}

func mentionsSecret(expression *ast.BinaryExpr) bool {
	found := false
	ast.Inspect(expression, func(node ast.Node) bool {
		switch typed := node.(type) {
		case *ast.Ident:
			found = found || isSecretName(typed.Name)
		case *ast.SelectorExpr:
			found = found || isSecretName(typed.Sel.Name)
		}
		return true
	})
	return found
}

func isSecretName(name string) bool {
	switch name {
	case "digest", "sum", "presented", "token":
		return true
	}
	return false
}
