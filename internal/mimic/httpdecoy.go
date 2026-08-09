package mimic

import (
	"io"
	"net/http"
)

// WebDefaultHTTPHandler serves the plaintext :80 default page for a persona style,
// used when no domain is configured (so there is nothing to redirect to). It mirrors
// the same body/headers as the raw net.Conn decoys.
func WebDefaultHTTPHandler(style string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		if style == "directadmin" {
			h.Set("Server", "Apache/2")
			h.Set("Vary", "User-Agent")
			h.Set("Accept-Ranges", "bytes")
			h.Set("Last-Modified", "Mon, 27 Oct 2025 18:36:27 GMT")
			h.Set("Content-Type", "text/html")
			_, _ = io.WriteString(w, daWebDefault)
			return
		}
		h.Set("Server", "Apache/2.4.52 (Ubuntu)")
		h.Set("Content-Type", "text/html; charset=UTF-8")
		_, _ = io.WriteString(w, apacheDefaultPage)
	})
}

// RedirectHTTPSHandler 301-redirects every request to the HTTPS site for domain,
// exactly as a real hosting box with SSL does. Carries an Apache signature.
func RedirectHTTPSHandler(domain string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Server", "Apache/2")
		http.Redirect(w, r, "https://"+domain+r.URL.RequestURI(), http.StatusMovedPermanently)
	})
}
