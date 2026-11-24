package api

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"net/http"
	"strings"

	"github.com/gabrielassisxyz/finwise/internal/contract"
)

const (
	agentTokenBytes      = 32
	agentTokenHashScheme = "sha256"
	agentTokenHashFields = 2
	authorizationHeader  = "Authorization"
	bearerScheme         = "Bearer"
)

// ErrMalformedAgentTokenHash carries no part of the value it rejected: the
// message ends up in a terminal and in a service manager's journal.
var ErrMalformedAgentTokenHash = errors.New("api: the agent token hash is not in the form sha256$<base64 of a SHA-256 digest>")

// AgentTokenHash is a parsed FINWISE_AGENT_TOKEN_HASH. It holds the digest of
// the token and nothing that could stand in for it.
type AgentTokenHash struct {
	digest [sha256.Size]byte
}

// NewAgentToken makes a token for an agent and the stored form of its hash. The
// token is 32 random bytes in unpadded base64url, as a session's is; 256 random
// bits cannot be guessed, so a plain SHA-256 is enough to keep.
func NewAgentToken() (token, stored string, err error) {
	raw := make([]byte, agentTokenBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", "", err
	}
	token = base64.RawURLEncoding.EncodeToString(raw)
	sum := sha256.Sum256([]byte(token))
	return token, agentTokenHashScheme + "$" + base64.StdEncoding.EncodeToString(sum[:]), nil
}

// ParseAgentTokenHash reads the stored form back. Any deviation is one error.
func ParseAgentTokenHash(stored string) (AgentTokenHash, error) {
	fields := strings.Split(stored, "$")
	if len(fields) != agentTokenHashFields || fields[0] != agentTokenHashScheme {
		return AgentTokenHash{}, ErrMalformedAgentTokenHash
	}
	decoded, err := base64.StdEncoding.DecodeString(fields[1])
	if err != nil || len(decoded) != sha256.Size || base64.StdEncoding.EncodeToString(decoded) != fields[1] {
		return AgentTokenHash{}, ErrMalformedAgentTokenHash
	}
	var hash AgentTokenHash
	copy(hash.digest[:], decoded)
	return hash, nil
}

// Verify compares the digest of presented with the stored one in constant time.
// Both sides have the same length whatever was presented, so neither the length
// of a wrong token nor where it differs shows in how long the answer takes.
func (hash AgentTokenHash) Verify(presented string) bool {
	sum := sha256.Sum256([]byte(presented))
	return subtle.ConstantTimeCompare(sum[:], hash.digest[:]) == 1
}

// submitProposalMethod is the one write method the agent's token opens: it
// validates and stores a proposal for the person to approve or refuse, never
// changing authored financial state directly.
const submitProposalMethod = "review.submitProposal"

// agentMayCall is the one statement of what the agent's token opens: the
// manifest's reads and review.submitProposal. Every other method, evaluations,
// approvals, refusals and the session methods included, is out of its reach.
func agentMayCall(method contract.Method) bool {
	return method.Class == contract.ClassRead || method.Qualified() == submitProposalMethod
}

// carriesAuthorization is whether the request names itself with the header at
// all, whatever the header says. Such a request is judged by the header alone.
func carriesAuthorization(r *http.Request) bool {
	return len(r.Header.Values(authorizationHeader)) > 0
}

// bearerToken is the token of an `Authorization: Bearer <token>` header, and
// false for any other shape, a second header included: with two the request
// does not say which one decides.
func bearerToken(r *http.Request) (string, bool) {
	values := r.Header.Values(authorizationHeader)
	if len(values) != 1 {
		return "", false
	}
	scheme, token, found := strings.Cut(values[0], " ")
	if !found || !strings.EqualFold(scheme, bearerScheme) || token == "" {
		return "", false
	}
	return token, true
}

// agentPresented is whether the request carries the agent's token. A server
// configured with no hash has none to match.
func (found *router) agentPresented(r *http.Request) bool {
	if found.verifyAgent == nil {
		return false
	}
	token, ok := bearerToken(r)
	return ok && found.verifyAgent(token)
}

// serveAgent answers a request that carries an Authorization header, and
// nothing else answers it: the session's cookie is never looked at. A request
// the token does not match is `locked`, as one with no session is; one it
// matches is refused as `read_only`, before its body is read, for any method
// the agent may not call, and otherwise answered as a read, never as a change.
func (found *router) serveAgent(w http.ResponseWriter, r *http.Request, method contract.Method) {
	if !found.agentPresented(r) {
		writeError(w, r, kindLocked)
		return
	}
	noteAgentCaller(r.Context())
	if !agentMayCall(method) {
		writeError(w, r, kindReadOnly)
		return
	}
	if method.Qualified() == submitProposalMethod {
		found.serveChange(w, r, method)
		return
	}
	arguments, body, ok := readArguments(w, r, method)
	if !ok {
		return
	}
	found.serveCheckedRead(w, r, method, arguments, body)
}
