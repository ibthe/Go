package tunnel

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"
)

func TestRelayAcceptedTCPUsesOriginalPeerAddress(t *testing.T) {
	inboundClient, inboundRelay := tcpPair(t)
	defer inboundClient.Close()
	outboundClient, outboundServer := tcpPair(t)
	defer outboundServer.Close()

	wantDestination := inboundRelay.LocalAddr().String()
	gotDestination := make(chan string, 1)
	ctx, cancel := context.WithCancel(context.Background())
	dialer := fixedDialer{dial: func(_ context.Context, network, address string) (net.Conn, error) {
		if network != "tcp" {
			t.Errorf("network = %q, want tcp", network)
		}
		gotDestination <- address
		return outboundClient, nil
	}}

	done := make(chan error, 1)
	go func() {
		_, err := RelayAcceptedTCP(ctx, inboundRelay, dialer)
		done <- err
	}()

	select {
	case got := <-gotDestination:
		if got != wantDestination {
			t.Fatalf("destination = %q, want original peer %q", got, wantDestination)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("dialer was not called")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("RelayAcceptedTCP error = %v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("relay did not stop after cancellation")
	}
}

func TestRelayAcceptedTCPRejectsNilConnection(t *testing.T) {
	if _, err := RelayAcceptedTCP(context.Background(), nil, fixedDialer{}); err == nil {
		t.Fatal("expected error for nil inbound connection")
	}
}
