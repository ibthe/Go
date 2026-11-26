# macOS Packet Tunnel adapter

This directory contains the first native boundary for Apple's Network Extension packet flow.

## Current state

- `PacketTunnelFlowAdapter.swift` owns the asynchronous `NEPacketTunnelFlow` read/write loop.
- `GoPacketEngineCABI.swift` implements the Go packet-engine C ABI with Swift-owned buffers, a bounded output reader, and joined shutdown.
- The adapter passes complete IP packets and their address-family metadata to a `GoPacketEngine` implementation.
- `GoPacketTunnelProvider.swift` owns the experimental Network Extension start/stop lifecycle and connects the C ABI engine to the packet-flow adapter.
- The provider deliberately configures IPv4/IPv6 interface addresses with **empty included-route lists**. It is a lifecycle smoke-test scaffold and does not redirect normal device traffic.
- `ProxyCredentialStore.swift` provides a Keychain-backed helper for proxy usernames/passwords, but no app target or settings UI calls it yet.
- `scripts/build-macos-go-library.sh` builds the Go C-shared engine for both macOS architectures, combines them into a universal2 dylib, verifies the exported C ABI, and packages the result as `GoPacketEngine.xcframework`.
- The gVisor-backed processor remains in the isolated `experiments/gvisor` module.
- This source is still not a runnable VPN: there is no Xcode app/extension target, signed entitlement/profile, linked Go dylib, or device validation. The smoke-test provider intentionally installs no traffic routes.

## Build the native Go library

On a macOS host with Xcode Command Line Tools and Go 1.27.3 installed:

```sh
bash scripts/build-macos-go-library.sh
```

The output is written to `build/macos/`:

- `libgo_packet_engine.dylib` — universal2 arm64 + x86_64 dynamic library
- `libgo_packet_engine.h` — generated C ABI header
- `GoPacketEngine.xcframework/` — macOS XCFramework containing the universal2 dylib and C header, ready to add to an Xcode target

The `gVisor netstack spike` GitHub Actions workflow builds and uploads these outputs as the `go-packet-engine-macos-universal` artifact (14-day retention). The workflow verifies both architecture slices, the exported C ABI symbols, and XCFramework packaging. This still does not validate signing or runtime linkage in an app.

## Integration contract

The future `NEPacketTunnelProvider` must:

1. Construct a concrete `GoPacketEngine` backed by the Go userspace stack.
2. Apply `NEPacketTunnelNetworkSettings` only after the engine has initialized successfully.
3. Construct `PacketTunnelFlowAdapter(packetFlow:engine:mtu:)` and call `start` after network settings are accepted.
4. Call `stop` from `stopTunnel(with:completionHandler:)`, then complete the provider shutdown.
5. Configure only routes and address families that the Go engine can actually process. The current provider uses empty included-route lists as a safety boundary.

The Go implementation must treat packets as IP-layer data (no Ethernet header), preserve IPv4/IPv6 family information, process inbound packets synchronously, emit outbound packets asynchronously (TCP replies may arrive later), honor MTU limits, and avoid logging packet contents or destinations. A processing error drops that packet; it must never trigger direct-network fallback.

## Validation still required

The Swift sources, including the provider lifecycle scaffold, are type-checked in macOS CI against the runner's Apple SDK. The native Go dylib and XCFramework are built successfully for both macOS architectures in CI, but the Swift wrapper has not yet been linked against the XCFramework inside an Xcode target. There is no signed Network Extension app target or device validation yet. The next step is to add the Xcode project/extension target, embed and sign the universal dylib, and test lifecycle startup/shutdown on a Mac before considering any traffic routes.
