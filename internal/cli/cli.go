// Package cli implements the hellojade command: a thin, scriptable front end
// over the hellojade Partner Intake API.
//
// Everything here is written so the command can be tested end to end without
// a network: Main takes its streams and its environment as arguments, and
// every command routes through the same client constructor, which honors
// --base-url. The test suite points it at an httptest stub.
package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"

	hellojade "github.com/hellojade-ai/leads-go"
)

// Version is the CLI's own version, independent of the client module's.
const Version = "0.1.0"

// Exit codes. They are part of the interface: a script may branch on them,
// so they are documented in `hellojade help exit-codes` and must not be
// renumbered without a major version.
const (
	// ExitOK is success. A 202 (accepted) and a 200 (duplicate) are both
	// success — see the idempotency notes in `hellojade help leads`.
	ExitOK = 0
	// ExitError is an unclassified local failure: unreadable file, bad
	// JSON on stdin, an error the contract does not describe.
	ExitError = 1
	// ExitUsage is a mistake in the command line, caught before any
	// request is made.
	ExitUsage = 2
	// ExitAuth is 401: the key is missing, wrong, or revoked. A
	// configuration problem, never a code problem.
	ExitAuth = 3
	// ExitRejected is a 4xx the server will keep rejecting: 400, 413, 422.
	// Retrying it unchanged only burns rate limit.
	ExitRejected = 4
	// ExitRateLimited is 429 still returned after the client exhausted its
	// Retry-After waits.
	ExitRateLimited = 5
	// ExitServer is a 5xx (including 503 not_accepting) still returned
	// after the client exhausted its retries. Theirs, not yours.
	ExitServer = 6
	// ExitNetwork is a transport failure or a timeout: DNS, TLS, refused
	// connection, deadline exceeded. Check the URL scheme first — the
	// intake host has no listener on port 80.
	ExitNetwork = 7
)

// app carries the parsed global options and the streams every command
// writes to.
type app struct {
	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer
	getenv func(string) string

	baseURL      string
	apiKeyFile   string
	userAgent    string
	requestID    string
	timeout      time.Duration
	maxAttempts  int
	retryBackoff time.Duration
	jsonOut      bool
	quiet        bool
	noRetry      bool
}

// globalFlags maps every global flag to whether it consumes a following
// argument. Globals may appear before or after the subcommand, so they are
// lifted out of the argument list before the command is dispatched.
var globalFlags = map[string]bool{
	"--base-url": true, "--api-key-file": true, "--user-agent": true,
	"--request-id": true, "--timeout": true, "--max-attempts": true,
	"--retry-backoff": true,
	"--json":          false, "--quiet": false, "--no-retry": false,
	"--help": false, "-h": false, "--version": false, "-v": false,
}

// Main runs the command and returns the process exit code. It never calls
// os.Exit, so tests can drive it directly.
func Main(args []string, stdin io.Reader, stdout, stderr io.Writer, getenv func(string) string) int {
	if getenv == nil {
		getenv = os.Getenv
	}
	a := &app{
		stdin: stdin, stdout: stdout, stderr: stderr, getenv: getenv,
		timeout:      hellojade.DefaultTimeout,
		maxAttempts:  hellojade.DefaultRetryPolicy.MaxAttempts,
		retryBackoff: hellojade.DefaultRetryPolicy.BaseBackoff,
	}

	globals, rest, err := splitGlobals(args)
	if err != nil {
		return a.usage(err.Error())
	}

	fs := a.flagSet("hellojade")
	var wantHelp, wantVersion bool
	fs.StringVar(&a.baseURL, "base-url", "", "intake base URL (default $HELLOJADE_BASE_URL, else "+hellojade.DefaultBaseURL+")")
	fs.StringVar(&a.apiKeyFile, "api-key-file", "", "read the API key from FILE instead of $HELLOJADE_API_KEY")
	fs.StringVar(&a.userAgent, "user-agent", "", "identify your integration in the User-Agent header")
	fs.StringVar(&a.requestID, "request-id", "", "X-Request-Id echoed by the server; makes support answerable")
	fs.DurationVar(&a.timeout, "timeout", a.timeout, "per-attempt timeout")
	fs.IntVar(&a.maxAttempts, "max-attempts", a.maxAttempts, "delivery attempts for 5xx and transport errors")
	fs.DurationVar(&a.retryBackoff, "retry-backoff", a.retryBackoff, "first retry backoff; doubles each attempt")
	fs.BoolVar(&a.jsonOut, "json", false, "machine-readable output on stdout")
	fs.BoolVar(&a.quiet, "quiet", false, "suppress advisory notes on stderr")
	fs.BoolVar(&a.noRetry, "no-retry", false, "exactly one attempt; never retry")
	fs.BoolVar(&wantHelp, "help", false, "print help and exit")
	fs.BoolVar(&wantHelp, "h", false, "print help and exit")
	fs.BoolVar(&wantVersion, "version", false, "print the version and exit")
	fs.BoolVar(&wantVersion, "v", false, "print the version and exit")
	if err := fs.Parse(globals); err != nil {
		return a.usage(err.Error())
	}
	if a.timeout <= 0 {
		return a.usage("--timeout must be positive")
	}
	if a.maxAttempts < 1 {
		return a.usage("--max-attempts must be at least 1")
	}

	if wantVersion && len(rest) == 0 {
		return a.version()
	}
	if wantHelp || len(rest) == 0 {
		a.printHelp()
		if len(rest) == 0 && !wantHelp {
			return ExitUsage
		}
		return ExitOK
	}

	switch cmd := rest[0]; cmd {
	case "auth":
		return a.auth(rest[1:])
	case "leads":
		return a.leads(rest[1:])
	case "vocabulary", "vocab":
		return a.vocabulary(rest[1:])
	case "health":
		return a.health(rest[1:])
	case "completion":
		return a.completion(rest[1:])
	case "version":
		return a.version()
	case "help":
		return a.help(rest[1:])
	default:
		return a.usage(fmt.Sprintf("unknown command %q — run 'hellojade help'", cmd))
	}
}

