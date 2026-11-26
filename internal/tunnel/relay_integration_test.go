package tunnel

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/ibthe/Go/internal/proxytransport"
)

func TestRelayTCPThroughHTTPSConnectProxy(t *testing.T) {
	backend, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer backend.Close()

	go func() {
		for {
			conn, err := backend.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				_, _ = io.Copy(conn, conn)
			}()
		}
	}()

	proxy := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodConnect {
			http.Error(w, "CONNECT required", http.StatusMethodNotAllowed)
			return
		}
		hijacker, ok := w.(http.Hijacker)
		if !ok {
			http.Error(w, "hijacking unsupported", http.StatusInternalServerError)
			return
		}
		clientConn, rw, err := hijacker.Hijack()
		if err != nil {
			return
		}
		upstream, err := net.Dial("tcp", r.Host)
		if err != nil {
			_, _ = fmt.Fprint(rw, "HTTP/1.1 502 Bad Gateway\r\nConnection: close\r\n\r\n")
			_ = rw.Flush()
			_ = clientConn.Close()
			return
		}
		if _, err := fmt.Fprint(rw, "HTTP/1.1 200 Connection Established\r\n\r\n"); err != nil {
			_ = clientConn.Close()
			_ = upstream.Close()
			return
		}
		if err := rw.Flush(); err != nil {
			_ = clientConn.Close()
			_ = upstream.Close()
			return
		}
		go func() {
			_, _ = io.Copy(upstream, clientConn)
			_ = upstream.Close()
		}()
		_, _ = io.Copy(clientConn, upstream)
		_ = clientConn.Close()
		_ = upstream.Close()
	}))
	defer proxy.Close()

	proxyURL, err := url.Parse(proxy.URL)
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	roots.AddCert(proxy.Certificate())
	dialer := &proxytransport.Dialer{
		ProxyURL: proxyURL,
		TLSConfig: &tls.Config{RootCAs: roots},
		Timeout: 3 * time.Second,
	}

	inboundClient, inboundRelay := tcpPair(t)
	defer inboundClient.Close()

	ctx, cancel := context.WithCancel(context.Background())
	relayDone := make(chan error, 1)
	statsDone := make(chan RelayStats, 1)
	go func() {
		stats, err := RelayTCP(ctx, inboundRelay, dialer, backend.Addr().String())
		statsDone <- stats
		relayDone <- err
	}()

	payload := []byte("end-to-end payload over TLS CONNECT")
	if _, err := inboundClient.Write(payload); err != nil {
		cancel()
		t.Fatal(err)
	}
	_ = inboundClient.SetReadDeadline(time.Now().Add(3 * time.Second))
	got := make([]byte, len(payload))
	if _, err := io.ReadFull(inboundClient, got); err != nil {
		cancel()
		t.Fatalf("read echoed payload through relay: %v", err)
	}
	if string(got) != string(payload) {
		cancel()
		t.Fatalf("echoed payload = %q, want %q", got, payload)
	}

	cancel()
	select {
	case err := <-relayDone:
		if err != nil && err != context.Canceled {
			t.Fatalf("RelayTCP returned %v after cancellation", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("relay did not stop after cancellation")
	}
	stats := <-statsDone
	if stats.InboundToOutbound != uint64(len(payload)) || stats.OutboundToInbound != uint64(len(payload)) {
		t.Fatalf("unexpected relay stats: %+v", stats)
	}
}
