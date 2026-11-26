package main

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/ibthe/Go/internal/packetengine"
	"github.com/ibthe/Go/internal/tunnel"
	"gvisor.dev/gvisor/pkg/buffer"
	"gvisor.dev/gvisor/pkg/tcpip"
	"gvisor.dev/gvisor/pkg/tcpip/adapters/gonet"
	"gvisor.dev/gvisor/pkg/tcpip/header"
	"gvisor.dev/gvisor/pkg/tcpip/link/channel"
	"gvisor.dev/gvisor/pkg/tcpip/network/ipv4"
	"gvisor.dev/gvisor/pkg/tcpip/network/ipv6"
	"gvisor.dev/gvisor/pkg/tcpip/stack"
	"gvisor.dev/gvisor/pkg/tcpip/transport/tcp"
	"gvisor.dev/gvisor/pkg/waiter"
)

const processorNIC = tcpip.NICID(1)

// GVisorProcessor adapts complete IP packets to TCP streams and sends accepted
// flows through the supplied dialer. The dialer should normally be a proxypool.Pool.
type GVisorProcessor struct {
	dialer tunnel.ContextDialer

	processMu sync.Mutex
	mu        sync.RWMutex
	flowMu    sync.Mutex
	stopping  bool
	running   bool
	mtu       int
	stack     *stack.Stack
	link      *channel.Endpoint
	ctx       context.Context
	cancel    context.CancelFunc
	emit      func(packetengine.Packet) error
	flows     sync.WaitGroup
	outputs   sync.WaitGroup
	lastErr   atomic.Value
}

func NewGVisorProcessor(dialer tunnel.ContextDialer) (*GVisorProcessor, error) {
	if dialer == nil {
		return nil, errors.New("TCP dialer is required; direct fallback is not supported")
	}
	return &GVisorProcessor{dialer: dialer}, nil
}

func (p *GVisorProcessor) Start(mtu int, emit func(packetengine.Packet) error) error {
	p.processMu.Lock()
	defer p.processMu.Unlock()
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.running {
		return nil
	}
	if mtu < 1280 || mtu > 65535 {
		return fmt.Errorf("invalid MTU %d", mtu)
	}
	if emit == nil {
		return errors.New("packet output callback is required")
	}

	ctx, cancel := context.WithCancel(context.Background())
	link := channel.New(1024, uint32(mtu), "")
	s := stack.New(stack.Options{
		NetworkProtocols:   []stack.NetworkProtocolFactory{ipv4.NewProtocol, ipv6.NewProtocol},
		TransportProtocols: []stack.TransportProtocolFactory{tcp.NewProtocol},
	})
	forwarder := tcp.NewForwarder(s, 0, 1024, p.handleTCP)
	// Install the handler before enabling the NIC so the first SYN cannot race
	// with transport-handler registration.
	s.SetTransportProtocolHandler(tcp.ProtocolNumber, forwarder.HandlePacket)
	if err := s.CreateNIC(processorNIC, link); err != nil {
		cancel()
		s.Destroy()
		return fmt.Errorf("create gVisor NIC: %s", err)
	}
	for _, protocol := range []tcpip.NetworkProtocolNumber{ipv4.ProtocolNumber, ipv6.ProtocolNumber} {
		if _, err := s.SetNICForwarding(processorNIC, protocol, true); err != nil {
			cancel()
			s.Destroy()
			return fmt.Errorf("enable gVisor forwarding: %s", err)
		}
	}
	if err := s.SetSpoofing(processorNIC, true); err != nil {
		cancel()
		s.Destroy()
		return fmt.Errorf("enable gVisor spoofing: %s", err)
	}
	if err := s.SetPromiscuousMode(processorNIC, true); err != nil {
		cancel()
		s.Destroy()
		return fmt.Errorf("enable gVisor promiscuous mode: %s", err)
	}
	s.SetRouteTable([]tcpip.Route{
		{Destination: header.IPv4EmptySubnet, NIC: processorNIC},
		{Destination: header.IPv6EmptySubnet, NIC: processorNIC},
	})

	p.ctx, p.cancel, p.stack, p.link = ctx, cancel, s, link
	p.mtu, p.emit = mtu, emit
	p.stopping = false
	p.running = true
	p.lastErr.Store(errorBox{})
	p.outputs.Add(1)
	go p.readOutput(ctx, link, emit)
	return nil
}

