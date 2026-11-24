package main

import (
	"bytes"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/gabrielassisxyz/finwise/internal/api"
	"github.com/gabrielassisxyz/finwise/internal/webui"
)

const builtIndex = "<!doctype html><title>built</title><script type=module src=/assets/app-1a2b3c.js></script>"

func builtFiles() fs.FS {
	return fstest.MapFS{
		"index.html":           {Data: []byte(builtIndex)},
		"assets/app-1a2b3c.js": {Data: []byte("console.log('built');")},
	}
}

// answered is everything of a response a client can tell apart.
type answered struct {
	status int
	header string
	body   string
}

func answerOf(t *testing.T, response *http.Response) answered {
	t.Helper()
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	var header bytes.Buffer
	if err := response.Header.Write(&header); err != nil {
		t.Fatal(err)
	}
	return answered{status: response.StatusCode, header: header.String(), body: string(body)}
}

type probe struct {
	method, target, body string
	header               map[string]string
}

func (sent probe) through(t *testing.T, handler http.Handler) answered {
	t.Helper()
	request := httptest.NewRequest(sent.method, sent.target, strings.NewReader(sent.body))
	for name, value := range sent.header {
		request.Header.Set(name, value)
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return answerOf(t, recorder.Result())
}

// theAPI is the handler serve builds from its settings, without a database.
func theAPI(t *testing.T) http.Handler {
	t.Helper()
	hash, err := api.ParsePINHash(hashOf(t))
	if err != nil {
		t.Fatal(err)
	}
	env := environment{now: func() time.Time { return testNow }}
	handler, err := api.NewHandler(apiConfig(serveSettings{ledgerPath: "/nonexistent/pluggy-snapshot.db", pinHash: hash}, nil, env, slog.New(slog.DiscardHandler)))
	if err != nil {
		t.Fatal(err)
	}
	return handler
}

var jsonHeader = map[string]string{"Content-Type": "application/json"}

// The interface sits in front of the API, so the proof that it changed nothing
// is the API's own answer to the same request, byte for byte, with and without
// it: status, every header and the body.
func TestTheAPIAnswersExactlyAsWithoutTheInterface(t *testing.T) {
	direct := theAPI(t)
	composed := withInterface(direct, webui.Handler(builtFiles()))
	probes := []probe{
		{method: http.MethodPost, target: "/api/session/status", body: "[]", header: jsonHeader},
		{method: http.MethodPost, target: "/api/session/unlock", body: `["654321"]`, header: jsonHeader},
		{method: http.MethodPost, target: "/api/accounts/list", body: "[]", header: jsonHeader},
		{method: http.MethodGet, target: "/api/session/status"},
		{method: http.MethodPost, target: "/api/not/a/method", body: "[]", header: jsonHeader},
		{method: http.MethodGet, target: "/api"},
		{method: http.MethodGet, target: "/api/"},
		{method: http.MethodGet, target: "/healthz"},
		{method: http.MethodPost, target: "/healthz"},
		{method: http.MethodOptions, target: "/api/session/status"},
		{method: http.MethodPost, target: "//api/session/status", body: "[]", header: jsonHeader},
		{method: http.MethodPost, target: "/./api/session/status", body: "[]", header: jsonHeader},
		{method: http.MethodPost, target: "/revisao/../api/session/status", body: "[]", header: jsonHeader},
		{method: http.MethodGet, target: "/api/../healthz"},
		{method: http.MethodPost, target: "/api/accounts/list", body: "[]", header: map[string]string{"Content-Type": "application/json", "Authorization": "Bearer an-agents-token"}},
	}
	for _, sent := range probes {
		want, got := sent.through(t, direct), sent.through(t, composed)
		if got != want {
			t.Errorf("%s %s: with the interface %+v, without %+v", sent.method, sent.target, got, want)
		}
		if strings.Contains(got.body, builtIndex) {
			t.Errorf("%s %s was answered with the interface", sent.method, sent.target)
		}
	}
}

func TestEveryOtherPathIsTheInterfacesAndNeverTheAPIs(t *testing.T) {
	composed := withInterface(theAPI(t), webui.Handler(builtFiles()))
	for _, target := range []string{"/", "/revisao", "/config/categorizacao", "/contas/cartao", "/apix", "/api-docs", "/healthzz", "/healthz/extra", "/index.html"} {
		got := probe{method: http.MethodGet, target: target}.through(t, composed)
		if got.status != http.StatusOK || got.body != builtIndex || !strings.Contains(got.header, "Cache-Control: no-store") {
			t.Errorf("GET %s answered %d, %q", target, got.status, got.body)
		}
	}
	script := probe{method: http.MethodGet, target: "/assets/app-1a2b3c.js"}.through(t, composed)
	if script.status != http.StatusOK || !strings.Contains(script.header, "public, max-age=31536000, immutable") {
		t.Errorf("a built asset answered %d with headers %q", script.status, script.header)
	}
	missing := probe{method: http.MethodGet, target: "/assets/missing.js"}.through(t, composed)
	if missing.status != http.StatusNotFound || strings.Contains(missing.body, "<!doctype") {
		t.Errorf("a missing asset answered %d %q", missing.status, missing.body)
	}
	if posted := (probe{method: http.MethodPost, target: "/revisao", body: "[]", header: jsonHeader}).through(t, composed); posted.status != http.StatusMethodNotAllowed {
		t.Errorf("a POST to a screen's path answered %d, want 405", posted.status)
	}
}

func TestOnlyThePathsOfTheAPIAreOwnedByIt(t *testing.T) {
	owned := map[string]bool{
		"/api": true, "/api/": true, "/api/session/status": true, "/api/x/../session/status": true,
		"//api/x": true, "/./api/x": true, "/healthz": true, "/revisao/../healthz": true,
		"/": false, "/apix": false, "/api-docs": false, "/healthzz": false, "/healthz/x": false,
		"/revisao": false, "/assets/api/x.js": false, "/x/api/y": false, "/API/session/status": false,
	}
	for target, want := range owned {
		if got := ownedByAPI(target); got != want {
			t.Errorf("ownedByAPI(%q) = %v, want %v", target, got, want)
		}
	}
}

// The real server, over the interface embedded in this tree: the placeholder
// where the interface was not built, the built files where it was.
func TestTheServerAnswersTheEmbeddedInterfaceAndItsAPIUnchanged(t *testing.T) {
	embeddedIndex, err := fs.ReadFile(webui.Built(), "index.html")
	if err != nil {
		t.Fatal(err)
	}
	server := startServer(t, []string{"serve", "--dev-insecure-cookie"}, serveVars(t))
	get := func(target string) (*http.Response, string) {
		response, err := http.Get("http://" + server.address + target)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		body, _ := io.ReadAll(response.Body)
		return response, string(body)
	}

	for _, target := range []string{"/", "/revisao", "/config/categorizacao"} {
		response, body := get(target)
		if response.StatusCode != http.StatusOK || body != string(embeddedIndex) {
			t.Errorf("GET %s answered %d, not the embedded index.html", target, response.StatusCode)
		}
		if got := response.Header.Get("Content-Type"); got != "text/html; charset=utf-8" {
			t.Errorf("GET %s: Content-Type %q", target, got)
		}
		if got := response.Header.Get("Cache-Control"); got != "no-store" {
			t.Errorf("GET %s: Cache-Control %q", target, got)
		}
		if got := response.Header.Get("X-Content-Type-Options"); got != "nosniff" {
			t.Errorf("GET %s: X-Content-Type-Options %q", target, got)
		}
	}
	if response, body := get("/assets/missing.js"); response.StatusCode != http.StatusNotFound || strings.Contains(body, "<html") {
		t.Errorf("a missing asset answered %d %q", response.StatusCode, body)
	}
	if response, _ := get("/healthz"); response.StatusCode != http.StatusOK {
		t.Errorf("/healthz answered %d", response.StatusCode)
	}
	response, body := server.post(t, "/api/session/status", "[]")
	if response.StatusCode != http.StatusOK || !strings.HasPrefix(response.Header.Get("Content-Type"), "application/json") || strings.Contains(body, "<html") {
		t.Errorf("session.status answered %d %q (%s)", response.StatusCode, body, response.Header.Get("Content-Type"))
	}
	if _, body := server.post(t, "/api/session/unlock", `["`+testPIN+`"]`); body != `{"ok":true}` {
		t.Errorf("the PIN answered %s", body)
	}
}
