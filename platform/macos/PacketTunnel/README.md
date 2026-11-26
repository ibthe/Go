# macOS Packet Tunnel adapter

This directory contains the first native boundary for Apple's Network Extension packet flow.

## Current state

- `PacketTunnelFlowAdapter.swift` owns the asynchronous `NEPacketTunnelFlow` read/write loop.
- The adapter passes complete IP packets and their address-family metadata to a `GoPacketEngine` implementation.
- A Go engine implementation and the `NEPacketTunnelProvider` lifecycle owner are **not wired yet**.
- This source is not a runnable VPN and does not install routes or claim to capture device traffic.

## Integration contract

The future `NEPacketTunnelProvider` must:

1. Construct a concrete `GoPacketEngine` backed by the Go userspace stack.
2. Apply `NEPacketTunnelNetworkSettings` only after the engine has initialized successfully.
3. Construct `PacketTunnelFlowAdapter(packetFlow:engine:mtu:)` and call `start`.
4. Call `stop` from `stopTunnel(with:completionHandler:)`, then complete the provider shutdown.
5. Configure only routes and address families that the Go engine can actually process.

The Go implementation must treat packets as IP-layer data (no Ethernet header), preserve IPv4/IPv6 family information, process inbound packets synchronously, emit outbound packets asynchronously (TCP replies may arrive later), honor MTU limits, and avoid logging packet contents or destinations. A processing error drops that packet; it must never trigger direct-network fallback.

## Validation still required

This Swift source has not yet been compiled against an Apple SDK or exercised inside a signed Network Extension target. The next integration step is a concrete Go engine bridge (likely a narrow C ABI or cgo wrapper) plus an Xcode Packet Tunnel target with the Network Extension entitlement.