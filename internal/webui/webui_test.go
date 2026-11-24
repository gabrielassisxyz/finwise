package webui

import (
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

const (
	indexBody  = "<!doctype html><title>interface</title><script type=module src=/assets/app-1a2b3c.js></script>"
	scriptBody = "console.log('built');"
	styleBody  = "body{margin:0}"
)

// builtInterface is what `vite build` writes, invented: an index that names a
// hashed script, the script, a style and a font.
func builtInterface() fs.FS {
	return fstest.MapFS{
		"index.html":                    {Data: []byte(indexBody)},
		"assets/app-1a2b3c.js":          {Data: []byte(scriptBody)},
		"assets/app-4d5e6f.css":         {Data: []byte(styleBody)},
		"assets/geist-latin-9z8y.woff2": {Data: []byte("wOF2 not a real font")},
		"favicon.svg":                   {Data: []byte("<svg xmlns=\"http://www.w3.org/2000/svg\"/>")},
	}
}

type answer struct {
	status int
	header http.Header
	body   string
}

func ask(t *testing.T, handler http.Handler, method, target string) answer {
	t.Helper()
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(method, target, nil))
	result := recorder.Result()
	defer result.Body.Close()
	body, err := io.ReadAll(result.Body)
	if err != nil {
		t.Fatal(err)
	}
	return answer{status: result.StatusCode, header: result.Header, body: string(body)}
}

func (got answer) mustBeTheIndex(t *testing.T, target string) {
	t.Helper()
	if got.status != http.StatusOK || got.body != indexBody {
		t.Errorf("%s answered %d %q, want 200 and the index", target, got.status, got.body)
	}
	if contentType := got.header.Get("Content-Type"); contentType != "text/html; charset=utf-8" {
		t.Errorf("%s: Content-Type %q", target, contentType)
	}
	if cache := got.header.Get("Cache-Control"); cache != "no-store" {
		t.Errorf("%s: Cache-Control %q, want no-store so a new release is picked up", target, cache)
	}
	got.mustNotBeSniffed(t, target)
}

func (got answer) mustNotBeSniffed(t *testing.T, target string) {
	t.Helper()
	if sniff := got.header.Get("X-Content-Type-Options"); sniff != "nosniff" {
		t.Errorf("%s: X-Content-Type-Options %q, want nosniff", target, sniff)
	}
}

func TestTheRootIsTheIndex(t *testing.T) {
	ask(t, Handler(builtInterface()), http.MethodGet, "/").mustBeTheIndex(t, "/")
}

func TestTheIndexNamedOutrightIsTheIndexAndNotARedirect(t *testing.T) {
	ask(t, Handler(builtInterface()), http.MethodGet, "/index.html").mustBeTheIndex(t, "/index.html")
}

func TestAScreensPathIsAnsweredWithTheIndexSoAReloadWorks(t *testing.T) {
	handler := Handler(builtInterface())
	for _, target := range []string{"/revisao", "/config/categorizacao", "/revisao/fechamento", "/contas/cartao/", "/cofrinhos/some-id", "/budget/cenarios?mes=2026-11"} {
		ask(t, handler, http.MethodGet, target).mustBeTheIndex(t, target)
	}
}

func TestABuiltAssetIsServedWithItsTypeAndTheImmutableCache(t *testing.T) {
	handler := Handler(builtInterface())
	cases := map[string]struct{ contentType, body string }{
		"/assets/app-1a2b3c.js":          {"text/javascript; charset=utf-8", scriptBody},
		"/assets/app-4d5e6f.css":         {"text/css; charset=utf-8", styleBody},
		"/assets/geist-latin-9z8y.woff2": {"font/woff2", "wOF2 not a real font"},
	}
	for target, want := range cases {
		got := ask(t, handler, http.MethodGet, target)
		if got.status != http.StatusOK || got.body != want.body {
			t.Errorf("%s answered %d %q", target, got.status, got.body)
		}
		if contentType := got.header.Get("Content-Type"); contentType != want.contentType {
			t.Errorf("%s: Content-Type %q, want %q", target, contentType, want.contentType)
		}
		if cache := got.header.Get("Cache-Control"); cache != "public, max-age=31536000, immutable" {
			t.Errorf("%s: Cache-Control %q", target, cache)
		}
		got.mustNotBeSniffed(t, target)
	}
}