// splitGlobals lifts global flags out of args wherever they appear, so
// `hellojade --json leads submit …` and `hellojade leads submit … --json`
// both work. Everything after a bare `--` is positional.
func splitGlobals(args []string) (globals, rest []string, err error) {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			rest = append(rest, args[i+1:]...)
			return globals, rest, nil
		}
		name, value, hasValue := strings.Cut(arg, "=")
		takesValue, isGlobal := globalFlags[name]
		if !isGlobal {
			rest = append(rest, arg)
			continue
		}
		switch {
		case hasValue:
			globals = append(globals, name+"="+value)
		case takesValue:
			if i+1 >= len(args) {
				return nil, nil, fmt.Errorf("%s needs a value", name)
			}
			globals = append(globals, name, args[i+1])
			i++
		default:
			globals = append(globals, name)
		}
	}
	return globals, rest, nil
}

// flagSet returns a flag set that reports its own errors through usage()
// rather than printing the Go default banner.
func (a *app) flagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.Usage = func() {}
	return fs
}

// parse runs fs over args, turning -h into the command's own help. The
// second return value is true when the caller should return the code.
func (a *app) parse(fs *flag.FlagSet, args []string, help func()) (int, bool) {
	for _, arg := range args {
		if arg == "--" {
			break
		}
		if arg == "-h" || arg == "--help" || arg == "help" {
			help()
			return ExitOK, true
		}
	}
	if err := fs.Parse(args); err != nil {
		return a.usage(fmt.Sprintf("%s: %v", fs.Name(), err)), true
	}
	return 0, false
}

// -------------------------------------------------------------------------
// commands

func (a *app) auth(args []string) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		a.printAuthHelp()
		if len(args) == 0 {
			return ExitUsage
		}
		return ExitOK
	}
	if args[0] != "check" {
		return a.usage(fmt.Sprintf("unknown auth command %q (only 'check')", args[0]))
	}
	fs := a.flagSet("auth check")
	if code, done := a.parse(fs, args[1:], a.printAuthHelp); done {
		return code
	}
	if fs.NArg() > 0 {
		return a.usage(fmt.Sprintf("auth check: unexpected argument %q", fs.Arg(0)))
	}

	c, err := a.client(true)
	if err != nil {
		return a.usage(err.Error())
	}
	ctx, cancel := a.ctx()
	defer cancel()

	// The key check IS an empty POST: the server authenticates before it
	// validates, so a 422 proves the key and stores nothing. CheckKey
	// returns nil for exactly that outcome.
	err = c.CheckKey(ctx)
	if err != nil {
		if a.jsonOut {
			a.writeJSON(map[string]any{
				"ok": false, "valid": false,
				"base_url": a.resolvedBaseURL(),
				"error":    err.Error(),
			})
			return exitCodeFor(err)
		}
		return a.fail(err)
	}
	if a.jsonOut {
		a.writeJSON(map[string]any{
			"ok": true, "valid": true, "base_url": a.resolvedBaseURL(),
			"detail": "the key authenticated and the empty body was rejected by the validator; nothing was stored",
		})
		return ExitOK
	}
	fmt.Fprintf(a.stdout, "ok: the key is valid and active at %s\n", a.resolvedBaseURL())
	a.info("nothing was stored, delivered, emailed or written to any CRM.")
	return ExitOK
}

