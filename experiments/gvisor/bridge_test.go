// Package main contains an isolated gVisor netstack experiment.
// It is deliberately a nested module so its Go toolchain and dependencies do
// not affect the production application.
package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
	"sync/atomic"

	"gvisor.dev/gvisor/pkg/tcpip"
	"gvisor.dev/gvisor/pkg/tcpip/adapters/gonet"
	"gvisor.dev/gvisor/pkg/tcpip/link/channel"
	"gvisor.dev/gvisor/pkg/tcpip/network/ipv4"
	"gvisor.dev/gvisor/pkg/tcpip/stack"
	"gvisor.dev/gvisor/pkg/tcpip/transport/tcp"

	"github.com/ibthe/Go/internal/proxytransport"
	"github.com/ibthe/Go/internal/tunnel"
)

const (
	nicID = tcpip.NICID(1)
	mtu   = 1500
)

func newStack(t *testing.T, address [4]byte) (*stack.Stack, *channel.Endpoint) {
	t.Helper()
	link := channel.New(1024, mtu, "")
	s := stack.New(stack.Options{
		NetworkProtocols:   []stack.NetworkProtocolFactory{ipv4.NewProtocol},
		TransportProtocols: []stack.TransportProtocolFactory{tcp.NewProtocol},
	})
	if err := s.CreateNIC(nicID, link); err != nil {
		s.Destroy()
		t.Fatalf("create gVisor NIC: %s", err)
	}
	protocolAddress := tcpip.ProtocolAddress{
		Protocol: ipv4.ProtocolNumber,
		AddressWithPrefix: tcpip.AddressWithPrefix{
			Address:   tcpip.AddrFrom4(address),
			PrefixLen: 24,
		},
	}
	if err := s.AddProtocolAddress(nicID, protocolAddress, stack.AddressProperties{}); err != nil {
		s.Destroy()
		t.Fatalf("assign gVisor IPv4 address: %s", err)
	}
	subnet, err := tcpip.NewSubnet(
		tcpip.AddrFrom4([4]byte{10, 77, 0, 0}),
		tcpip.MaskFrom("\xff\xff\xff\x00"),
	)
	if err != nil {
		s.Destroy()
		t.Fatalf("create test subnet: %v", err)
	}
	s.SetRouteTable([]tcpip.Route{{Destination: subnet, NIC: nicID}})
	return s, link
}

// bridgePackets connects two in-memory IP interfaces. Packets produced by one
// stack are injected into the other stack without parsing or rewriting them.
func bridgePackets(ctx context.Context, a, b *channel.Endpoint) func() (uint64, uint64) {
	done := make(chan struct{}, 2)
	var aToB, bToA atomic.Uint64
	pump := func(src, dst *channel.Endpoint, count *atomic.Uint64) {
		defer func() { done <- struct{}{} }()
		for {
			pkt := src.ReadContext(ctx)
			if pkt == nil {
				return
			}
			count.Add(1)
			dst.InjectInbound(ipv4.ProtocolNumber, pkt)
		}
	}
	go pump(a, b, &aToB)
	go pump(b, a, &bToA)
	return func() (uint64, uint64) {
		<-done
		<-done
		return aToB.Load(), bToA.Load()
	}
}

func TestIPv4TCPFlowBridgesIntoGoRelay(t *testing.T) {
	serverStack, serverLink := newStack(t, [4]byte{10, 77, 0, 2})
	clientStack, clientLink := newStack(t, [4]byte{10, 77, 0, 1})
	ctx, cancel := context.WithCancel(context.Background())
	waitBridge := bridgePackets(ctx, clientLink, serverLink)
	t.Cleanup(func() {
		cancel()
		aToB, bToA := waitBridge()
		t.Logf("gVisor packet exchange: client-to-server=%d server-to-client=%d", aToB, bToA)
		clientStack.Destroy()
		serverStack.Destroy()
	})

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
				buf := make([]byte, 4096)
				for {
					n, err := conn.Read(buf)
					if n > 0 {
						if _, writeErr := conn.Write(buf[:n]); writeErr != nil {
							return
						}
					}
					if err != nil {
						return
					}
				}
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
		if r.Host != "10.77.0.2:8443" {
			http.Error(w, "unexpected CONNECT destination", http.StatusBadGateway)
			return
		}
		upstream, err := net.Dial("tcp", backend.Addr().String())
		if err != nil {
			_, _ = io.WriteString(rw, "HTTP/1.1 502 Bad Gateway\r\nConnection: close\r\n\r\n")
			_ = rw.Flush()
			_ = clientConn.Close()
			return
		}
		if _, err := io.WriteString(rw, "HTTP/1.1 200 Connection Established\r\n\r\n"); err != nil {
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

	listener, err := gonet.ListenTCP(serverStack, tcpip.FullAddress{
		Addr: tcpip.AddrFrom4([4]byte{10, 77, 0, 2}),
		Port: 8443,
	}, ipv4.ProtocolNumber)
	if err != nil {
		t.Fatalf("listen on userspace stack: %v", err)
	}
	defer listener.Close()

	relayDone := make(chan error, 1)
	go func() {
		inbound, err := listener.Accept()
		if err != nil {
			relayDone <- err
			return
		}
		// The CONNECT target must be the accepted flow's original local
		// destination, not the client's source address.
		_, err = tunnel.RelayAcceptedTCP(ctx, inbound, dialer)
		relayDone <- err
	}()

	dialCtx, cancelDial := context.WithTimeout(ctx, 5*time.Second)
	defer cancelDial()
	clientConn, err := gonet.DialContextTCP(dialCtx, clientStack, tcpip.FullAddress{
		Addr: tcpip.AddrFrom4([4]byte{10, 77, 0, 2}),
		Port: 8443,
	}, ipv4.ProtocolNumber)
	if err != nil {
		t.Fatalf("dial through client userspace stack: %v", err)
	}
	defer clientConn.Close()

	payload := []byte("synthetic IPv4/TCP packet -> gVisor -> net.Conn -> Go relay")
	if err := clientConn.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := clientConn.Write(payload); err != nil {
		t.Fatalf("write through gVisor TCP: %v", err)
	}
	got := make([]byte, len(payload))
	if _, err := io.ReadFull(clientConn, got); err != nil {
		t.Fatalf("read relayed response: %v", err)
	}
	if string(got) != string(payload) {
		t.Fatalf("response = %q, want %q", got, payload)
	}

	cancel()
	select {
	case err := <-relayDone:
		if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, net.ErrClosed) {
			t.Fatalf("relay stopped with error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("relay did not stop after cancellation")
	}
}