type errorBox struct{ err error }

func (p *GVisorProcessor) ProcessPacket(packet []byte, family packetengine.Family) error {
	p.processMu.Lock()
	defer p.processMu.Unlock()

	p.mu.RLock()
	if !p.running {
		p.mu.RUnlock()
		return errors.New("gVisor processor is not running")
	}
	s, mtu := p.stack, p.mtu
	p.mu.RUnlock()

	if len(packet) == 0 || len(packet) > mtu {
		return errors.New("invalid IP packet size")
	}
	var protocol tcpip.NetworkProtocolNumber
	switch family {
	case packetengine.FamilyIPv4:
		protocol = ipv4.ProtocolNumber
		if packet[0]>>4 != 4 {
			return errors.New("IPv4 family does not match packet version")
		}
	case packetengine.FamilyIPv6:
		protocol = ipv6.ProtocolNumber
		if packet[0]>>4 != 6 {
			return errors.New("IPv6 family does not match packet version")
		}
	default:
		return errors.New("unsupported IP family")
	}
	if value := p.lastErr.Load().(errorBox); value.err != nil {
		return fmt.Errorf("gVisor output stopped: %w", value.err)
	}

	inbound := stack.NewPacketBuffer(stack.PacketBufferOptions{Payload: buffer.MakeWithData(append([]byte(nil), packet...))})
	inbound.NetworkProtocolNumber = protocol
	p.link.InjectInbound(protocol, inbound)
	return nil
}

func (p *GVisorProcessor) handleTCP(request *tcp.ForwarderRequest) {
	p.flowMu.Lock()
	if p.stopping {
		p.flowMu.Unlock()
		request.Complete(true)
		return
	}
	p.flows.Add(1)
	p.flowMu.Unlock()

	go func() {
		defer p.flows.Done()
		var queue waiter.Queue
		ep, err := request.CreateEndpoint(&queue)
		if err != nil {
			request.Complete(true)
			return
		}
		request.Complete(false)
		conn := gonet.NewTCPConn(&queue, ep)
		defer conn.Close()
		_, _ = tunnel.RelayAcceptedTCP(p.ctx, conn, p.dialer)
	}()
}

func (p *GVisorProcessor) readOutput(ctx context.Context, link *channel.Endpoint, emit func(packetengine.Packet) error) {
	defer p.outputs.Done()
	for {
		pkt := link.ReadContext(ctx)
		if pkt == nil {
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
		case ipv6.ProtocolNumber:
			family = packetengine.FamilyIPv6
		default:
			continue
		}
		if err := emit(packetengine.Packet{Data: data, Family: family}); err != nil {
			p.lastErr.Store(errorBox{err: err})
			p.mu.RLock()
			cancel := p.cancel
			p.mu.RUnlock()
			if cancel != nil {
				cancel()
			}
			return
		}
	}
}

func (p *GVisorProcessor) Stop() error {
	p.processMu.Lock()
	defer p.processMu.Unlock()

	p.mu.Lock()
	if !p.running {
		p.mu.Unlock()
		return nil
	}
	p.running = false
	cancel, s := p.cancel, p.stack
	p.mu.Unlock()

	p.flowMu.Lock()
	p.stopping = true
	p.flowMu.Unlock()
	cancel()
	s.Destroy()
	p.outputs.Wait()
	p.flows.Wait()

	p.mu.Lock()
	p.stack, p.link, p.ctx, p.cancel, p.emit = nil, nil, nil, nil, nil
	p.mtu = 0
	p.mu.Unlock()
	return nil
}

