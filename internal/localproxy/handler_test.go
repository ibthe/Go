package localproxy

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestListenerRejectsNonLoopback(t *testing.T) {
	for _, address := range []string{"0.0.0.0:0", ":18080", "192.0.2.1:0"} {
		if ln, err := NewListener(address); err == nil {
			_ = ln.Close()
			t.Fatalf("NewListener(%q) unexpectedly succeeded", address)
		}
	}
}

func TestListenerAcceptsLoopback(t *testing.T) {
	ln, err := NewListener("127.0.0.1:0")
	if err != nil { t.Fatal(err) }
	_ = ln.Close()
}

func TestHandlerRejectsNonConnect(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://localhost/", nil)
	(Handler{}).ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed { t.Fatalf("status = %d", rec.Code) }
}

func TestHandlerRejectsInvalidDestination(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodConnect, "http://example.com:443", nil)
	req.Host = "example.com"
	(Handler{}).ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest { t.Fatalf("status = %d", rec.Code) }
}
