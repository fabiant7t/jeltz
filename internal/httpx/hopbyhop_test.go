package httpx

import (
	"net/http"
	"testing"
)

func TestRemoveHopByHop_RemovesStandardHeaders(t *testing.T) {
	h := http.Header{}
	h.Set("Connection", "keep-alive")
	h.Set("Proxy-Connection", "keep-alive")
	h.Set("Keep-Alive", "timeout=5")
	h.Set("Te", "trailers")
	h.Set("Trailer", "X-Trailer")
	h.Set("Transfer-Encoding", "chunked")
	h.Set("Upgrade", "websocket")
	h.Set("Proxy-Authenticate", "Basic realm=test")
	h.Set("Proxy-Authorization", "Basic abc")
	h.Set("X-Keep", "ok")

	RemoveHopByHop(h)

	if h.Get("X-Keep") != "ok" {
		t.Fatalf("X-Keep unexpectedly removed")
	}
	for _, k := range []string{
		"Connection",
		"Proxy-Connection",
		"Keep-Alive",
		"Te",
		"Trailer",
		"Transfer-Encoding",
		"Upgrade",
		"Proxy-Authenticate",
		"Proxy-Authorization",
	} {
		if got := h.Get(k); got != "" {
			t.Fatalf("%s: got %q, want empty", k, got)
		}
	}
}

func TestRemoveHopByHop_RemovesConnectionListedHeaders(t *testing.T) {
	h := http.Header{}
	h.Add("Connection", "X-Foo, X-Bar")
	h.Add("Connection", "X-Baz")
	h.Set("X-Foo", "1")
	h.Set("X-Bar", "2")
	h.Set("X-Baz", "3")
	h.Set("X-Keep", "ok")

	RemoveHopByHop(h)

	if h.Get("X-Keep") != "ok" {
		t.Fatalf("X-Keep unexpectedly removed")
	}
	for _, k := range []string{"X-Foo", "X-Bar", "X-Baz", "Connection"} {
		if got := h.Get(k); got != "" {
			t.Fatalf("%s: got %q, want empty", k, got)
		}
	}
}
