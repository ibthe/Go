# Proxy configuration

Go accepts an external JSON list of HTTPS CONNECT proxy endpoints. The example
configuration includes user-supplied, unverified candidate endpoints marked
`disabled: true`; these are retained for later testing but are excluded from
the active proxy pool.

## File format

Start from [configs/proxies.example.json](../configs/proxies.example.json).
Replace the example-only hostnames with endpoints you are authorized to use.
Each endpoint must be `https://host:port`; URL userinfo, paths, query strings,
and fragments are rejected. Set `disabled: true` to retain a candidate without
allowing the pool to use it. Omit `disabled` or set it to `false` only after
reviewing compatibility and authorization. Disabled entries are validated but
skipped by `Config.Resolve`.

Credentials are optional and are referenced by environment-variable names:

- `username_env`
- `password_env`

Set both variables or neither. The values are read at runtime and held in
memory only; do not put usernames or passwords in the JSON file, command line,
logs, or source control.

In the macOS app, import the JSON file into the Go App Group container. Save
credentials in Keychain using the matching proxy `id`; the app and packet
tunnel extension share a dedicated Keychain access group. The extension maps
the JSON environment-variable references to those Keychain values and passes
the resulting map synchronously to the Go C ABI. Secret values are not written
to the App Group file or the system VPN profile. This sharing requires a
registered App Group, matching Keychain access-group entitlement, and signed
provisioning profiles; unsigned CI cannot verify runtime access.

On macOS and Linux, restrict the file to its owner:

```sh
chmod 600 proxies.json
```

## Loading and dialing

The shared core exposes:

- `proxyconfig.Load(path)` to read and validate the file;
- `Config.Resolve(os.LookupEnv)` for development environment variables;
- `Config.ResolveWithLookup(func(string) (string, bool)` for platform secure-store bridges;
- `proxypool.New(resolved, timeout)` to build a dialer with rotating start
  order and sequential failover.

The pool never makes a direct connection when configured proxies fail. It
supports TCP through HTTPS CONNECT only; it does not provide UDP, DNS, QUIC,
or a complete device VPN. A successful CONNECT handshake is not proof of
whole-device tunnel functionality.

Example integration:

```go
cfg, err := proxyconfig.Load("proxies.json")
if err != nil {
    return err
}
resolved, err := cfg.Resolve(os.LookupEnv)
if err != nil {
    return err
}
pool, err := proxypool.New(resolved, 10*time.Second)
if err != nil {
    return err
}
conn, err := pool.DialContext(ctx, "tcp", "example.com:443")
```

## FastProxy data

When the researched endpoint list becomes available, add only endpoints whose
independent use is authorized and which pass compatibility checks. Import the
host and port values into this file; keep credentials in the environment for
development and in the platform's native secure credential store for packaged
macOS, Windows, and Android applications. Do not import the PAC file as
executable configuration or publish browsing-derived host lists.

## Unverified candidate endpoints

The following addresses were supplied for evaluation and are included in
`configs/proxies.example.json` as disabled candidates:

- Primary: `23.237.103.44:8443`, `ff.s1.redirtry.top:8443`
- Backup: `162.244.81.74:8443`, `162.244.81.207:8443`
- YouTube: `23.237.103.46:8443`, `ff.y1.redirtry.top:8443`
- Google Play: `de26.staticvaultcdn.xyz:443`

The 2026-11-26 compatibility probe passed 0 of 7: four endpoints returned
HTTP 400, two returned HTTP 403, and one timed out. These results do not prove
the services are permanently unusable, but none is currently compatible with
our unauthenticated HTTPS CONNECT transport. Keep them disabled until valid
operator instructions/credentials are available and a new test succeeds.
See [the full audit](PROXY_ENDPOINT_AUDIT.md).
