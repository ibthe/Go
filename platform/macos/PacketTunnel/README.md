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
- `platform/macos/project.yml` defines a generated Xcode project with a host macOS app and a separate packet-tunnel extension target. The extension links and embeds the Go XCFramework.
- CI successfully generates the Xcode project and builds both targets without code signing. This is a buildable development shell, not yet a distributable or operational VPN: there is no signed entitlement/profile, tunnel-management UI, secure app-to-extension configuration handoff, or device validation. The smoke-test provider intentionally installs no traffic routes.

## Build the native Go library

On a macOS host with Xcode Command Line Tools and Go 1.27.3 installed:

```sh
bash scripts/build-macos-go-library.sh
```

The output is written to `build/macos/`:

- `libgo_packet_engine.dylib` — universal2 arm64 + x86_64 dynamic library
- `libgo_packet_engine.h` — generated C ABI header
- `GoPacketEngine.xcframework/` — macOS XCFramework containing the universal2 dylib and C header, ready to add to an Xcode target

The `gVisor netstack spike` GitHub Actions workflow builds and uploads these outputs as the `go-packet-engine-macos-universal` artifact (14-day retention). The separate `macOS app build` workflow consumes the XCFramework, generates `Go.xcodeproj` from `project.yml`, and successfully builds the app and extension with code signing disabled.

## Integration contract

The future `NEPacketTunnelProvider` must:

1. Construct a concrete `GoPacketEngine` backed by the Go userspace stack.
2. Apply `NEPacketTunnelNetworkSettings` only after the engine has initialized successfully.
3. Construct `PacketTunnelFlowAdapter(packetFlow:engine:mtu:)` and call `start` after network settings are accepted.
4. Call `stop` from `stopTunnel(with:completionHandler:)`, then complete the provider shutdown.
5. Configure only routes and address families that the Go engine can actually process. The current provider uses empty included-route lists as a safety boundary.

The Go implementation must treat packets as IP-layer data (no Ethernet header), preserve IPv4/IPv6 family information, process inbound packets synchronously, emit outbound packets asynchronously (TCP replies may arrive later), honor MTU limits, and avoid logging packet contents or destinations. A processing error drops that packet; it must never trigger direct-network fallback.

## Validation still required

The Swift sources and provider lifecycle scaffold compile as part of the actual Xcode extension target. CI has verified Go library generation, XCFramework packaging, Xcode project generation, linking, and unsigned app/extension build. Remaining work: add a real tunnel-management UI and secure configuration handoff, obtain the required Apple signing capability and provisioning profile, sign the app/extension, then validate lifecycle startup/shutdown on a Mac. No traffic routes should be enabled until the engine supports the required protocols and DNS behavior.
