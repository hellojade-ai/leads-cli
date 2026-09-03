# Contributing

Thanks for looking at this. The tool is deliberately small: one binary, one
dependency ([leads-go](https://github.com/hellojade-ai/leads-go), which is
itself standard-library only). Keep it that way.

## Ground rules

- **No new dependencies.** A pull request that adds a `require` line other
  than the client module will be closed. `flag` and `encoding/json` do
  everything this needs; a CLI framework would be more code than the CLI.
- **Go 1.24 is the floor.** CI runs on it across Linux, macOS and Windows.
- **The exit codes are the interface.** A script branches on them. Adding one
  is fine; changing what an existing one means is a major version.
- **`cli.Main` takes its streams and its environment as arguments** and never
  calls `os.Exit`. That is what makes the whole command testable without a
  network or a process. Do not reach for `os.Stdout` or `os.Getenv` inside a
  command.
- **The API key never reaches stdout or stderr.** `TestAPIKeyNeverPrinted`
  checks every stream of every command for the test key; keep it green.
- **No live calls in tests.** Everything runs against `httptest`. The only
  request you should make against production while developing is
  `hellojade auth check` with your own key.
- **The completion tables must match the help text.** They are separate
  literals in different files, so `TestCompletionTablesMatchHelp` asserts they
  agree. Add a flag to one, add it to the other.
- American English in code, comments and docs.

## Building and testing

```sh
export GOFLAGS=-mod=readonly
go vet ./...
go test -race -count=1 ./...
gofmt -l .            # must print nothing
./build.sh            # every target, into ./dist, with SHA256SUMS
./build.sh linux/amd64   # or just one
```

On this project's own machine the toolchain is `jo` (Jade Go) rather than
stock `go`; `build.sh` picks it up automatically when it is on `PATH`.

## Changing behavior against the API

The API is the contract, not this repository. Before changing how a status
code is handled, read the row for it in
[the integration brief](https://intake.hellojade.ai/api/INTEGRATION.md) and
the [OpenAPI document](https://intake.hellojade.ai/api/openapi.json). If the
server is answering in a way neither describes, contact the person at
hellojade who issued your key and include the `request_id` — never the key.

## Pull requests

1. Add or update a test that fails without your change.
2. Add a line under `[Unreleased]` in `CHANGELOG.md`.
3. Update `hellojade help <topic>` if you changed a flag or a command, and the
   completion tables with it.
4. Make sure the commands above pass.

## Releasing

Releases are **GitHub tags only**. This tool is not published to Homebrew, apt,
or any other registry, and no workflow pushes to one.

1. Move the `[Unreleased]` entries in `CHANGELOG.md` under the new version
   with its date, and update the link reference at the bottom of the file.
2. Bump `Version` in `internal/cli/cli.go`.
3. Commit, then `git tag -a vX.Y.Z -m "vX.Y.Z"` and push the tag.
4. `.github/workflows/release.yml` runs the tests, cross-compiles the six
   binaries, verifies `SHA256SUMS`, and attaches everything to the release.

The tag is the release. Never move one after it is pushed — a partner may
have pinned it.

## Security

Do not open a public issue for a security problem, and never paste an API key
anywhere. See [SECURITY.md](SECURITY.md).
