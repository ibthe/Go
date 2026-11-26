# macOS Packet Tunnel adapter

This directory contains the first native boundary for Apple's Network Extension packet flow.

## Current state

- `PacketTunnelFlowAdapter.swift` owns the asynchronous `NEPacketTunnelFlow` read/write loop.
- `GoPacketEngineCABI.swift` implements the Go packet-engine C ABI with Swift-owned buffers, a bounded output reader, and joined shutdown.
- The adapter passes complete IP packets and their address-family metadata to a `GoPacketEngine` implementation.
- The gVisor-backed processor exists only in the isolated `experiments/gvisor` module and is exported through a C ABI. The native shared library is not yet linked into an app target.
- `ProxyCredentialStore.swift` provides a Keychain-backed helper for proxy usernames/passwords, but no app target or settings UI calls it yet.
- `GoPacketTunnelProvider.swift` now owns the experimental Network Extension start/stop lifecycle and connects the C ABI engine to the packet-flow adapter.
- The provider deliberately configures IPv4/IPv6 interface addresses with **empty included-route lists**. It is a lifecycle smoke-test scaffold and does not redirect normal device traffic.
- This source is still not a runnable VPN: there is no Xcode app/extension target, signed entitlement/profile, linked Go dylib, or device validation. The smoke-test provider intentionally installs no traffic routes.

## Integration contract

The future `NEPacketTunnelProvider` must:

1. Construct a concrete `GoPacketEngine` backed by the Go userspace stack.
2. Apply `NEPacketTunnelNetworkSettings` only after the engine has initialized successfully.
3. Construct `PacketTunnelFlowAdapter(packetFlow:engine:mtu:)` and call `start` after network settings are accepted.
4. Call `stop` from `stopTunnel(with:completionHandler:)`, then complete the provider shutdown.
5. Configure only routes and address families that the Go engine can actually process. The current provider uses empty included-route lists as a safety boundary.

The Go implementation must treat packets as IP-layer data (no Ethernet header), preserve IPv4/IPv6 family information, process inbound packets synchronously, emit outbound packets asynchronously (TCP replies may arrive later), honor MTU limits, and avoid logging packet contents or destinations. A processing error drops that packet; it must never trigger direct-network fallback.

## Validation still required

The Swift sources, including the provider lifecycle scaffold, are type-checked in macOS CI against the runner's Apple SDK. The C ABI wrapper has not yet been linked against the generated c-shared library, exercised inside a signed Network Extension target, or installed on a device. The next integration step is an Xcode Packet Tunnel target with the Network Extension entitlement, generated C header/module map, and packaged Go dylib.