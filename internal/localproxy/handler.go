// Package localproxy exposes the configured upstream pool as a loopback-only HTTP CONNECT proxy.
package localproxy

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"

	"github.com/ibthe/Go/internal/tunnel"
)

type Dialer interface {
	DialContext(ctx context.Context, network, address string) (net.Conn, error)
}

// Handler accepts CONNECT requests only. It must be served on a loopback
// listener; NewListener enforces that boundary.
type Handler struct{ Dialer Dialer }

func NewListener(address string) (net.Listener, error) {
	host, _, err := net.SplitHostPort(address)
	if err != nil { return nil, fmt.Errorf("listen address must be host:port: %w", err) }
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() { return nil, errors.New("local proxy must bind to a loopback IP address") }
	return net.Listen("tcp", address)
}

func (h Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodConnect {
		http.Error(w, "CONNECT method required", http.StatusMethodNotAllowed)
		return
	}
	if h.Dialer == nil {
		http.Error(w, "proxy pool unavailable", http.StatusServiceUnavailable)
		return
	}
	if r.URL == nil || r.Host == "" || strings.ContainsAny(r.Host, "\\r\\n /") {
		http.Error(w, "invalid CONNECT destination", http.StatusBadRequest)
		return
	}
	host, port, err := net.SplitHostPort(r.Host)
	if err != nil || host == "" || port == "" {
		http.Error(w, "CONNECT destination must include host and port", http.StatusBadRequest)
		return
	}
	hijacker, ok := w.(http.Hijacker)
	if !ok { http.Error(w, "hijacking unsupported", http.StatusInternalServerError); return }
	client, rw, err := hijacker.Hijack()
	if err != nil { return }
	if _, err = rw.WriteString("HTTP/1.1 200 Connection Established\\r\\n\\r\\n"); err == nil { err = rw.Flush() }
	if err != nil { _ = client.Close(); return }
	// Preserve any bytes net/http read beyond the CONNECT headers.
	inbound := &bufferedConn{Conn: client, reader: rw.Reader}
	_, _ = tunnel.RelayTCP(r.Context(), inbound, h.Dialer, r.Host)
}

type bufferedConn struct { net.Conn; reader *bufio.Reader }
func (c *bufferedConn) Read(p []byte) (int, error) { return c.reader.Read(p) }
