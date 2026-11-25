# Routing rules

Rule bundles are local data, not executable scripts. See `rules.schema.json`.

Actions:
- `proxy`: route through a configured and healthy proxy outbound.
- `direct`: connect without a proxy.
- `block`: reject the connection.

Do not add third-party proxy endpoints or copied domain/IP databases without confirming authorization, provenance, update policy, and security implications. Never commit credentials.
