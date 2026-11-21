package api

import (
	"net/http"
	"time"
)

const (
	hostCookieName  = "__Host-finwise-session"
	plainCookieName = "finwise-session"
)

// sessionCookie builds and reads the session cookie. The __Host- prefix makes
// a browser refuse the cookie unless it is Secure, has Path=/ and no Domain,
// so a sibling subdomain cannot plant one.
type sessionCookie struct {
	name   string
	secure bool
	maxAge time.Duration
}

// newSessionCookie drops both the prefix and Secure for development over plain
// HTTP, where a browser would otherwise discard the cookie.
func newSessionCookie(insecureForDevelopment bool, maxAge time.Duration) sessionCookie {
	if insecureForDevelopment {
		return sessionCookie{name: plainCookieName, secure: false, maxAge: maxAge}
	}
	return sessionCookie{name: hostCookieName, secure: true, maxAge: maxAge}
}

func (cookie sessionCookie) base() *http.Cookie {
	return &http.Cookie{
		Name:     cookie.name,
		Path:     "/",
		HttpOnly: true,
		Secure:   cookie.secure,
		SameSite: http.SameSiteStrictMode,
	}
}

// issue sets the cookie for a full lifetime. It is sent again on every request
// that finds a live session, because the server's expiry slides and a cookie
// fixed at login would die while the session was still in use.
func (cookie sessionCookie) issue(w http.ResponseWriter, token string) {
	issued := cookie.base()
	issued.Value = token
	issued.MaxAge = int(cookie.maxAge / time.Second)
	http.SetCookie(w, issued)
}

func (cookie sessionCookie) clear(w http.ResponseWriter) {
	cleared := cookie.base()
	cleared.MaxAge = -1
	http.SetCookie(w, cleared)
}

func (cookie sessionCookie) tokenOf(r *http.Request) string {
	found, err := r.Cookie(cookie.name)
	if err != nil {
		return ""
	}
	return found.Value
}
