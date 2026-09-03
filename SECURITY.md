# Security

## Reporting a vulnerability

Email **security@hellojade.ai**. Include the CLI version (`hellojade version`),
a description, and steps to reproduce. You will get an acknowledgment within
two business days. Please do not open a public issue for a security report.

## Your API key

- The key authenticates you and attributes every lead you send. Treat it like
  a password: environment variable or secret store, never source, never a URL,
  never a log line.
- **Never pass a key as a command-line argument.** There is deliberately no
  `--api-key` flag. An argument lands in your shell history and is visible in
  `ps` to every user on the box. Use `HELLOJADE_API_KEY`, or
  `--api-key-file /run/secrets/hellojade` for a systemd credential or a mounted
  secret.
- This tool never writes the key anywhere: it is sent only in the `X-API-Key`
  header, `--dry-run` renders it as `<redacted, configured>`, and it is
  excluded from every error message. `TestAPIKeyNeverPrinted` asserts that
  across every command and both output streams.
- If a key has been exposed, tell hellojade immediately and ask for a
  rotation. A replacement is issued so you can cut over before the old key is
  revoked. hellojade stores only a hash, so a lost key is rotated, never
  recovered.

## Reporting a problem to hellojade support

Quote the `request_id` printed on any failure, or the `event_id` from a
success. **Never the key**, and never the full response body if it contains
lead data belonging to someone else.

## Verifying a download

Every release carries a `SHA256SUMS` file covering all six binaries:

```sh
sha256sum --check --ignore-missing SHA256SUMS
```

The binaries are built from a tagged commit by
[`.github/workflows/release.yml`](.github/workflows/release.yml), which
re-verifies the checksums before attaching them.

## Supported versions

Only the latest minor release receives fixes.
