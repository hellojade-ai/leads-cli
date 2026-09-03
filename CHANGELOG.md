# Changelog

All notable changes to this tool are recorded here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the tool follows
[Semantic Versioning](https://semver.org/). The **exit codes are part of the
interface** and will not be renumbered without a major version.

## [0.1.0] — 2026-09-03

### Added

- `hellojade auth check` — the integration brief's key check. Posts `{}`, so a
  `422` proves the key and stores nothing.
- `hellojade leads submit` — one lead, from flags, from a JSON file, or from
  stdin (`--json-file -`), with flags overriding individual JSON fields.
  `--extra k=v` is repeatable and keeps JSON-parseable values typed.
- `--dry-run` prints the method, URL, headers and pretty-printed body and
  sends nothing. The API key is shown as `<redacted, configured>`, never
  printed.
- `hellojade vocabulary [--areas]` (`GET /v1/vocabulary`) and
  `hellojade health` (`GET /healthz`), both unauthenticated.
- `hellojade completion bash|zsh|fish`, generated from the command tree
  compiled into the binary; a test asserts the tree matches the help text.
- Distinct exit codes per outcome — `0` ok, `1` error, `2` usage, `3` auth,
  `4` rejected, `5` rate-limited, `6` server, `7` network — documented in
  `hellojade help exit-codes`.
- `--json` on every command, with advisory notes kept on stderr so piped
  stdout stays clean.
- Global flags accepted before or after the subcommand: `--base-url`,
  `--api-key-file`, `--user-agent`, `--request-id`, `--timeout`,
  `--max-attempts`, `--retry-backoff`, `--quiet`, `--no-retry`.
- Local refusals that would otherwise cost a round trip and a rate-limit
  token: `source` as an `--extra` key, a `--cost` outside 0.01–999.99, and a
  submit with no `Idempotency-Key`. A key with no namespace separator warns.
- `build.sh`, cross-compiling static binaries for linux, darwin and windows on
  amd64 and arm64 with a `SHA256SUMS` file, and a release workflow that
  attaches them to the tag.
- A suite driving `cli.Main` end to end against an `httptest` stub: every
  documented status code, transport failure, retry and backoff behavior, a
  `429` not consuming a delivery attempt, and an assertion that the API key
  never reaches stdout or stderr.

[0.1.0]: https://github.com/hellojade-ai/leads-cli/releases/tag/v0.1.0
