// Package main contains an isolated gVisor netstack experiment.
// It is deliberately a nested module so its Go toolchain and dependencies do
// not affect the production application.
package main

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	"gvisor.dev/gvisor/pkg/tcpip"
	"gvisor.dev/gvisor/pkg/tcpip/link/channel"
	"gvisor.dev/gvisor/pkg/tcpip/network/ipv4"
	"gvisor.dev/gvisor/pkg/tcpip/stack"
	"gvisor.dev/gvisor/pkg/tcpip/transport/tcp"

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
			Prefix:    24,
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
func bridgePackets(ctx context.Context, a, b *channel.Endpoint) func() {
	var done = make(chan struct{}, 2)
	pump := func(src, dst *channel.Endpoint) {
		defer func() { done <- struct{}{} }()
		for {
			pkt := src.ReadContext(ctx)
			if pkt == nil {
				return
			}
			dst.InjectInbound(ipv4.ProtocolNumber, pkt)
		}
	}
	go pump(a, b)
	go pump(b, a)
	return func() {
		<-done
		<-done
	}
}

type dialFunc func(context.Context, string, string) (net.Conn, error)

func (f dialFunc) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	return f(ctx, network, address)
}

func TestIPv4TCPFlowBridgesIntoGoRelay(t *testing.T) {
	serverStack, serverLink := newStack(t, [4]byte{10, 77, 0, 2})
	clientStack, clientLink := newStack(t, [4]byte{10, 77, 0, 1})
	ctx, cancel := context.WithCancel(context.Background())
	waitBridge := bridgePackets(ctx, clientLink, serverLink)
	t.Cleanup(func() {
		cancel()
		waitBridge()
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
						_, _ = conn.Write(buf[:n])
					}
					if err != nil {
						return
					}
				}
			}()
		}
	}()

	listener, err := gonetListenTCP(serverStack, tcpip.FullAddress{
		Addr: tcpip.AddrFrom4([4]byte{10, 77, 0, 2}),
		Port: 8443,
	})
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
		_, err = tunnel.RelayAcceptedTCP(ctx, inbound, dialFunc(func(ctx context.Context, network, address string) (net.Conn, error) {
			if network != "tcp" {
				return nil, fmt.Errorf("unexpected network %q", network)
			}
			// RelayAcceptedTCP must use the destination address from the
			// accepted flow, not the remote client's source address.
			if address != "10.77.0.2:8443" {
				return nil, fmt.Errorf("wrong original destination %q", address)
			}
			return (&net.Dialer{}).DialContext(ctx, "tcp", backend.Addr().String())
		}))
		relayDone <- err
	}()

	clientConn, err := gonetDialTCP(ctx, clientStack, tcpip.FullAddress{
		Addr: tcpip.AddrFrom4([4]byte{10, 77, 0, 2}),
		Port: 8443,
	})
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
