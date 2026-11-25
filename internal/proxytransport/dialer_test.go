package proxytransport

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func proxyServer(t *testing.T, status int, wantAuth string) (*httptest.Server, *tls.Config) {
	t.Helper()
	s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodConnect { t.Errorf("method = %s, want CONNECT", r.Method) }
		if r.Host != "example.com:443" { t.Errorf("CONNECT host = %s", r.Host) }
		if got := r.Header.Get("Proxy-Authorization"); got != wantAuth { t.Errorf("proxy auth = %q, want %q", got, wantAuth) }
		if status != http.StatusOK { w.WriteHeader(status); return }
		h, ok := w.(http.Hijacker)
		if !ok { t.Error("response writer does not support hijacking"); return }
		conn, rw, err := h.Hijack()
		if err != nil { t.Error(err); return }
		defer conn.Close()
		_, _ = rw.WriteString("HTTP/1.1 200 Connection Established\r\n\r\nwelcome")
		_ = rw.Flush()
		_, _ = io.Copy(conn, conn)
	}))
	pool := x509.NewCertPool()
	pool.AddCert(s.Certificate())
	return s, &tls.Config{RootCAs: pool}
}

func TestDialContextCarriesTCPData(t *testing.T) {
	s, cfg := proxyServer(t, http.StatusOK, "")
	defer s.Close()
	u, err := url.Parse(s.URL)
	if err != nil { t.Fatal(err) }
	d := &Dialer{ProxyURL: u, TLSConfig: cfg, Timeout: 2*time.Second}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	conn, err := d.DialContext(ctx, "tcp", "example.com:443")
	if err != nil { t.Fatal(err) }
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(2*time.Second))
	got := make([]byte, len("welcome"))
	if _, err := io.ReadFull(conn, got); err != nil { t.Fatal(err) }
	if string(got) != "welcome" { t.Fatalf("greeting = %q", got) }
	if _, err := conn.Write([]byte("ping")); err != nil { t.Fatal(err) }
	echo := make([]byte, 4)
	if _, err := io.ReadFull(conn, echo); err != nil { t.Fatal(err) }
	if string(echo) != "ping" { t.Fatalf("echo = %q", echo) }
}

func TestDialContextProxyAuthentication(t *testing.T) {
	const auth = "Basic dXNlcjpwYXNz"
	s, cfg := proxyServer(t, http.StatusOK, auth)
	defer s.Close()
	u, _ := url.Parse(s.URL)
	u.User = url.UserPassword("user", "pass")
	d := &Dialer{ProxyURL: u, TLSConfig: cfg}
	conn, err := d.DialContext(context.Background(), "tcp", "example.com:443")
	if err != nil { t.Fatal(err) }
	_ = conn.Close()
}

func TestDialContextRejectsProxyStatus(t *testing.T) {
	s, cfg := proxyServer(t, http.StatusForbidden, "")
	defer s.Close()
	u, _ := url.Parse(s.URL)
	d := &Dialer{ProxyURL: u, TLSConfig: cfg}
	_, err := d.DialContext(context.Background(), "tcp", "example.com:443")
	if err == nil || !strings.Contains(err.Error(), "403") { t.Fatalf("expected 403 error, got %v", err) }
}

func TestDialContextRejectsUDP(t *testing.T) {
	u, _ := url.Parse("https://127.0.0.1:443")
	d := &Dialer{ProxyURL: u}
	_, err := d.DialContext(context.Background(), "udp", net.JoinHostPort("example.com", "53"))
	if err == nil { t.Fatal("expected UDP to be rejected") }
}

func TestDialContextRequiresHTTPSProxy(t *testing.T) {
	u, _ := url.Parse("http://127.0.0.1:8080")
	d := &Dialer{ProxyURL: u}
	_, err := d.DialContext(context.Background(), "tcp", "example.com:443")
	if err == nil { t.Fatal("expected endpoint validation error") }
}
