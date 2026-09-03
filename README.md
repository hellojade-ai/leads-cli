# hellojade (CLI)

The command-line client for the **hellojade Partner Intake API** — post a lead, prove your
key, read the live vocabulary, probe health, from a shell script or a terminal.

- One static binary, no runtime, no configuration file. Built on
  [leads-go](https://github.com/hellojade-ai/leads-go), which is standard-library only
- **Distinct exit codes per outcome**, so a script can branch on *why* something failed
  rather than parsing text
- `--json` on every command for machines, human output by default
- `--dry-run` prints the exact request — method, URL, headers, pretty body — and sends
  nothing
- The API key is read from the environment or a file and is **never printed**, not even by
  `--dry-run` (tested)

| | |
|---|---|
| API reference and live playground | <https://intake.hellojade.ai/api> |
| OpenAPI 3.0 contract | <https://intake.hellojade.ai/api/openapi.json> |
| Integration brief (the eight rules) | <https://intake.hellojade.ai/api/INTEGRATION.md> |
| Becoming a lead provider | <https://hellojade.ai/developers/provide-leads> |
| Other kits | [Go](https://github.com/hellojade-ai/leads-go) · [Node](https://github.com/hellojade-ai/leads-node) · [Python](https://github.com/hellojade-ai/leads-python) · [Ruby](https://github.com/hellojade-ai/leads-ruby) · [.NET](https://github.com/hellojade-ai/leads-dotnet) · [Rust](https://github.com/hellojade-ai/leads-rust) · [Browser JS](https://github.com/hellojade-ai/leads-js) |

## Install

**This tool is not published to any package registry** — no Homebrew tap, no apt repository,
no container image. It is built from this repository, or downloaded from a GitHub release.

### From a release

Attached to every [release](https://github.com/hellojade-ai/leads-cli/releases) are static
binaries for linux, darwin and windows on amd64 and arm64, plus a `SHA256SUMS` file.

```sh
curl -fsSLO https://github.com/hellojade-ai/leads-cli/releases/download/v0.1.0/hellojade_v0.1.0_linux_amd64
curl -fsSLO https://github.com/hellojade-ai/leads-cli/releases/download/v0.1.0/SHA256SUMS
sha256sum --check --ignore-missing SHA256SUMS
install -m 0755 hellojade_v0.1.0_linux_amd64 /usr/local/bin/hellojade
hellojade version
```

### From source

```sh
git clone --branch v0.1.0 https://github.com/hellojade-ai/leads-cli.git
cd leads-cli
go build -o hellojade ./cmd/hellojade      # Go 1.24+
# or, for every platform at once, into ./dist:
./build.sh
```

`go install github.com/hellojade-ai/leads-cli/cmd/hellojade@v0.1.0` also works, since the
module is public and resolvable through the Go module proxy.

| | |
|---|---|
| Module | `github.com/hellojade-ai/leads-cli` |
| Binary | `hellojade` |
| Version | `0.1.0` |
| Go floor | 1.24 |
| Dependencies | `github.com/hellojade-ai/leads-go` (itself dependency-free) |

## Quickstart

### 1. Prove the key first — it stores nothing

The API authenticates *before* it validates, so an empty body sent with a valid key comes
back `422` and nothing is stored, delivered or emailed. Do this before you script anything,
and again on launch day.

```sh
export HELLOJADE_API_KEY='...'      # never on the command line: it lands in your history and in ps
hellojade auth check
# ok: the key is valid and active at https://intake.hellojade.ai
```

```
exit 0   valid and active (the server answered 422 to the empty body)
exit 3   401 — missing, mistyped, revoked, or pointed at the wrong host
exit 5   429 — the per-IP budget, applied before authentication. Says nothing about your key
exit 7   transport failure. Check for http:// first: nothing listens on port 80
```

`hellojade auth check --json | jq .valid` for a machine.

### 2. Submit a lead

```sh
hellojade leads submit \
  --external-id 'acme-leads:A-99812' \
  --first-name Dana --last-name Whitfield --phone '(630) 555-0142' \
  --email dana.whitfield@example.com \
  --street-address '418 N Maple St' --city Naperville --state IL --zip 60540 \
  --project-area roof --project-service replacement --project-material 'asphalt shingle' \
  --project-details 'Hail damage on the south slope, insurance claim already filed.' \
  --cost 555.55 \
  --extra partner_job_id=XZ-1
# accepted evt_0198f2c1a4b00000a3d19f4c2b7e source=acme-leads flags=none
```

Only `--first-name`, `--last-name` and `--phone` are required. Send everything you have and
nothing you do not — an unset flag is omitted from the JSON, never sent as `null` or a
placeholder. `--extra k=v` is repeatable and a value that parses as JSON stays typed.

**Do not send `source`.** It is not a field: your key's registered label *is* the source, and
the server echoes it back. The CLI refuses `--extra source=...` before any request goes out.

The whole lead can come from JSON instead, with flags overriding individual fields:

```sh
hellojade leads submit --json-file lead.json --external-id 'acme-leads:A-99812'
cat lead.json | hellojade leads submit --json-file - --dry-run
```

`--dry-run` prints the request and exits 0 without sending — the fastest way to see exactly
what a set of flags produces.

```
POST https://intake.hellojade.ai/v1/intake
X-API-Key: <redacted, configured>
Content-Type: application/json
Idempotency-Key: acme-leads:A-99812

{
  "external_id": "acme-leads:A-99812",
  "first_name": "Dana",
  ...
}
```

### 3. Branch on the outcome

```sh
hellojade leads submit --external-id "acme-leads:$id" \
    --first-name "$first" --last-name "$last" --phone "$phone"
case $? in
  0)     mark_sent "$id" ;;              # 202 accepted or 200 duplicate — both are success
  2|3|4) alert_a_human "$id" ;;          # our bug: usage, a bad key, a body the server refuses
  5|6|7) requeue_for_later "$id" ;;      # theirs or the network: resend with the SAME key
esac
```

Runnable versions of all of this are in [`examples/`](examples/).

## Commands

| command | what it does | HTTP |
|---|---|---|
| `hellojade auth check` | proves the key without creating a lead | `POST /v1/intake` with `{}` |
| `hellojade leads submit` | posts one lead | `POST /v1/intake` |
| `hellojade vocabulary [--areas]` | the live `project_area` terms and their status | `GET /v1/vocabulary` (no key) |
| `hellojade health` | liveness of the intake edge; exits 6 when not ok | `GET /healthz` (no key) |
| `hellojade completion bash\|zsh\|fish` | a completion script generated from the binary's own command tree | — |
| `hellojade version` | CLI and client-library versions | — |
| `hellojade help [topic]` | help for any command, plus `help exit-codes` | — |

Every command takes `--json`. `hellojade help <topic>` is thorough — `help exit-codes` in
particular is the table a script author wants.

## Global flags

| flag | default | notes |
|---|---|---|
| `--base-url URL` | `$HELLOJADE_BASE_URL`, else `https://intake.hellojade.ai` | HTTPS only in production — **nothing listens on port 80** |
| `--api-key-file FILE` | — | read the key from a file instead of the environment |
| `--json` | off | machine-readable output on stdout |
| `--quiet` | off | suppress the advisory notes on stderr |
| `--request-id S` | unset | `X-Request-Id`, up to 64 characters. Set it to your own correlation id and the server adopts it — in the response header, in every error body, and in hellojade's access log. Left unset, the server mints one and returns it |
| `--user-agent S` | `leads-cli/<v> leads-go/<v>` | identify your integration |
| `--timeout D` | `20s` | per attempt |
| `--max-attempts N` | `5` | delivery attempts for 5xx and transport errors |
| `--retry-backoff D` | `1s` | first backoff; doubles each attempt |
| `--no-retry` | off | exactly one attempt |

Global flags may appear **before or after** the subcommand. Advisory notes go to stderr, so
`--json` output on stdout stays clean for a pipe.

### Environment

| | |
|---|---|
| `HELLOJADE_API_KEY` | your key. **Never pass a key as a command-line argument** — it lands in your shell history and is visible in `ps` to every user on the box |
| `HELLOJADE_BASE_URL` | override the base URL |

## Exit codes

They are part of the interface. A script may branch on them; they will not be renumbered
without a major version.

| code | name | when | retry? |
|---|---|---|---|
| `0` | ok | `202 accepted` **or** `200 duplicate` — a duplicate is what a retry is supposed to produce | done |
| `1` | error | an unclassified local failure: unreadable file, bad JSON on stdin | no |
| `2` | usage | a mistake in the command line. **Nothing was sent** | no |
| `3` | auth | `401` — the key is missing, wrong, or revoked. Configuration, never code | no |
| `4` | rejected | `400 invalid_json`, `413 body_too_large`, `422 validation_failed`. The server will keep rejecting this body | no |
| `5` | rate-limited | `429` still returned after the client exhausted its `Retry-After` waits | later |
| `6` | server | `5xx`, including `503 not_accepting`, after `--max-attempts`. Also `health` reporting `ok=false` | later, same key |
| `7` | network | transport failure or timeout: DNS, TLS, refused connection, deadline exceeded | later, same key |

On a `422` the human output lists **every** failing field, not just the first; `--json`
returns them as a `fields` object. On any error, the `request_id` is printed — quote it, or
the `event_id`, in a support conversation. **Never the key.**

## Retry and idempotency semantics

1. **Always send an `Idempotency-Key`, and make it your own stable id for the lead**,
   namespaced to you. `--external-id acme-leads:A-99812` supplies it by default;
   `--idempotency-key` overrides it. Not a timestamp, not a fresh UUID per attempt.
2. **Namespace it.** Dedupe is scoped to the **tenant**, not to your key. A bare `1234`
   can collide with another source's lead under the same customer, and yours is then
   silently never stored — you get a `200` pointing at *their* event. The CLI warns on a key
   with no namespace separator.
3. A repeat of an accepted key returns `200 duplicate` with the **original** `event_id`, and
   exits `0`. That is success.
4. **Retries are automatic** for transport errors and 5xx, with exponential backoff plus
   jitter, up to `--max-attempts`. The same `Idempotency-Key` goes out on every attempt, so a
   request that actually arrived cannot create a duplicate.
5. **`429` waits `max(Retry-After, backoff)` and does not consume a delivery attempt.**
   `Retry-After` is a floor, not a strategy.
6. **Any other 4xx is never retried.** A `422` means the body needs fixing; a `401` means the
   configuration does. A `422` does *not* consume the `Idempotency-Key` — send the same key
   again with a corrected body.
7. **Flags are not errors.** `phone_unnormalized`, `project_area_unknown`,
   `project_service_unknown`, `email_shape_suspect`, `extra_fields_preserved` and
   `country_unrecognized` arrive on a **successful** response. They are printed as
   `flags=...`; read them, do not retry on them.
8. **Do not hard-code the `project_area` vocabulary.** It grows by database insert on the
   server with no deploy. `hellojade vocabulary --areas > areas.txt` takes a build-time
   snapshot; an unrecognized term is stored verbatim and flagged, never rejected, so send
   your raw value rather than guessing at a mapping.

## Shell completion

```sh
source <(hellojade completion bash)                              # bash, this shell
hellojade completion bash > /etc/bash_completion.d/hellojade     # bash, persistent
hellojade completion zsh  > "${fpath[1]}/_hellojade" && compinit # zsh
hellojade completion fish > ~/.config/fish/completions/hellojade.fish
```

The script is generated from the command tree compiled into the binary, and a test asserts
that tree matches the help text — so completions cannot drift from the commands that exist.

## Development

```sh
export GOFLAGS=-mod=readonly
go vet ./...
go test -race -count=1 ./...
gofmt -l .            # must print nothing
./build.sh            # cross-compile linux/darwin/windows x amd64/arm64 into ./dist
```

The suite drives `cli.Main` directly — it takes its streams and its environment as
arguments, so every command is exercised end to end against an `httptest` stub with no
network and no global state. It covers every documented status code (200, 202, 400, 401,
413, 422, 429, 5xx), transport failure, the retry and backoff behavior, that a `429` does not
consume a delivery attempt, `--dry-run` sending nothing, the local refusals (`source`, an
out-of-range `--cost`, a missing idempotency key), flag/JSON merge precedence, and that **the
API key never reaches stdout or stderr**.

**No test makes a live call.** The only request you should ever make against production while
developing is `hellojade auth check` with your own key.

> **Do not test with a real-looking lead.** A real name and phone number reaching a real
> salesperson's phone is a cost to a real person. Use `--dry-run`, use `auth check`, or ask
> hellojade for a sandbox key.

## Releasing

Tags on GitHub only — this tool is **not** published to any package registry. Pushing a
`v*` tag runs the release workflow, which cross-compiles the six binaries, writes
`SHA256SUMS`, and attaches them to the release. See
[CONTRIBUTING.md](CONTRIBUTING.md#releasing).

## License

[MIT](LICENSE) © hellojade
