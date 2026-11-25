# Platform adapters

Native system tunnel adapters will live in separate subdirectories:

- `macos/`: Network Extension packet tunnel provider (Swift host app and extension).
- `windows/`: tunnel adapter and service.
- `android/`: Kotlin app using Android `VpnService`.

These adapters are intentionally placeholders. The current repository does not yet capture or forward system traffic.
