package tunnel

import (
	"context"
	"errors"
	"fmt"
	"net"
)

// RelayAcceptedTCP forwards an accepted TCP connection to the peer address
// represented by inbound.RemoteAddr. This is the adapter boundary for a
// userspace IP stack's accepted TCP connections: the stack owns packet parsing,
// TCP state and packet emission, while RelayAcceptedTCP owns the outbound
// CONNECT stream and byte relay.
func RelayAcceptedTCP(ctx context.Context, inbound net.Conn, dialer ContextDialer) (RelayStats, error) {
	if inbound == nil {
		return RelayStats{}, errors.New("inbound connection is required")
	}
	remote := inbound.RemoteAddr()
	if remote == nil {
		_ = inbound.Close()
		return RelayStats{}, errors.New("inbound remote address is required")
	}
	if _, ok := remote.(*net.TCPAddr); !ok {
		_ = inbound.Close()
		return RelayStats{}, fmt.Errorf("inbound remote address must be TCP, got %T", remote)
	}
	return RelayTCP(ctx, inbound, dialer, remote.String())
}
