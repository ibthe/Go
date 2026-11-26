# Download and run the macOS development app

The **macOS App Build** workflow publishes an artifact named
`go-macos-download-package`.

## Install

1. Open the latest successful [macOS App Build workflow](https://github.com/ibthe/Go/actions/workflows/macos-app-build.yml).
2. Open the successful run and download `go-macos-download-package` under **Artifacts**.
3. Unzip `Go-macOS-unsigned.zip`.
4. Move `Go.app` into **Applications**.
5. If macOS blocks the app, open **System Settings → Privacy & Security** and use **Open Anyway** only if that option is offered for this app. You may also Control-click the app and choose **Open**.

The CI build is ad-hoc signed and its signature is checked before packaging. Ad-hoc signing is not an Apple Developer ID signature, does not establish a trusted developer identity, and does not guarantee Gatekeeper acceptance on every Mac. Do not disable Gatekeeper or run blanket quarantine-removal commands.

## VPN status — not ready for real traffic

This package is a development app shell, **not a functioning VPN**:
- the Packet Tunnel extension is not signed with an Apple Developer Team or provisioned;
- App Group and shared Keychain entitlements are not authorized for distribution;
- the provider intentionally installs no included traffic routes;
- the current engine is an experimental TCP-only prototype and does not protect all device traffic.

Do not rely on this build for privacy or security, and do not assume that Connect establishes a protected tunnel. A production build requires a registered App ID, Network Extension capability, matching App Group and Keychain entitlements, provisioning, Developer ID signing, notarization, and further TCP/UDP/DNS/QUIC/routing/fail-closed validation.

No Apple account password, verification code, certificate private key, or provisioning secret should be sent in chat.
