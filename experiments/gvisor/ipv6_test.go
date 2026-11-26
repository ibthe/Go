package main

import (
	"context"
	"io"
	"net"
	"testing"
	"time"

	"gvisor.dev/gvisor/pkg/tcpip"
	"gvisor.dev/gvisor/pkg/tcpip/adapters/gonet"
	"gvisor.dev/gvisor/pkg/tcpip/link/channel"
	"gvisor.dev/gvisor/pkg/tcpip/network/ipv6"
	"gvisor.dev/gvisor/pkg/tcpip/stack"
	"gvisor.dev/gvisor/pkg/tcpip/transport/tcp"

	"github.com/ibthe/Go/internal/tunnel"
)

var (
	ipv6ClientAddr = tcpip.AddrFrom16([16]byte{0xfd, 0x77, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1})
	ipv6ServerAddr = tcpip.AddrFrom16([16]byte{0xfd, 0x77, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 2})
)

func newIPv6Stack(t *testing.T, address, peerAddress tcpip.Address, mac, peerMAC string) (*stack.Stack, *channel.Endpoint) {
	t.Helper()
	link := channel.New(1024, mtu, tcpip.LinkAddress(mac))
	link.LinkEPCapabilities = stack.CapabilityResolutionRequired
	s := stack.New(stack.Options{
		NetworkProtocols:   []stack.NetworkProtocolFactory{ipv6.NewProtocol},
		TransportProtocols: []stack.TransportProtocolFactory{tcp.NewProtocol},
	})
	if err := s.CreateNIC(nicID, link); err != nil {
		s.Destroy()
		t.Fatalf("create IPv6 gVisor NIC: %s", err)
	}
	if err := s.AddProtocolAddress(nicID, tcpip.ProtocolAddress{
		Protocol: ipv6.ProtocolNumber,
		AddressWithPrefix: tcpip.AddressWithPrefix{
			Address: address, PrefixLen: 64,
		},
	}, stack.AddressProperties{}); err != nil {
		s.Destroy()
		t.Fatalf("assign IPv6 address: %s", err)
	}
	subnet, err := tcpip.NewSubnet(
		tcpip.AddrFrom16([16]byte{0xfd, 0x77}),
		tcpip.MaskFrom("\xff\xff\xff\xff\xff\xff\xff\xff\x00\x00\x00\x00\x00\x00\x00\x00"),
	)
	if err != nil {
		s.Destroy()
		t.Fatalf("create IPv6 test subnet: %v", err)
	}
	s.SetRouteTable([]tcpip.Route{{Destination: subnet, NIC: nicID}})
	if err := s.AddStaticNeighbor(nicID, ipv6.ProtocolNumber, peerAddress, tcpip.LinkAddress(peerMAC)); err != nil {
		s.Destroy()
		t.Fatalf("add IPv6 static neighbor: %s", err)
	}
	return s, link
}

type pipeDialer struct {
	destination chan string
}

func (d *pipeDialer) DialContext(_ context.Context, network, address string) (net.Conn, error) {
	if network != "tcp" {
		return nil, net.UnknownNetworkError(network)
	}
	d.destination <- address
	client, server := net.Pipe()
	go func() {
		defer server.Close()
		buf := make([]byte, 4096)
		for {
			n, err := server.Read(buf)
			if n > 0 {
				if _, writeErr := server.Write(buf[:n]); writeErr != nil {
					return
				}
			}
			if err != nil {
				return
			}
		}
	}()
	return client, nil
}

func TestIPv6TCPFlowBridgesIntoGoRelay(t *testing.T) {
	serverStack, serverLink := newIPv6Stack(t, ipv6ServerAddr, ipv6ClientAddr,
		"\x02\x00\x00\x00\x00\x12", "\x02\x00\x00\x00\x00\x11")
	clientStack, clientLink := newIPv6Stack(t, ipv6ClientAddr, ipv6ServerAddr,
		"\x02\x00\x00\x00\x00\x11", "\x02\x00\x00\x00\x00\x12")
	ctx, cancel := context.WithCancel(context.Background())
	waitBridge := bridgePackets(ctx, clientLink, serverLink)
	t.Cleanup(func() {
		cancel()
		aToB, bToA := waitBridge()
		t.Logf("gVisor IPv6 packet exchange: client-to-server=%d server-to-client=%d", aToB, bToA)
		clientStack.Destroy()
		serverStack.Destroy()
	})

	listener, err := gonet.ListenTCP(serverStack, tcpip.FullAddress{
		Addr: ipv6ServerAddr,
		Port: 8443,
	}, ipv6.ProtocolNumber)
	if err != nil {
		t.Fatalf("listen on IPv6 userspace stack: %v", err)
	}
	defer listener.Close()

	dialer := &pipeDialer{destination: make(chan string, 1)}
	relayDone := make(chan error, 1)
	go func() {
		inbound, err := listener.Accept()
		if err != nil {
			relayDone <- err
			return
		}
		_, err = tunnel.RelayAcceptedTCP(ctx, inbound, dialer)
		relayDone <- err
	}()

	dialCtx, cancelDial := context.WithTimeout(ctx, 5*time.Second)
	defer cancelDial()
	clientConn, err := gonet.DialContextTCP(dialCtx, clientStack, tcpip.FullAddress{
		Addr: ipv6ServerAddr,
		Port: 8443,
	}, ipv6.ProtocolNumber)
	if err != nil {
		t.Fatalf("dial through IPv6 userspace stack: %v", err)
	}
	defer clientConn.Close()
	if err := clientConn.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatal(err)
	}

	payload := []byte("synthetic IPv6/TCP packet -> gVisor -> net.Conn -> Go relay")
	if _, err := clientConn.Write(payload); err != nil {
		t.Fatalf("write through gVisor IPv6 TCP: %v", err)
	}
	got := make([]byte, len(payload))
	if _, err := io.ReadFull(clientConn, got); err != nil {
		t.Fatalf("read relayed IPv6 response: %v", err)
	}
	if string(got) != string(payload) {
		t.Fatalf("response = %q, want %q", got, payload)
	}

	select {
	case destination := <-dialer.destination:
		if destination != "[fd77::2]:8443" {
			t.Fatalf("relay destination = %q, want [fd77::2]:8443", destination)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("relay did not dial the original IPv6 destination")
	}

	cancel()
	select {
	case err := <-relayDone:
		if err != nil && err != context.Canceled && err != net.ErrClosed {
			t.Fatalf("IPv6 relay stopped with error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("IPv6 relay did not stop after cancellation")
	}
}
