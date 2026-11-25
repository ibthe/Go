// Package tunnel contains transport-level building blocks for the packet tunnel.
package tunnel

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
)

// ContextDialer is implemented by proxytransport.Dialer and compatible dialers.
type ContextDialer interface {
	DialContext(ctx context.Context, network, address string) (net.Conn, error)
}

// RelayStats reports payload bytes copied in each direction.
type RelayStats struct {
	InboundToOutbound uint64
	OutboundToInbound uint64
}

// RelayTCP connects an accepted inbound TCP stream to destination through dialer.
// It preserves TCP half-close semantics where the underlying connections support
// CloseWrite. The caller retains ownership of inbound; both streams are closed
// when the relay exits.
func RelayTCP(ctx context.Context, inbound net.Conn, dialer ContextDialer, destination string) (RelayStats, error) {
	var stats RelayStats
	if inbound == nil {
		return stats, errors.New("inbound connection is required")
	}
	if dialer == nil {
		_ = inbound.Close()
		return stats, errors.New("outbound dialer is required")
	}
	if destination == "" {
		_ = inbound.Close()
		return stats, errors.New("destination is required")
	}

	outbound, err := dialer.DialContext(ctx, "tcp", destination)
	if err != nil {
		_ = inbound.Close()
		return stats, fmt.Errorf("dial outbound: %w", err)
	}

	done := make(chan struct{})
	var closeOnce sync.Once
	closeBoth := func() {
		closeOnce.Do(func() {
			_ = inbound.Close()
			_ = outbound.Close()
		})
	}
	defer close(done)
	defer closeBoth()

	// Cancellation must interrupt blocked io.Copy calls.
	go func() {
		select {
		case <-ctx.Done():
			closeBoth()
		case <-done:
		}
	}()

	var inToOut, outToIn atomic.Uint64
	type copyResult struct {
		err error
	}
	results := make(chan copyResult, 2)

	copyOneWay := func(dst, src net.Conn, count *atomic.Uint64) {
		n, copyErr := io.Copy(dst, src)
		count.Store(uint64(n))
		if cw, ok := dst.(interface{ CloseWrite() error }); ok {
			if err := cw.CloseWrite(); copyErr == nil {
				copyErr = err
			}
		}
		results <- copyResult{err: copyErr}
	}

	go copyOneWay(outbound, inbound, &inToOut)
	go copyOneWay(inbound, outbound, &outToIn)

	var firstErr error
	for i := 0; i < 2; i++ {
		result := <-results
		if result.err != nil && firstErr == nil {
			firstErr = result.err
			closeBoth()
		}
	}
	stats.InboundToOutbound = inToOut.Load()
	stats.OutboundToInbound = outToIn.Load()

	if err := ctx.Err(); err != nil {
		return stats, err
	}
	if firstErr != nil {
		return stats, fmt.Errorf("relay TCP stream: %w", firstErr)
	}
	return stats, nil
}
