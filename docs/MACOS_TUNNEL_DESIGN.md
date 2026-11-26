# macOS Packet Tunnel Design

## Status

The transport-side TCP relay is implemented and covered by unit and HTTPS CONNECT integration tests. `internal/tunnel.RelayAcceptedTCP` maps an accepted flow's **local** address (the original destination IP:port) to the outbound CONNECT destination; its remote address is the originating client and must not be used for routing.

An isolated gVisor experiment now lives in `experiments/gvisor`, with its own Go module and a pinned upstream revision. Tests now connect two in-memory gVisor IPv4/TCP and IPv6/TCP stacks, transfer packets between their channel endpoints, accept resulting flows as `net.Conn`, and pass them through the Go relay. The IPv4 test exercises a local TLS HTTPS CONNECT proxy; the IPv6 test verifies preservation of the original bracketed IPv6 destination. A dedicated workflow uses the newer Go toolchain required by gVisor. This remains a prototype, not the production packet engine.

A first native boundary has also been added at `platform/macos/PacketTunnel/PacketTunnelFlowAdapter.swift`. It pumps complete IP packets between `NEPacketTunnelFlow` and a `GoPacketEngine` protocol, serializes packet processing, and drops packets on engine errors rather than bypassing the tunnel. It is deliberately not yet wired to a Go implementation or a signed `NEPacketTunnelProvider` target.

## Why a packet-to-stream adapter is required

Apple's `NEPacketTunnelProvider` receives IP packets from `NEPacketTunnelFlow`. The current Go `proxytransport.Dialer` accepts a destination and creates a TCP byte stream through an HTTPS CONNECT proxy. These are different abstraction levels: a packet tunnel cannot pass an IP packet directly to `DialContext`.

A userspace network stack (or an equivalent packet-to-flow engine) must terminate the TCP state machine, reassemble payloads, handle retransmissions, and expose accepted TCP flows as `net.Conn` values. Go's existing HTTPS CONNECT dialer can then be used as the outbound dial function for those flows.

## Proposed data path

```text
macOS applications
      |
      v
NEPacketTunnelProvider / NEPacketTunnelFlow
      |
      | IPv4 / IPv6 packets
      v
Packet adapter (Swift <-> Go boundary)
      |
      v
Userspace IP stack
  - IPv4 and IPv6
  - TCP connection tracking and reassembly
  - TCP forwarder / accepted net.Conn
      |
      v
Go routing policy
      |
      +---- direct TCP dial (only for direct-policy destinations)
      |
      +---- HTTPS CONNECT Dialer (proxy-policy destinations)
      |
      +---- reject (block-policy destinations)
```

This is a target architecture, not an implemented or validated data path.

## Candidate userspace stack

gVisor netstack is a candidate because it is a reusable userspace network stack and provides Go network connection adapters. Its own documentation notes that its API is not guaranteed to be stable and is not published with Go module-style versions. Before adopting it, run a small isolated spike and pin an exact upstream revision. Do not make the main app depend on an unpinned moving branch.

Spike acceptance criteria:

1. Inject synthetic IPv4 TCP packets into the stack and receive a TCP flow.
2. Verify TCP handshake, payload reassembly, half-close, reset, and idle timeout behavior.
3. Relay the flow through a local HTTPS CONNECT test proxy using `proxytransport.Dialer` and `internal/tunnel.RelayAcceptedTCP`.
4. Return response bytes as correctly formed IP packets through the packet endpoint.
5. Add IPv6 tests before enabling IPv6 routes in the system tunnel.
6. Verify cancellation and resource cleanup when the provider stops or a proxy fails.
7. Run race-enabled tests where supported and record the pinned upstream revision and license.

## DNS, UDP, QUIC, and policy

TCP-over-HTTPS CONNECT does not carry arbitrary UDP. The first implementation must not claim support for UDP, QUIC, or complete DNS leak prevention.

DNS needs an explicit design: either handle DNS inside the tunnel with a defined resolver policy, or route DNS through a separately validated mechanism. Do not silently send DNS outside the tunnel while claiming leak protection.

Until UDP forwarding is implemented and tested, the provider must not install routes that imply unsupported UDP traffic is safely handled. The product must clearly report the limited protocol coverage.

Routing policy must be applied to the destination before selecting an outbound. Proxy-required traffic must fail closed if no proxy is usable; it must never silently fall back to direct. Direct routing is permitted only when the local policy explicitly selects it.

## macOS integration boundary

The native extension is responsible for:

- Implementing the `NEPacketTunnelProvider` lifecycle.
- Applying tunnel network settings only after the packet engine is ready.
- Reading and writing packets through `NEPacketTunnelFlow`.
- Starting and stopping the Go engine with deterministic cleanup.
- Reporting provider state and actionable errors to the containing app.

The Go engine is responsible for:

- Packet-to-flow processing.
- Routing decisions and outbound selection.
- Proxy transport and connection lifecycle.
- Avoiding destination logging by default.

The macOS target requires the Network Extension packet-tunnel entitlement, an App Extension target with the packet-tunnel extension point, signing, and provisioning. Those Apple project and account settings cannot be validated by ordinary Go unit tests.

## Security and failure behavior

- Never disable TLS certificate verification for proxy connections in production.
- Do not log proxy credentials, full URLs containing credentials, or browsing destinations.
- On provider stop, close accepted flows, outbound connections, packet endpoints, and goroutines.
- Do not report the tunnel as connected until packet processing is initialized.
- If a proxy-required flow cannot be established, reject it rather than bypassing the proxy.
- Keep third-party endpoints and rule data out of the public repository unless permission and provenance are established.

## Next implementation milestone

Implement the concrete `GoPacketEngine` behind the Swift adapter and connect it to the gVisor packet endpoint through a narrow, testable Go/C boundary. Before installing system routes, validate packet ownership, checksum handling, resets, half-close, cancellation, MTU behavior, and IPv4/IPv6 round trips. Keep gVisor isolated until its API and performance are validated. Then add the signed Swift Packet Tunnel Provider target and configure only the routes the engine can safely carry.
