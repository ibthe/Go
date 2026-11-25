package proxycheck

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func testServer(t *testing.T, status int) (*httptest.Server, *tls.Config) {
	t.Helper()
	s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodConnect { t.Errorf("method = %s, want CONNECT", r.Method) }
		if r.Host != "example.com:443" { t.Errorf("host = %s", r.Host) }
		w.WriteHeader(status)
	}))
	pool := x509.NewCertPool()
	pool.AddCert(s.Certificate())
	return s, &tls.Config{RootCAs: pool}
}

func TestCheckAcceptsCONNECT(t *testing.T) {
	s, cfg := testServer(t, http.StatusOK)
	defer s.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	got, err := Check(ctx, s.URL, "example.com:443", cfg)
	if err != nil { t.Fatal(err) }
	if got.Target != "example.com:443" || got.Latency <= 0 { t.Fatalf("unexpected result: %+v", got) }
}

func TestCheckRejectsNon200(t *testing.T) {
	s, cfg := testServer(t, http.StatusForbidden)
	defer s.Close()
	if _, err := Check(context.Background(), s.URL, "example.com:443", cfg); err == nil { t.Fatal("expected CONNECT rejection") }
}

func TestCheckRequiresHTTPS(t *testing.T) {
	if _, err := Check(context.Background(), "http://127.0.0.1:8080", "example.com:443", nil); err == nil { t.Fatal("expected endpoint validation error") }
}

func TestFirstHealthyEmpty(t *testing.T) {
	if _, err := FirstHealthy(context.Background(), nil, "example.com:443", nil); err == nil { t.Fatal("expected empty list error") }
}