func (a *app) vocabulary(args []string) int {
	fs := a.flagSet("vocabulary")
	var areasOnly bool
	fs.BoolVar(&areasOnly, "areas", false, "print only the project_area terms, one per line")
	if code, done := a.parse(fs, args, a.printVocabularyHelp); done {
		return code
	}
	if fs.NArg() > 0 {
		return a.usage(fmt.Sprintf("vocabulary: unexpected argument %q", fs.Arg(0)))
	}

	// No key needed for this endpoint.
	c, err := a.client(false)
	if err != nil {
		return a.usage(err.Error())
	}
	ctx, cancel := a.ctx()
	defer cancel()
	v, err := c.Vocabulary(ctx)
	if err != nil {
		return a.fail(err)
	}
	if areasOnly {
		for _, area := range v.Areas() {
			fmt.Fprintln(a.stdout, area)
		}
		return ExitOK
	}
	if a.jsonOut {
		a.writeJSON(v)
		return ExitOK
	}
	fmt.Fprintf(a.stdout, "required: %s\n", strings.Join(v.Required, ", "))
	services := make([]string, len(v.ProjectService))
	for i, s := range v.ProjectService {
		services[i] = string(s)
	}
	fmt.Fprintf(a.stdout, "project_service: %s\n", strings.Join(services, ", "))
	fmt.Fprintf(a.stdout, "project_area (%d):\n", len(v.ProjectArea))
	areas := append([]hellojade.VocabularyArea(nil), v.ProjectArea...)
	sort.Slice(areas, func(i, j int) bool { return areas[i].Area < areas[j].Area })
	for _, area := range areas {
		fmt.Fprintf(a.stdout, "  %-28s %s\n", area.Area, area.Status)
	}
	a.info("this list grows by database insert on the server. Fetch it; do not hard-code it. An unrecognized term is stored verbatim and flagged, never rejected — send your raw value.")
	return ExitOK
}

func (a *app) health(args []string) int {
	fs := a.flagSet("health")
	if code, done := a.parse(fs, args, a.printHealthHelp); done {
		return code
	}
	if fs.NArg() > 0 {
		return a.usage(fmt.Sprintf("health: unexpected argument %q", fs.Arg(0)))
	}
	c, err := a.client(false)
	if err != nil {
		return a.usage(err.Error())
	}
	ctx, cancel := a.ctx()
	defer cancel()
	h, err := c.Health(ctx)
	if err != nil {
		return a.fail(err)
	}
	if a.jsonOut {
		a.writeJSON(h)
	} else {
		state := "ok"
		if !h.OK {
			state = "DEGRADED"
		}
		fmt.Fprintf(a.stdout, "%s store_writable=%t pending=%d dead=%d",
			state, h.StoreWritable, h.Pending, h.Dead)
		if h.OldestPendingAgeSeconds != nil {
			fmt.Fprintf(a.stdout, " oldest_pending_age_s=%d", *h.OldestPendingAgeSeconds)
		}
		fmt.Fprintln(a.stdout)
	}
	if !h.OK {
		return ExitServer
	}
	return ExitOK
}

func (a *app) version() int {
	if a.jsonOut {
		a.writeJSON(map[string]string{
			"hellojade_cli": Version,
			"hellojade_go":  hellojade.Version,
		})
		return ExitOK
	}
	fmt.Fprintf(a.stdout, "hellojade %s (leads-go %s)\n", Version, hellojade.Version)
	return ExitOK
}

func (a *app) help(args []string) int {
	if len(args) == 0 {
		a.printHelp()
		return ExitOK
	}
	switch args[0] {
	case "auth":
		a.printAuthHelp()
	case "leads":
		a.printLeadsHelp()
	case "vocabulary", "vocab":
		a.printVocabularyHelp()
	case "health":
		a.printHealthHelp()
	case "completion":
		a.printCompletionHelp()
	case "exit-codes":
		a.printExitCodesHelp()
	default:
		return a.usage(fmt.Sprintf("no help topic %q", args[0]))
	}
	return ExitOK
}

