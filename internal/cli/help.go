package cli

import "fmt"

func (a *app) printHelp() {
	fmt.Fprint(a.stdout, `hellojade — command-line client for the hellojade Partner Intake API

USAGE
  hellojade [global flags] <command> [args]

COMMANDS
  auth check       prove your API key works without creating a lead
  leads submit     post one lead
  vocabulary       the accepted project_area terms, live from the server
  health           liveness of the intake edge
  completion       print a shell completion script (bash, zsh, fish)
  version          print the version
  help [topic]     help for a command, or 'help exit-codes'

START HERE
  export HELLOJADE_API_KEY=...
  hellojade auth check          # 422 under the hood: proves the key, stores nothing

GLOBAL FLAGS
  --base-url URL        default $HELLOJADE_BASE_URL, else https://intake.hellojade.ai
  --api-key-file FILE   read the key from a file instead of the environment
  --json                machine-readable output on stdout
  --quiet               suppress the advisory notes on stderr
  --request-id S        X-Request-Id; the server echoes it, so support can trace it
  --user-agent S        identify your integration
  --timeout D           per attempt (default 20s)
  --max-attempts N      delivery attempts for 5xx and transport errors (default 5)
  --retry-backoff D     first backoff; doubles each attempt (default 1s)
  --no-retry            exactly one attempt
  --version, -h/--help

  Global flags may appear before or after the command.

ENVIRONMENT
  HELLOJADE_API_KEY   your key. Never pass a key as a command-line argument:
                      it lands in your shell history and is visible in ps.
  HELLOJADE_BASE_URL  override the base URL (must be https:// in production)

EXIT CODES
  0 ok  1 error  2 usage  3 auth(401)  4 rejected(4xx)  5 rate-limited(429)
  6 server(5xx)  7 network
  Full table: hellojade help exit-codes

DOCUMENTATION
  https://intake.hellojade.ai/api                  docs + live playground
  https://intake.hellojade.ai/api/openapi.json     machine-readable contract
  https://intake.hellojade.ai/api/INTEGRATION.md   the integration brief
  https://hellojade.ai/developers/provide-leads    becoming a lead provider
`)
}

func (a *app) printAuthHelp() {
	fmt.Fprint(a.stdout, `hellojade auth check — prove your key, create nothing

USAGE
  hellojade auth check [--base-url URL] [--json]

Run this before you write any integration code, and again on the day you go
live. It is the one check that separates "my credentials are wrong" from "my
payload is wrong", and it costs a customer nothing.

The endpoint authenticates BEFORE it validates, so this sends a real request
with your key and a deliberately empty body. Authentication succeeds, the
validator rejects the empty body with 422, and nothing is stored, delivered,
emailed or written to any CRM. A 422 does not consume an Idempotency-Key.

  exit 0   the key is valid and active
  exit 3   401: missing, mistyped, revoked, or you are on the wrong host
  exit 5   429: you are over the per-IP budget. That limit is applied BEFORE
           authentication, so it says nothing about your key. Wait and repeat
  exit 7   transport failure. Check for http:// first — nothing listens on
           port 80, so it fails with a connection error, not a redirect

EXAMPLES
  hellojade auth check
  hellojade auth check --json | jq .valid
  hellojade --api-key-file /run/secrets/hj auth check
`)
}

func (a *app) printVocabularyHelp() {
	fmt.Fprint(a.stdout, `hellojade vocabulary — the accepted project_area terms

USAGE
  hellojade vocabulary [--areas] [--json]

GET /v1/vocabulary. No API key required. The server reads this from the same
database table its validator reads, so it cannot go stale the way a constant
in a document can.

  --areas   print only the project_area terms, one per line, for scripting

Do NOT hard-code this list: it grows by database insert, with no deploy. An
unrecognized project_area or project_service is not an error — the value is
stored verbatim, flagged, and counted so the vocabulary grows from real
traffic. Send your raw term rather than guessing at a mapping.

EXAMPLES
  hellojade vocabulary
  hellojade vocabulary --areas > areas.txt      # a build-time snapshot
  hellojade vocabulary --json | jq -r '.required[]'
`)
}

func (a *app) printHealthHelp() {
	fmt.Fprint(a.stdout, `hellojade health — liveness of the intake edge

USAGE
  hellojade health [--json]

GET /healthz. No API key required. Exits 6 when the server reports itself not
ok, so it works as a monitoring probe.

  ok=false / store_writable=false is the 503 not_accepting condition: the
  server's store is unwritable. That is theirs, not yours. Retry later with
  the same Idempotency-Key; nothing you send is lost by waiting.

EXAMPLES
  hellojade health
  hellojade health --json | jq .store_writable
`)
}

func (a *app) printCompletionHelp() {
	fmt.Fprint(a.stdout, `hellojade completion — print a shell completion script

USAGE
  hellojade completion bash|zsh|fish

INSTALL
  bash   source <(hellojade completion bash)
         # persist: hellojade completion bash > /etc/bash_completion.d/hellojade
  zsh    hellojade completion zsh > "${fpath[1]}/_hellojade"; compinit
  fish   hellojade completion fish > ~/.config/fish/completions/hellojade.fish

The script is generated from the command tree in the binary, so it cannot
drift from the commands the binary actually has.
`)
}

func (a *app) printExitCodesHelp() {
	fmt.Fprint(a.stdout, `hellojade — exit codes

They are part of the interface. A script may branch on them; they will not be
renumbered without a major version.

  0  ok            success. A 202 (accepted) AND a 200 (duplicate) both land
                   here — a duplicate is what you want from a retry, and it
                   carries the original event_id
  1  error         an unclassified local failure: unreadable file, bad JSON
                   on stdin, something the contract does not describe
  2  usage         a mistake in the command line. Nothing was sent
  3  auth          401. The key is missing, wrong, or revoked. Configuration,
                   never code
  4  rejected      400, 413 or 422. The server will keep rejecting this body;
                   fix it. Retrying unchanged only burns rate limit
  5  rate-limited  429 still returned after the client exhausted its
                   Retry-After waits
  6  server        5xx, including 503 not_accepting, still returned after the
                   client exhausted its retries. Theirs, not yours — resend
                   later with the same Idempotency-Key. Also 'health' when
                   the server reports ok=false
  7  network       transport failure or timeout: DNS, TLS, refused
                   connection, deadline exceeded

RETRY POLICY BEHIND THESE
  Codes 6 and 7 are only reached after --max-attempts attempts with growing
  backoff. Code 5 is only reached after the client has already waited out
  Retry-After repeatedly; a 429 does not consume a delivery attempt. Codes 3
  and 4 are returned on the first response — they are never retried.

SCRIPTING
  hellojade leads submit --external-id acme:1 --first-name A --last-name B \
      --phone 6305550142
  case $? in
    0) ;;                       # sent (or already had it)
    3|4) exit 1 ;;              # our bug: alert a human, do not requeue
    5|6|7) requeue_for_later ;; # theirs or the network: same key, later
  esac
`)
}
