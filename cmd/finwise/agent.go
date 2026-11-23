package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/contract"
)

const (
	agentUsage       = "usage: finwise agent read <method> [<json-arguments>]"
	agentURLVariable = "FINWISE_URL"
	defaultAgentURL  = "http://127.0.0.1:8080"
	// agentTimeout is the server's WriteTimeout: past it the server has given up
	// on the answer too.
	agentTimeout = 60 * time.Second
	// maxAgentAnswerBytes keeps an address that is not the API from being read
	// into memory without end.
	maxAgentAnswerBytes = 64 << 20
)

// plainErrorKinds are the kinds of a failed call that carry nothing else, the
// contract's own. The client prints a kind only if it is one of these, so that
// nothing a server sent is ever repeated.
var plainErrorKinds = []string{"locked", "read_only", "ledger_unavailable", "bad_request", "unknown_method", "internal"}

const refusedKind = "refused"

// agentCall is what the agent asked for, checked against the manifest.
type agentCall struct {
	method    contract.Method
	arguments []byte
}

// runAgent is the agent's client of the running server. It has one verb, and no
// way to call anything but a read.
func runAgent(ctx context.Context, args []string, env environment) int {
	if len(args) == 0 || args[0] != "read" {
		fmt.Fprintln(env.stderr, agentUsage)
		return exitUsage
	}
	return runAgentRead(ctx, args[1:], env)
}

// runAgentRead calls one read of the contract and prints the server's answer
// and nothing else. What it writes on standard error is a fixed sentence: never
// the token, an argument, or anything the server sent.
func runAgentRead(ctx context.Context, args []string, env environment) int {
	call, problem := agentCallOf(args)
	if problem != "" {
		fmt.Fprintln(env.stderr, problem)
		fmt.Fprintln(env.stderr, agentUsage)
		return exitUsage
	}
	token := env.getenv(agentTokenVariable)
	if token == "" {
		fmt.Fprintf(env.stderr, "finwise: %s is not set\n", agentTokenVariable)
		return exitFailure
	}
	if !isHeaderSafe(token) {
		fmt.Fprintf(env.stderr, "finwise: %s holds characters a token never has\n", agentTokenVariable)
		return exitFailure
	}
	address := firstNonEmpty(env.getenv(agentURLVariable), defaultAgentURL)
	if !usableAgentAddress(address) {
		fmt.Fprintf(env.stderr, "finwise: %s must be an https:// address, or an http:// one on this machine, with no user name, query or fragment\n", agentURLVariable)
		return exitFailure
	}
	status, answer, err := postRead(ctx, newAgentClient(agentTimeout), strings.TrimRight(address, "/")+call.method.Route, token, call.arguments)
	if err != nil {
		fmt.Fprintf(env.stderr, "finwise: could not reach the server at %s\n", address)
		return exitFailure
	}
	return reportAnswer(env, call.method, status, answer)
}

// agentCallOf checks the method against the embedded manifest, so that nothing
// is sent for a name it lacks or a method that is not a read, and the arguments
// against the shape the server takes. The problem is a fixed sentence, empty
// only when there is none.
func agentCallOf(args []string) (agentCall, string) {
	if len(args) == 0 || len(args) > 2 {
		return agentCall{}, "finwise: agent read takes a method and, optionally, its arguments"
	}
	methods, err := contract.Load()
	if err != nil {
		return agentCall{}, "finwise: the contract cannot be read"
	}
	found := slices.IndexFunc(methods, func(method contract.Method) bool { return method.Qualified() == args[0] })
	if found < 0 || methods[found].Class != contract.ClassRead {
		return agentCall{}, "finwise: that is not a read of the contract"
	}
	arguments := []byte("[]")
	if len(args) == 2 {
		arguments = []byte(args[1])
	}
	var parsed []json.RawMessage
	if json.Unmarshal(arguments, &parsed) != nil || parsed == nil {
		return agentCall{}, "finwise: the arguments are not a JSON array"
	}
	return agentCall{method: methods[found], arguments: arguments}, ""
}

// isHeaderSafe is whether the value can be a header's: visible ASCII only, which
// every token the server makes is.
func isHeaderSafe(value string) bool {
	for index := 0; index < len(value); index++ {
		if value[index] <= ' ' || value[index] > '~' {
			return false
		}
	}
	return true
}

// usableAgentAddress is the rule that keeps the token off a network in clear:
// https for any host, http only for this machine. An address with a user name
// would put a password in the messages that name it.
func usableAgentAddress(address string) bool {
	parsed, err := url.Parse(address)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" {
		return false
	}
	switch parsed.Scheme {
	case "https":
		return parsed.Hostname() != ""
	case "http":
		return isLoopbackHost(parsed.Hostname())
	}
	return false
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// newAgentClient never follows a redirect: the token would go wherever the
// server said, and a reply that is not the API's own is a failure anyway.
func newAgentClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout:       timeout,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}

// postRead sends the arguments as a read's body. An error is only that nothing
// usable came back; its text may carry the address and is never shown.
func postRead(ctx context.Context, client *http.Client, target, token string, arguments []byte) (int, []byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(arguments))
	if err != nil {
		return 0, nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+token)
	response, err := client.Do(request)
	if err != nil {
		return 0, nil, err
	}
	defer response.Body.Close()
	answer, err := io.ReadAll(io.LimitReader(response.Body, maxAgentAnswerBytes+1))
	return response.StatusCode, answer, err
}

// reportAnswer prints a 200's body as received, and for anything else one
// sentence made of the method, which the manifest named, and the kind and
// reason, which are printed only when they are the contract's own.
func reportAnswer(env environment, method contract.Method, status int, answer []byte) int {
	notTheAPIs := func() int {
		fmt.Fprintf(env.stderr, "finwise: %s: the answer is not the API's\n", method.Qualified())
		return exitFailure
	}
	if len(answer) > maxAgentAnswerBytes {
		return notTheAPIs()
	}
	if status == http.StatusOK {
		if !json.Valid(answer) {
			return notTheAPIs()
		}
		fmt.Fprintf(env.stdout, "%s\n", answer)
		return 0
	}
	kind, reason, ok := failureOf(answer)
	if !ok {
		return notTheAPIs()
	}
	if reason != "" {
		fmt.Fprintf(env.stderr, "finwise: %s answered %s (%s)\n", method.Qualified(), kind, reason)
	} else {
		fmt.Fprintf(env.stderr, "finwise: %s answered %s\n", method.Qualified(), kind)
	}
	return exitFailure
}

// failureOf reads the contract's error envelope. It returns the client's own
// copy of the kind and the reason, never the bytes that were sent.
func failureOf(answer []byte) (kind, reason string, ok bool) {
	var envelope struct {
		Error *struct {
			Kind   string `json:"kind"`
			Reason string `json:"reason"`
		} `json:"error"`
	}
	if json.Unmarshal(answer, &envelope) != nil || envelope.Error == nil {
		return "", "", false
	}
	if index := slices.Index(plainErrorKinds, envelope.Error.Kind); index >= 0 && envelope.Error.Reason == "" {
		return plainErrorKinds[index], "", true
	}
	if envelope.Error.Kind != refusedKind {
		return "", "", false
	}
	manifest, err := contract.LoadManifest()
	if err != nil {
		return "", "", false
	}
	index := slices.Index(manifest.RefusalReasons, envelope.Error.Reason)
	if index < 0 {
		return "", "", false
	}
	return refusedKind, manifest.RefusalReasons[index], true
}
