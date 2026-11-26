# Download and run the macOS development app

The **macOS App Build** workflow publishes a downloadable artifact named
`go-macos-download-package`.

## Install

1. Open the latest successful [macOS App Build workflow](https://github.com/ibthe/Go/actions/workflows/macos-app-build.yml).
2. Open the successful run and download `go-macos-download-package` under **Artifacts**.
3. Unzip `Go-macOS-unsigned.zip`.
4. Move `Go.app` into **Applications**.
5. Control-click the app, choose **Open**, and confirm the macOS prompt.

This is an unsigned development build. macOS may show an unidentified-developer warning.

## Important limitations

This package lets you launch and inspect the macOS app shell. It is **not a functioning VPN**:
- the Packet Tunnel extension is not signed or provisioned with an Apple Developer Team;
- App Group and shared Keychain entitlements are not runtime-authorized;
- the provider intentionally installs no included traffic routes;
- the current engine is an experimental TCP-only prototype and is not ready to protect all device traffic.

Do not use this build as a privacy or security tool, and do not assume that the Connect control establishes a protected tunnel. A properly signed and provisioned build plus further packet, DNS, UDP/QUIC, routing, and fail-closed validation is required before real VPN use.
