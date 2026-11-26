# macOS Packet Tunnel adapter

This directory contains the first native boundary for Apple's Network Extension packet flow.

## Current state

- `PacketTunnelFlowAdapter.swift` owns the asynchronous `NEPacketTunnelFlow` read/write loop.
- `GoPacketEngineCABI.swift` implements the Go packet-engine C ABI with Swift-owned buffers, a bounded output reader, and joined shutdown.
- The adapter passes complete IP packets and their address-family metadata to a `GoPacketEngine` implementation.
- `GoPacketTunnelProvider.swift` owns the experimental Network Extension start/stop lifecycle and connects the C ABI engine to the packet-flow adapter.
- The provider deliberately configures IPv4/IPv6 interface addresses with **empty included-route lists**. It is a lifecycle smoke-test scaffold and does not redirect normal device traffic.
- `GoSharedConfiguration.swift` copies the non-secret proxy JSON into the shared App Group container, applies owner-only directory/file permissions, and validates the file again in the extension.
- `ProxyCredentialStore.swift` stores username/password pairs in a dedicated shared Keychain access group, indexed by proxy ID. The host app and extension compile the same implementation.
- `GoPacketTunnelProvider.swift` loads the fixed configuration file from the App Group, reads each proxy's credentials from Keychain, and passes the resolved credential map to Go in memory. Neither secrets nor arbitrary filesystem paths are stored in `providerConfiguration`.
- `GoPacketEngineCABI.swift` calls `GoEngineCreateWithCredentials`; the C ABI accepts a bounded JSON credential map synchronously and does not retain Swift-owned pointers.
- `platform/macos/App/TunnelManager.swift` saves only the App Group identifier, fixed config filename, and non-secret tunnel settings in the system VPN profile. The dashboard imports a config file and can save credentials by proxy ID.
- `scripts/build-macos-go-library.sh` builds the Go C-shared engine for both macOS architectures, combines them into a universal2 dylib, verifies the exported C ABI, and packages the result as `GoPacketEngine.xcframework`.
- The gVisor-backed processor remains in the isolated `experiments/gvisor` module.
- `platform/macos/project.yml` defines a generated Xcode project with a host macOS app and a separate packet-tunnel extension target. The extension links and embeds the Go XCFramework.
- CI builds both targets without code signing. App Group and Keychain sharing are declared in both targets' entitlements, but they are not runtime-verified until a matching App Group and Keychain access group are registered in the Apple Developer account and the app/extension are signed with provisioning profiles. Unsigned CI cannot validate entitlement enforcement or Keychain sharing. The smoke-test provider intentionally installs no traffic routes.

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

The Swift sources and provider lifecycle scaffold compile as part of the actual Xcode extension target. CI has verified Go library generation, XCFramework packaging, Xcode project generation, linking, and unsigned app/extension build. Remaining work: register `group.app.go.macos` and the shared Keychain access group for the App ID, provision both targets with matching entitlements, sign the app/extension, and verify config/Keychain handoff on a real Mac. The current CI build proves compilation and linking only, not entitlement authorization or runtime access. No traffic routes should be enabled until the engine supports the required protocols and DNS behavior.
