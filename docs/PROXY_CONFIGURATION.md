# Proxy configuration

Go accepts an external JSON list of HTTPS CONNECT proxy endpoints. No FastProxy
addresses, credentials, PAC data, or third-party server list is bundled.

## File format

Start from [configs/proxies.example.json](../configs/proxies.example.json).
Replace the example-only hostnames with endpoints you are authorized to use.
Each endpoint must be `https://host:port`; URL userinfo, paths, query strings,
and fragments are rejected.

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
