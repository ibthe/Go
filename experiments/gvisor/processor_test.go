package main

import (
	"context"
	"errors"
	"io"
	"net"
	"testing"
	"time"

	"github.com/ibthe/Go/internal/packetengine"
	"gvisor.dev/gvisor/pkg/buffer"
	"gvisor.dev/gvisor/pkg/tcpip"
	"gvisor.dev/gvisor/pkg/tcpip/adapters/gonet"
	"gvisor.dev/gvisor/pkg/tcpip/network/ipv4"
	"gvisor.dev/gvisor/pkg/tcpip/stack"
)

type echoDialer struct {
	destinations chan string
}

func (d *echoDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	if network != "tcp" {
		return nil, net.UnknownNetworkError(network)
	}
	d.destinations <- address
	client, server := net.Pipe()
	go func() {
		defer server.Close()
		_, _ = io.Copy(server, client)
	}()
	return client, nil
}

func TestGVisorProcessorConnectsPacketEngineToTCPDialer(t *testing.T) {
	clientStack, clientLink := newStack(t,
		[4]byte{10, 77, 0, 1}, "\x02\x00\x00\x00\x00\x01",
		[4]byte{10, 77, 0, 2}, "\x02\x00\x00\x00\x00\x02",
	)
	defer clientStack.Destroy()

	dialer := &echoDialer{destinations: make(chan string, 1)}
	processor, err := NewGVisorProcessor(dialer)
	if err != nil {
		t.Fatal(err)
	}
	engine, err := packetengine.New(processor)
	if err != nil {
		t.Fatal(err)
	}
	if err := engine.Start(mtu); err != nil {
		t.Fatal(err)
	}
	defer engine.Stop()

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	pumpDone := make(chan error, 2)
	go func() {
		for {
			pkt := clientLink.ReadContext(ctx)
			if pkt == nil {
				pumpDone <- ctx.Err()
				return
			}
			protocol := pkt.NetworkProtocolNumber
			view := pkt.ToView()
			data := view.ToSlice()
			view.Release()
			pkt.DecRef()
			var family packetengine.Family
			switch protocol {
			case ipv4.ProtocolNumber:
				family = packetengine.FamilyIPv4
			default:
				continue
			}
			if err := engine.Process(data, family); err != nil {
				pumpDone <- err
				return
			}
		}
	}()
	go func() {
		for {
			packet, err := engine.ReadOutput(ctx)
			if err != nil {
				pumpDone <- err
				return
			}
			inbound := stack.NewPacketBuffer(stack.PacketBufferOptions{Payload: buffer.MakeWithData(packet.Data)})
			switch packet.Family {
			case packetengine.FamilyIPv4:
				inbound.NetworkProtocolNumber = ipv4.ProtocolNumber
			default:
				pumpDone <- errors.New("unexpected output family")
				return
			}
			clientStack.InjectInbound(inbound.NetworkProtocolNumber, inbound)
		}
	}()

	target := tcpip.FullAddress{Addr: tcpip.AddrFrom4([4]byte{10, 77, 0, 2}), Port: 8443}
	clientConn, err := gonet.DialContextTCP(ctx, clientStack, target, ipv4.ProtocolNumber)
	if err != nil {
		t.Fatalf("establish synthetic client flow: %v", err)
	}
	defer clientConn.Close()
	payload := []byte("packet engine to gVisor to proxy dialer")
	if _, err := clientConn.Write(payload); err != nil {
		t.Fatalf("write client payload: %v", err)
	}
	got := make([]byte, len(payload))
	if _, err := io.ReadFull(clientConn, got); err != nil {
		t.Fatalf("read proxied echo: %v", err)
	}
	if string(got) != string(payload) {
		t.Fatalf("echo = %q, want %q", got, payload)
	}
	select {
	case destination := <-dialer.destinations:
		if destination != "10.77.0.2:8443" {
			t.Fatalf("dial destination = %q", destination)
		}
	case <-ctx.Done():
		t.Fatal("TCP relay was not opened")
	}
}
