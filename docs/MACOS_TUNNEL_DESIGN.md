# macOS Packet Tunnel Design

## Status

The transport-side TCP relay is implemented and covered by unit and end-to-end tests. `internal/tunnel.RelayAcceptedTCP` now maps an accepted TCP connection's peer address to the HTTPS CONNECT destination, while `RelayTCP` handles bidirectional stream copying and lifecycle. This is the adapter boundary a userspace IP stack can call.

The packet-to-flow engine and macOS Network Extension are not implemented. This repository does not yet contain a working macOS VPN.

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

Build the gVisor compatibility spike as an isolated Go module, pinned to an exact upstream revision and kept out of the production dependency graph. Prove packet injection, TCP forwarding through `RelayAcceptedTCP`, response packet emission, and shutdown in CI before adding the Swift Packet Tunnel Provider target and its signing configuration.