// A missing script answered with HTML shows up as a syntax error in the
// browser, far from its cause.
func TestAMissingAssetIsA404AndNeverTheIndex(t *testing.T) {
	handler := Handler(builtInterface())
	for _, target := range []string{"/assets/missing.js", "/assets/app-0000.css", "/assets/deeper/missing.js", "/assets/", "/assets", "/assets/../assets/missing.js"} {
		got := ask(t, handler, http.MethodGet, target)
		if got.status != http.StatusNotFound {
			t.Errorf("%s answered %d, want 404", target, got.status)
		}
		if strings.Contains(got.body, "<!doctype html>") || strings.Contains(got.body, "interface") {
			t.Errorf("%s answered the index: %q", target, got.body)
		}
		if cache := got.header.Get("Cache-Control"); cache != "no-store" {
			t.Errorf("%s: a 404 carries Cache-Control %q, which a proxy could keep", target, cache)
		}
		got.mustNotBeSniffed(t, target)
	}
}

func TestAFileOutsideAssetsIsServedAndNotCached(t *testing.T) {
	got := ask(t, Handler(builtInterface()), http.MethodGet, "/favicon.svg")
	if got.status != http.StatusOK || !strings.HasPrefix(got.body, "<svg") {
		t.Errorf("/favicon.svg answered %d %q", got.status, got.body)
	}
	if contentType := got.header.Get("Content-Type"); contentType != "image/svg+xml" {
		t.Errorf("Content-Type %q", contentType)
	}
	if cache := got.header.Get("Cache-Control"); cache != "no-store" {
		t.Errorf("a file that is not hashed carries Cache-Control %q", cache)
	}
}

func TestAPathThatEscapesTheAssetsDirectoryIsJudgedByWhereItLands(t *testing.T) {
	handler := Handler(builtInterface())
	ask(t, handler, http.MethodGet, "/assets/%2e%2e/index.html").mustBeTheIndex(t, "/assets/../index.html")
	ask(t, handler, http.MethodGet, "/assets/../../../etc/passwd").mustBeTheIndex(t, "/assets/../../../etc/passwd")
	if got := ask(t, handler, http.MethodGet, "/revisao/../assets/app-1a2b3c.js"); got.body != scriptBody {
		t.Errorf("a path that lands on an asset answered %q", got.body)
	}
}

func TestOnlyReadsAreAnswered(t *testing.T) {
	handler := Handler(builtInterface())
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch} {
		got := ask(t, handler, method, "/revisao")
		if got.status != http.StatusMethodNotAllowed || got.header.Get("Allow") != "GET, HEAD" {
			t.Errorf("%s answered %d with Allow %q", method, got.status, got.header.Get("Allow"))
		}
		got.mustNotBeSniffed(t, method)
	}
}

func TestHeadAnswersTheHeadersWithoutTheBody(t *testing.T) {
	handler := Handler(builtInterface())
	got := ask(t, handler, http.MethodHead, "/assets/app-1a2b3c.js")
	if got.status != http.StatusOK || got.body != "" {
		t.Errorf("HEAD answered %d with a body of %d bytes", got.status, len(got.body))
	}
	if got.header.Get("Content-Length") != "21" || got.header.Get("Cache-Control") != "public, max-age=31536000, immutable" {
		t.Errorf("HEAD headers: %v", got.header)
	}
}

func TestWithoutAnIndexNothingIsMadeUp(t *testing.T) {
	got := ask(t, Handler(fstest.MapFS{"assets/app.js": {Data: []byte(scriptBody)}}), http.MethodGet, "/revisao")
	if got.status != http.StatusNotFound {
		t.Errorf("answered %d, want 404", got.status)
	}
}

// What the repository versions is the placeholder, and a tree where the
// interface was built carries its assets beside a different index.html. An
// index that is not the placeholder, with no assets to go with it, is a built
// file committed by mistake: the tracked interface would then name scripts the
// binary does not hold.
func TestTheEmbeddedIndexIsThePlaceholderUnlessTheInterfaceWasBuilt(t *testing.T) {
	embeddedFiles := Built()
	index, err := fs.ReadFile(embeddedFiles, indexName)
	if err != nil {
		t.Fatalf("the embedded directory has no index.html: %v", err)
	}
	if _, err := fs.Stat(embeddedFiles, assetsDir); err == nil {
		t.Skip("the interface was built into this tree; only the placeholder is versioned")
	}
	for _, want := range []string{`lang="pt-BR"`, "A interface não foi construída", "npm --prefix web run build", "internal/webui/dist"} {
		if !strings.Contains(string(index), want) {
			t.Errorf("the placeholder does not say %q", want)
		}
	}
	got := ask(t, Handler(embeddedFiles), http.MethodGet, "/")
	if got.status != http.StatusOK || got.body != string(index) {
		t.Errorf("/ answered %d, not the placeholder", got.status)
	}
}
