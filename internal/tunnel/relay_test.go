package tunnel

import (
	"context"
	"errors"
	"io"
	"net"
	"strings"
	"testing"
	"time"
)

type fixedDialer struct {
	dial func(context.Context, string, string) (net.Conn, error)
}

func (d fixedDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	return d.dial(ctx, network, address)
}

func tcpPair(t *testing.T) (net.Conn, net.Conn) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	accepted := make(chan net.Conn, 1)
	go func() {
		c, err := ln.Accept()
		if err == nil {
			accepted <- c
		}
	}()
	client, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	server := <-accepted
	return client, server
}

func TestRelayTCPBidirectionalAndHalfClose(t *testing.T) {
	inboundClient, inboundRelay := tcpPair(t)
	defer inboundClient.Close()

	outboundClient, outboundServer := tcpPair(t)
	defer outboundServer.Close()

	dialer := fixedDialer{dial: func(context.Context, string, string) (net.Conn, error) {
		return outboundClient, nil
	}}

	relayDone := make(chan error, 1)
	statsDone := make(chan RelayStats, 1)
	go func() {
		stats, err := RelayTCP(context.Background(), inboundRelay, dialer, "example.test:443")
		statsDone <- stats
		relayDone <- err
	}()

	request := "request payload"
	if _, err := io.WriteString(inboundClient, request); err != nil {
		t.Fatal(err)
	}
	if err := inboundClient.(*net.TCPConn).CloseWrite(); err != nil {
		t.Fatal(err)
	}

	gotRequest, err := io.ReadAll(outboundServer)
	if err != nil {
		t.Fatal(err)
	}
	if string(gotRequest) != request {
		t.Fatalf("outbound received %q, want %q", gotRequest, request)
	}

	response := "response payload"
	if _, err := io.WriteString(outboundServer, response); err != nil {
		t.Fatal(err)
	}
	if err := outboundServer.(*net.TCPConn).CloseWrite(); err != nil {
		t.Fatal(err)
	}

	gotResponse, err := io.ReadAll(inboundClient)
	if err != nil {
		t.Fatal(err)
	}
	if string(gotResponse) != response {
		t.Fatalf("inbound received %q, want %q", gotResponse, response)
	}

	select {
	case err := <-relayDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("relay did not finish after both half-closes")
	}
	stats := <-statsDone
	if stats.InboundToOutbound != uint64(len(request)) || stats.OutboundToInbound != uint64(len(response)) {
		t.Fatalf("unexpected relay stats: %+v", stats)
	}
}

func TestRelayTCPDialFailureClosesInbound(t *testing.T) {
	client, inbound := tcpPair(t)
	defer client.Close()

	wantErr := errors.New("proxy unavailable")
	_, err := RelayTCP(context.Background(), inbound, fixedDialer{
		dial: func(context.Context, string, string) (net.Conn, error) {
			return nil, wantErr
		},
	}, "example.test:443")
	if !errors.Is(err, wantErr) {
		t.Fatalf("RelayTCP error = %v, want wrapped %v", err, wantErr)
	}
	_ = client.SetReadDeadline(time.Now().Add(time.Second))
	if _, err := client.Read(make([]byte, 1)); err == nil {
		t.Fatal("expected inbound peer to observe closed connection")
	}
}

func TestRelayTCPCancelUnblocksStreams(t *testing.T) {
	inboundClient, inboundRelay := tcpPair(t)
	defer inboundClient.Close()
	outboundClient, outboundServer := tcpPair(t)
	defer outboundServer.Close()

	ctx, cancel := context.WithCancel(context.Background())
	dialer := fixedDialer{dial: func(context.Context, string, string) (net.Conn, error) {
		return outboundClient, nil
	}}
	done := make(chan error, 1)
	go func() {
		_, err := RelayTCP(ctx, inboundRelay, dialer, "example.test:443")
		done <- err
	}()
	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("RelayTCP error = %v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("cancellation did not stop relay")
	}
}

func TestRelayTCPRejectsMissingInputs(t *testing.T) {
	_, inbound := tcpPair(t)
	defer inbound.Close()
	_, err := RelayTCP(context.Background(), inbound, nil, "example.test:443")
	if err == nil || !strings.Contains(err.Error(), "dialer") {
		t.Fatalf("expected missing dialer error, got %v", err)
	}
}
