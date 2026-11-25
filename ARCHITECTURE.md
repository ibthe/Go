# Architecture

## Design principles

1. The user-facing control is a single connect/disconnect switch.
2. Routing decisions are deterministic, local, testable, and independent of the tunnel implementation.
3. Proxy-required traffic must not silently fall back to direct routing when all configured proxy outbounds fail.
4. No browsing history or destination telemetry is collected by default.
5. Remote rule updates are data only: versioned, schema-validated, and signature-verified. Never execute downloaded code.
6. Platform integration is isolated behind adapters.

## Components

- **Routing core (Go):** hostname/IP classification and route policy.
- **Proxy manager:** health checks, ordered failover, connection lifecycle.
- **Tunnel engine:** packet capture and forwarding. This is not implemented yet.
- **Platform adapters:** macOS Network Extension, Windows tunnel/service, Android VpnService.
- **Rule updater:** signed rule bundles with rollback protection. Not implemented yet.
- **UI:** connection state, diagnostics, and explicit user-controlled exceptions.

## Traffic policy

The initial policy model has three actions: `proxy`, `direct`, and `block`. An unknown destination defaults to direct in the current routing library; the future tunnel controller must distinguish unknown traffic from destinations explicitly marked proxy-required and enforce fail-closed behavior for the latter.

DNS handling is a separate design decision. Do not resolve destinations through an untrusted remote control plane. DNS leak prevention, IPv6, UDP/QUIC, and captive-portal behavior require platform-level tests before claiming full-device coverage.

## Platform adapters

- **macOS:** packet tunnel via Apple's Network Extension APIs. Requires signing, provisioning, and the relevant entitlement. See [macOS Packet Tunnel Design](docs/MACOS_TUNNEL_DESIGN.md) for the packet-to-flow boundary, candidate userspace stack, and validation gates.
- **Windows:** evaluate Wintun plus a service and Windows Filtering Platform integration.
- **Android:** `VpnService`, foreground service lifecycle, and per-app routing controls where supported.

## Packet-to-stream boundary

The HTTPS CONNECT transport exposes TCP byte streams; it does not process IP packets. A packet tunnel therefore needs a userspace IP/TCP stack (or an equivalent packet-to-flow engine) between the platform packet interface and the Go proxy dialer. Do not wire raw packets directly to `proxytransport.Dialer` or claim full-device VPN functionality until this layer is implemented and tested.

gVisor netstack is a candidate for an isolated compatibility spike, not yet an adopted production dependency. Its API stability and exact upstream revision must be evaluated and pinned before integration.

## Proxy protocol compatibility

An endpoint written as `HTTPS host:port` in a PAC file commonly denotes an HTTP proxy connection protected by TLS. That does not by itself prove support for UDP, QUIC, SOCKS, or arbitrary IP tunnelling. Each endpoint must be tested for independent-client access and protocol support.

## Third-party infrastructure

No FastProxy endpoints, PAC dump, credentials, or third-party domain lists are committed. Reuse requires permission and independent protocol validation.
