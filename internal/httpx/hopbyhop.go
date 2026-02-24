// Package httpx provides shared HTTP utilities for jeltz.
package httpx

import (
	"net/http"
	"strings"
)

// hopByHopHeaders is the set of headers that must not be forwarded.
var hopByHopHeaders = map[string]struct{}{
	"Connection":          {},
	"Proxy-Connection":    {},
	"Keep-Alive":          {},
	"Te":                  {},
	"Trailer":             {},
	"Transfer-Encoding":   {},
	"Upgrade":             {},
	"Proxy-Authenticate":  {},
	"Proxy-Authorization": {},
}

// RemoveHopByHop removes hop-by-hop headers from h, including any headers
// listed in a Connection header value.
func RemoveHopByHop(h http.Header) {
	// Headers listed in Connection: are also hop-by-hop.
	for _, v := range h["Connection"] {
		for tok := range strings.SplitSeq(v, ",") {
			h.Del(strings.TrimSpace(tok))
		}
	}
	for name := range hopByHopHeaders {
		h.Del(name)
	}
}
