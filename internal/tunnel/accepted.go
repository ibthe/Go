package tunnel

import (
	"context"
	"errors"
	"fmt"
	"net"
)

// RelayAcceptedTCP forwards an accepted TCP connection to the peer address
// represented by inbound.LocalAddr. For an accepted inbound flow, LocalAddr
// is the original packet destination; RemoteAddr is the source peer. This is
// the adapter boundary for a userspace IP stack: the stack owns TCP state
// and packet emission, while RelayAcceptedTCP owns the outbound CONNECT
// stream and byte relay.
func RelayAcceptedTCP(ctx context.Context, inbound net.Conn, dialer ContextDialer) (RelayStats, error) {
	if inbound == nil {
		return RelayStats{}, errors.New("inbound connection is required")
	}
	destination := inbound.LocalAddr()
	if destination == nil {
		_ = inbound.Close()
		return RelayStats{}, errors.New("inbound local destination address is required")
	}
	if _, ok := destination.(*net.TCPAddr); !ok {
		_ = inbound.Close()
		return RelayStats{}, fmt.Errorf("inbound local destination address must be TCP, got %T", destination)
	}
	return RelayTCP(ctx, inbound, dialer, destination.String())
}
