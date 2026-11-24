// Package webui serves the interface built for real use: the files `vite
// build` writes, embedded in the program so that one process answers both the
// interface and its API.
package webui

import (
	"bytes"
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
	"time"
)

// dist holds the built interface. Only a placeholder index.html is versioned, so
// the program builds in a tree where the interface was not; the image build and
// the documented local command copy the real files over it.
//
//go:embed all:dist
var embedded embed.FS

const (
	indexName = "index.html"
	// assetsDir is where Vite writes the files whose names carry a hash of their
	// content, which is what makes them safe to cache for good.
	assetsDir      = "assets"
	immutableCache = "public, max-age=31536000, immutable"
	noStore        = "no-store"
)

// Built is the interface embedded in the program, rooted at its index.html.
func Built() fs.FS {
	files, err := fs.Sub(embedded, "dist")
	if err != nil {
		panic("webui: the embedded directory is not there: " + err.Error())
	}
	return files
}

// contentTypes is fixed instead of read from the host's mime database, so a
// file is answered with the same type wherever the program runs. A type it
// lacks is left to net/http, which falls back to the host's table and then to
// sniffing.
var contentTypes = map[string]string{
	".html":  "text/html; charset=utf-8",
	".js":    "text/javascript; charset=utf-8",
	".mjs":   "text/javascript; charset=utf-8",
	".css":   "text/css; charset=utf-8",
	".json":  "application/json",
	".map":   "application/json",
	".svg":   "image/svg+xml",
	".png":   "image/png",
	".jpg":   "image/jpeg",
	".jpeg":  "image/jpeg",
	".webp":  "image/webp",
	".ico":   "image/x-icon",
	".woff":  "font/woff",
	".woff2": "font/woff2",
	".txt":   "text/plain; charset=utf-8",
}

// Handler serves the interface in files. A file under assets/ is cached for
// good and, when it does not exist, is a 404: answering a missing script with
// HTML would hide the failure behind a syntax error. A path that names no other
// file gets index.html, so that a reload on any screen of the router's history
// mode works. Everything but the hashed assets is `no-store`, so a new release is
// picked up at once, and every answer carries `nosniff`.
func Handler(files fs.FS) http.Handler {
	return interfaceFiles{files: files}
}

type interfaceFiles struct {
	files fs.FS
}

func (served interfaceFiles) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		refuse(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
	if name == assetsDir || strings.HasPrefix(name, assetsDir+"/") {
		if !served.send(w, r, name, immutableCache) {
			refuse(w, "404 page not found", http.StatusNotFound)
		}
		return
	}
	if name != "" && name != indexName && served.send(w, r, name, noStore) {
		return
	}
	if !served.send(w, r, indexName, noStore) {
		refuse(w, "404 page not found", http.StatusNotFound)
	}
}

// send answers with the file called name and says whether there was one.
func (served interfaceFiles) send(w http.ResponseWriter, r *http.Request, name, cacheControl string) bool {
	info, err := fs.Stat(served.files, name)
	if err != nil || !info.Mode().IsRegular() {
		return false
	}
	content, err := fs.ReadFile(served.files, name)
	if err != nil {
		return false
	}
	if contentType, known := contentTypes[strings.ToLower(path.Ext(name))]; known {
		w.Header().Set("Content-Type", contentType)
	}
	w.Header().Set("Cache-Control", cacheControl)
	http.ServeContent(w, r, name, time.Time{}, bytes.NewReader(content))
	return true
}

// refuse is an error whose own page is never cached: a 404 kept by a proxy
// during a release would keep a script that exists now from being found.
func refuse(w http.ResponseWriter, message string, status int) {
	w.Header().Set("Cache-Control", noStore)
	http.Error(w, message, status)
}