// -------------------------------------------------------------------------
// client construction

// resolvedBaseURL reports the base URL that will actually be used, in
// precedence order: --base-url, $HELLOJADE_BASE_URL, the library default.
func (a *app) resolvedBaseURL() string {
	if a.baseURL != "" {
		return strings.TrimRight(a.baseURL, "/")
	}
	if v := strings.TrimSpace(a.getenv(hellojade.EnvBaseURL)); v != "" {
		return strings.TrimRight(v, "/")
	}
	return hellojade.DefaultBaseURL
}

// apiKey resolves the key from --api-key-file, else $HELLOJADE_API_KEY. It
// returns "" with no error when neither is set; the caller decides whether
// the command needs one.
func (a *app) apiKey() (string, error) {
	if a.apiKeyFile != "" {
		b, err := os.ReadFile(a.apiKeyFile)
		if err != nil {
			return "", fmt.Errorf("--api-key-file: %w", err)
		}
		key := strings.TrimSpace(string(b))
		if key == "" {
			return "", fmt.Errorf("--api-key-file: %s is empty", a.apiKeyFile)
		}
		return key, nil
	}
	return strings.TrimSpace(a.getenv(hellojade.EnvAPIKey)), nil
}

// client builds the API client. needKey makes a missing key a usage error
// here rather than a confusing 401 later.
func (a *app) client(needKey bool) (*hellojade.Client, error) {
	key, err := a.apiKey()
	if err != nil {
		return nil, err
	}
	if needKey && key == "" {
		return nil, fmt.Errorf("no API key: set $%s, or pass --api-key-file FILE. Never put a key in a command line — it lands in your shell history and in ps", hellojade.EnvAPIKey)
	}

	policy := hellojade.DefaultRetryPolicy
	policy.MaxAttempts = a.maxAttempts
	policy.BaseBackoff = a.retryBackoff
	// Keep the jitter proportional to the backoff. At the default 1s this is
	// exactly the library default of 500ms; a caller that sets a millisecond
	// backoff does not then wait half a second of jitter on every retry.
	policy.Jitter = a.retryBackoff / 2
	if a.noRetry {
		policy = hellojade.NoRetries
	}

	ua := a.userAgent
	if ua == "" {
		ua = "leads-cli/" + Version + " leads-go/" + hellojade.Version
	}

	opts := []hellojade.Option{
		hellojade.WithBaseURL(a.resolvedBaseURL()),
		hellojade.WithTimeout(a.timeout),
		hellojade.WithUserAgent(ua),
		hellojade.WithRetryPolicy(policy),
	}
	if key != "" {
		opts = append(opts, hellojade.WithAPIKey(key))
	}
	return hellojade.New(opts...)
}

// ctx bounds the whole command, retries included, at a generous multiple of
// the per-attempt timeout so a retry sequence is not cut off halfway.
func (a *app) ctx() (context.Context, context.CancelFunc) {
	budget := a.timeout * time.Duration(a.maxAttempts+1)
	if budget < a.timeout {
		budget = a.timeout
	}
	return context.WithTimeout(context.Background(), budget)
}

// -------------------------------------------------------------------------
// output

func (a *app) writeJSON(v any) {
	enc := json.NewEncoder(a.stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		fmt.Fprintf(a.stderr, "hellojade: writing output: %v\n", err)
	}
}

// info writes an advisory note to stderr, so it never pollutes piped
// stdout. --quiet silences these; they are never load-bearing.
func (a *app) info(format string, args ...any) {
	if a.quiet {
		return
	}
	fmt.Fprintf(a.stderr, "hellojade: "+format+"\n", args...)
}

// usage reports a command-line mistake. Nothing has been sent at this point.
func (a *app) usage(msg string) int {
	if a.jsonOut {
		a.writeJSON(map[string]any{"ok": false, "error": "usage", "message": msg})
	} else {
		fmt.Fprintf(a.stderr, "hellojade: %s\n", msg)
		fmt.Fprintln(a.stderr, "run 'hellojade help' for usage")
	}
	return ExitUsage
}

