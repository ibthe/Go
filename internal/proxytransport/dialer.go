// Package proxytransport establishes TCP streams through HTTPS CONNECT proxies.
package proxytransport

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Dialer opens TCP connections through a TLS-protected HTTP proxy.
// It supports HTTP Basic proxy authentication when credentials are present in
// ProxyURL. Callers must keep credentials out of logs and source control.
type Dialer struct {
	ProxyURL  *url.URL
	TLSConfig *tls.Config
	Timeout   time.Duration
}

// DialContext establishes a CONNECT tunnel to address and returns the tunneled
// connection. Only a 200 response is accepted. TLS certificate verification
// remains enabled unless the caller explicitly supplies an unsafe TLSConfig.
func (d *Dialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	if network != "tcp" && network != "tcp4" && network != "tcp6" {
		return nil, fmt.Errorf("unsupported network %q: HTTPS CONNECT carries TCP only", network)
	}
	if d == nil || d.ProxyURL == nil {
		return nil, errors.New("proxy URL is required")
	}
	u := d.ProxyURL
	if u.Scheme != "https" || u.Hostname() == "" || u.Port() == "" ||
		(u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" {
		return nil, errors.New("proxy URL must be https://host:port")
	}
	if address == "" || strings.ContainsAny(address, "\r\n ") {
		return nil, errors.New("invalid CONNECT destination")
	}

	timeout := d.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	dialCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	raw, err := (&net.Dialer{Timeout: timeout}).DialContext(dialCtx, "tcp", net.JoinHostPort(u.Hostname(), u.Port()))
	if err != nil {
		return nil, fmt.Errorf("connect to proxy: %w", err)
	}
	fail := func(err error) (net.Conn, error) {
		_ = raw.Close()
		return nil, err
	}

	cfg := &tls.Config{MinVersion: tls.VersionTLS12}
	if d.TLSConfig != nil {
		cfg = d.TLSConfig.Clone()
		if cfg.MinVersion < tls.VersionTLS12 {
			cfg.MinVersion = tls.VersionTLS12
		}
	}
	if cfg.ServerName == "" {
		cfg.ServerName = u.Hostname()
	}
	tlsConn := tls.Client(raw, cfg)
	if deadline, ok := dialCtx.Deadline(); ok {
		_ = tlsConn.SetDeadline(deadline)
	}
	if err := tlsConn.HandshakeContext(dialCtx); err != nil {
		return fail(fmt.Errorf("TLS handshake with proxy: %w", err))
	}

	req := &http.Request{
		Method: http.MethodConnect,
		URL:    &url.URL{Opaque: address},
		Host:   address,
		Header: make(http.Header),
	}
	if u.User != nil {
		password, _ := u.User.Password()
		token := base64.StdEncoding.EncodeToString([]byte(u.User.Username() + ":" + password))
		req.Header.Set("Proxy-Authorization", "Basic "+token)
	}
	if err := req.Write(tlsConn); err != nil {
		return fail(fmt.Errorf("send CONNECT request: %w", err))
	}

	reader := bufio.NewReader(tlsConn)
	resp, err := http.ReadResponse(reader, req)
	if err != nil {
		return fail(fmt.Errorf("read CONNECT response: %w", err))
	}
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		return fail(fmt.Errorf("proxy rejected CONNECT: %s", resp.Status))
	}
	_ = tlsConn.SetDeadline(time.Time{})
	// Keep the buffered reader: a proxy may send tunnel data in the same TLS
	// record as the CONNECT response.
	return &bufferedConn{Conn: tlsConn, reader: reader}, nil
}

type bufferedConn struct {
	net.Conn
	reader *bufio.Reader
}

func (c *bufferedConn) Read(p []byte) (int, error) { return c.reader.Read(p) }