// fail reports a request failure and picks the exit code from the contract.
func (a *app) fail(err error) int {
	code := exitCodeFor(err)

	if a.jsonOut {
		out := map[string]any{"ok": false, "error": err.Error(), "exit_code": code}
		var ve *hellojade.ValidationError
		if errors.As(err, &ve) {
			out["error"] = "validation_failed"
			out["fields"] = ve.Fields
			if ve.RequestID != "" {
				out["request_id"] = ve.RequestID
			}
		} else {
			var ae *hellojade.APIError
			if errors.As(err, &ae) {
				out["status"] = ae.StatusCode
				if ae.Code != "" {
					out["code"] = ae.Code
				}
				if ae.RequestID != "" {
					out["request_id"] = ae.RequestID
				}
				if ae.RetryAfter > 0 {
					out["retry_after_s"] = ae.RetryAfter.Seconds()
				}
			}
		}
		a.writeJSON(out)
		return code
	}

	var ve *hellojade.ValidationError
	if errors.As(err, &ve) {
		fmt.Fprintln(a.stderr, "hellojade: the server rejected the lead (422 validation_failed)")
		for _, name := range ve.FieldNames() {
			fmt.Fprintf(a.stderr, "  %-18s %s\n", name, ve.Fields[name])
		}
		if ve.RequestID != "" {
			fmt.Fprintf(a.stderr, "  request_id: %s\n", ve.RequestID)
		}
		a.info("every failing field is listed above. Fix them all and resend — do not retry this body unchanged, and note a 422 does not consume the Idempotency-Key.")
		return code
	}

	fmt.Fprintln(a.stderr, prefixed(err))
	var ae *hellojade.APIError
	if errors.As(err, &ae) && ae.RequestID != "" {
		fmt.Fprintf(a.stderr, "  request_id: %s\n", ae.RequestID)
	}
	switch code {
	case ExitAuth:
		a.info("401 is configuration, not code: check the key value for stray whitespace or a newline, check you are pointed at %s, then ask hellojade whether the key is still active.", a.resolvedBaseURL())
	case ExitRateLimited:
		a.info("still rate limited after waiting. The per-IP budget is shared by everyone behind your egress address, so this is not always about your volume. Doing a bulk backfill? Ask hellojade to raise the limit for a window.")
	case ExitServer:
		a.info("this is the server's side, not yours. The lead was not stored; resend it later with the same Idempotency-Key.")
	case ExitNetwork:
		if strings.HasPrefix(a.resolvedBaseURL(), "http://") {
			a.info("the base URL is http://. There is no listener on port 80 — it fails with a connection error, not a redirect. Use https://.")
		} else {
			a.info("transport failure. Check egress, DNS and TLS. The intake host has an IPv4 address and no AAAA record, so an IPv6-only egress cannot reach it.")
		}
	case ExitRejected:
		a.info("a 4xx other than 429 will not change on a retry. Fix the request; the request_id above is the handle support needs.")
	}
	return code
}

// exitCodeFor maps an error onto the documented exit codes.
func exitCodeFor(err error) int {
	switch {
	case err == nil:
		return ExitOK
	case errors.Is(err, hellojade.ErrNoAPIKey), errors.Is(err, hellojade.ErrReservedField):
		return ExitUsage
	case errors.Is(err, hellojade.ErrUnauthorized):
		return ExitAuth
	case errors.Is(err, hellojade.ErrRateLimited):
		return ExitRateLimited
	case errors.Is(err, hellojade.ErrNotAccepting), errors.Is(err, hellojade.ErrServer):
		return ExitServer
	case errors.Is(err, hellojade.ErrValidation),
		errors.Is(err, hellojade.ErrInvalidJSON),
		errors.Is(err, hellojade.ErrBodyTooLarge):
		return ExitRejected
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
		return ExitNetwork
	}
	var ae *hellojade.APIError
	if errors.As(err, &ae) {
		switch {
		case ae.StatusCode == 401:
			return ExitAuth
		case ae.StatusCode == 429:
			return ExitRateLimited
		case ae.StatusCode >= 500:
			return ExitServer
		case ae.StatusCode >= 400:
			return ExitRejected
		}
		return ExitError
	}
	var ne net.Error
	if errors.As(err, &ne) {
		return ExitNetwork
	}
	var ue *url.Error
	if errors.As(err, &ue) {
		return ExitNetwork
	}
	return ExitError
}

// prefixed renders an error with exactly one "hellojade: " prefix. The
// client library already prefixes its own errors.
func prefixed(err error) string {
	msg := err.Error()
	if strings.HasPrefix(msg, "hellojade:") {
		return msg
	}
	return "hellojade: " + msg
}
