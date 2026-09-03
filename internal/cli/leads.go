package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	hellojade "github.com/hellojade-ai/leads-go"
)

// extraFlag collects repeatable --extra key=value pairs. A value that parses
// as JSON is kept typed (numbers, booleans, objects); anything else is a
// string.
type extraFlag map[string]any

func (e *extraFlag) String() string { return "" }

func (e *extraFlag) Set(v string) error {
	k, val, ok := strings.Cut(v, "=")
	if !ok || k == "" {
		return fmt.Errorf("--extra wants key=value, got %q", v)
	}
	if *e == nil {
		*e = map[string]any{}
	}
	var parsed any
	if json.Unmarshal([]byte(val), &parsed) == nil {
		(*e)[k] = parsed
	} else {
		(*e)[k] = val
	}
	return nil
}

func (a *app) leads(args []string) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		a.printLeadsHelp()
		if len(args) == 0 {
			return ExitUsage
		}
		return ExitOK
	}
	if args[0] != "submit" {
		return a.usage(fmt.Sprintf("unknown leads command %q", args[0]))
	}
	return a.leadsSubmit(args[1:])
}

func (a *app) leadsSubmit(args []string) int {
	fs := a.flagSet("leads submit")
	var (
		lead     hellojade.Lead
		extra    extraFlag
		jsonSrc  string
		idemKey  string
		noIdem   bool
		dryRun   bool
		costFlag float64
	)
	fs.StringVar(&jsonSrc, "json-file", "", "read the lead as a JSON object from FILE, or '-' for stdin; flags override its fields")
	fs.StringVar(&lead.ExternalID, "external-id", "", "your id for the lead; also the default Idempotency-Key")
	fs.StringVar(&lead.FirstName, "first-name", "", "required")
	fs.StringVar(&lead.LastName, "last-name", "", "required")
	fs.StringVar(&lead.Phone, "phone", "", "required; any format")
	fs.StringVar(&lead.Email, "email", "", "")
	fs.StringVar(&lead.StreetAddress, "street-address", "", "")
	fs.StringVar(&lead.City, "city", "", "")
	fs.StringVar(&lead.State, "state", "", "2-letter US/CA code or free text")
	fs.StringVar(&lead.Zip, "zip", "", "")
	fs.StringVar(&lead.Country, "country", "", "ISO-3166 alpha-2; server defaults to US")
	fs.StringVar(&lead.ProjectArea, "project-area", "", "see 'hellojade vocabulary'; send your raw term if unsure")
	fs.StringVar((*string)(&lead.ProjectService), "project-service", "", "replacement | repair | remodel | maintain")
	fs.StringVar(&lead.ProjectMaterial, "project-material", "", "")
	fs.StringVar(&lead.ProjectDetails, "project-details", "", "free text; line breaks preserved")
	fs.Float64Var(&costFlag, "cost", 0, "USD you are charging for this contact, 0.01-999.99; omit when there is no charge")
	fs.Var(&extra, "extra", "unmodeled field as key=value (repeatable); JSON values are kept typed")
	fs.StringVar(&idemKey, "idempotency-key", "", "override the Idempotency-Key (default: --external-id)")
	fs.BoolVar(&noIdem, "no-idempotency-key", false, "send without an Idempotency-Key (not recommended)")
	fs.BoolVar(&dryRun, "dry-run", false, "print the request that would be sent and exit 0 without sending")

	if code, done := a.parse(fs, args, a.printLeadsHelp); done {
		return code
	}
	if fs.NArg() > 0 {
		return a.usage(fmt.Sprintf("leads submit: unexpected argument %q", fs.Arg(0)))
	}

	// Start from the JSON source, if any, then let explicitly set flags win.
	if jsonSrc != "" {
		var base hellojade.Lead
		raw, err := a.readSource(jsonSrc)
		if err != nil {
			return a.usage(err.Error())
		}
		if err := json.Unmarshal(raw, &base); err != nil {
			return a.usage(fmt.Sprintf("--json-file: not a JSON object: %v", err))
		}
		set := map[string]bool{}
		fs.Visit(func(f *flag.Flag) { set[f.Name] = true })
		merged := base
		override := func(name string, dst *string, src string) {
			if set[name] {
				*dst = src
			}
		}
		override("external-id", &merged.ExternalID, lead.ExternalID)
		override("first-name", &merged.FirstName, lead.FirstName)
		override("last-name", &merged.LastName, lead.LastName)
		override("phone", &merged.Phone, lead.Phone)
		override("email", &merged.Email, lead.Email)
		override("street-address", &merged.StreetAddress, lead.StreetAddress)
		override("city", &merged.City, lead.City)
		override("state", &merged.State, lead.State)
		override("zip", &merged.Zip, lead.Zip)
		override("country", &merged.Country, lead.Country)
		override("project-area", &merged.ProjectArea, lead.ProjectArea)
		if set["project-service"] {
			merged.ProjectService = lead.ProjectService
		}
		override("project-material", &merged.ProjectMaterial, lead.ProjectMaterial)
		override("project-details", &merged.ProjectDetails, lead.ProjectDetails)
		if set["cost"] {
			merged.Cost = costFlag
		}
		for k, v := range extra {
			if merged.Extra == nil {
				merged.Extra = map[string]any{}
			}
			merged.Extra[k] = v
		}
		lead = merged
	} else {
		lead.Cost = costFlag
		if len(extra) > 0 {
			lead.Extra = map[string]any(extra)
		}
	}

	// Refuse the obvious mistakes locally: they would cost a round trip and
	// a rate-limit token to learn from the server.
	if _, ok := lead.Extra["source"]; ok {
		return a.usage("leads submit: 'source' is not a field you send — it is your API key's registered label. Ask hellojade for a second key if you need a second source")
	}
	if lead.Cost != 0 && (lead.Cost < 0.01 || lead.Cost > 999.99) {
		return a.usage(fmt.Sprintf("leads submit: --cost %v is outside 0.01-999.99 (omit it when there is no charge; never send 0)", lead.Cost))
	}

	key := idemKey
	if key == "" {
		key = lead.ExternalID
	}
	if key == "" && !noIdem {
		return a.usage("leads submit: no Idempotency-Key. Pass --external-id (your own stable id for this lead, namespaced like acme-leads:1234) or --idempotency-key, or --no-idempotency-key to send without one")
	}
	if key != "" && !strings.ContainsAny(key, ":/-_.") {
		a.info("warning: Idempotency-Key %q looks un-namespaced. Dedupe is scoped to the whole customer, so prefix it with something only you use (acme-leads:%s).", key, key)
	}

	body, err := json.Marshal(&lead)
	if err != nil {
		return a.fail(err)
	}

	if dryRun {
		return a.dryRun(body, key)
	}

	c, err := a.client(true)
	if err != nil {
		return a.usage(err.Error())
	}
	ctx, cancel := a.ctx()
	defer cancel()
	acc, err := c.SubmitRaw(ctx, body, key, a.requestID)
	if err != nil {
		return a.fail(err)
	}
	if a.jsonOut {
		out := map[string]any{
			"ok":          true,
			"event_id":    acc.EventID,
			"status":      acc.Status,
			"received_at": acc.ReceivedAt,
			"flags":       acc.Flags,
		}
		if acc.Source != "" {
			out["source"] = acc.Source
		}
		if acc.RequestID != "" {
			out["request_id"] = acc.RequestID
		}
		a.writeJSON(out)
		return ExitOK
	}
	flags := "none"
	if len(acc.Flags) > 0 {
		parts := make([]string, len(acc.Flags))
		for i, f := range acc.Flags {
			parts[i] = string(f)
		}
		flags = strings.Join(parts, ",")
	}
	fmt.Fprintf(a.stdout, "%s %s", acc.Status, acc.EventID)
	if acc.Source != "" {
		fmt.Fprintf(a.stdout, " source=%s", acc.Source)
	}
	fmt.Fprintf(a.stdout, " flags=%s", flags)
	if acc.RequestID != "" {
		fmt.Fprintf(a.stdout, " request_id=%s", acc.RequestID)
	}
	fmt.Fprintln(a.stdout)
	if acc.Duplicate() {
		a.info("duplicate: the server already had this Idempotency-Key; the event_id above is the original. This is success.")
	}
	return ExitOK
}

func (a *app) readSource(src string) ([]byte, error) {
	if src == "-" {
		b, err := io.ReadAll(a.stdin)
		if err != nil {
			return nil, fmt.Errorf("reading stdin: %w", err)
		}
		return b, nil
	}
	b, err := os.ReadFile(src)
	if err != nil {
		return nil, fmt.Errorf("--json-file: %w", err)
	}
	return b, nil
}

// dryRun prints the request without sending it. The key is never printed;
// only whether one is configured.
func (a *app) dryRun(body []byte, idemKey string) int {
	key, err := a.apiKey()
	if err != nil {
		return a.usage(err.Error())
	}
	keyState := "<not configured>"
	if key != "" {
		keyState = "<redacted, configured>"
	}
	var pretty bytes.Buffer
	_ = json.Indent(&pretty, body, "", "  ")
	if a.jsonOut {
		var obj map[string]any
		_ = json.Unmarshal(body, &obj)
		headers := map[string]string{"Content-Type": "application/json", "X-API-Key": keyState}
		if idemKey != "" {
			headers["Idempotency-Key"] = idemKey
		}
		if a.requestID != "" {
			headers["X-Request-Id"] = a.requestID
		}
		a.writeJSON(map[string]any{
			"dry_run": true,
			"method":  "POST",
			"url":     a.resolvedBaseURL() + "/v1/intake",
			"headers": headers,
			"body":    obj,
		})
		return ExitOK
	}
	fmt.Fprintf(a.stdout, "POST %s/v1/intake\n", a.resolvedBaseURL())
	fmt.Fprintf(a.stdout, "X-API-Key: %s\n", keyState)
	fmt.Fprintln(a.stdout, "Content-Type: application/json")
	if idemKey != "" {
		fmt.Fprintf(a.stdout, "Idempotency-Key: %s\n", idemKey)
	}
	if a.requestID != "" {
		fmt.Fprintf(a.stdout, "X-Request-Id: %s\n", a.requestID)
	}
	fmt.Fprintf(a.stdout, "\n%s\n", pretty.String())
	a.info("dry run: nothing was sent")
	return ExitOK
}

func (a *app) printLeadsHelp() {
	fmt.Fprint(a.stdout, `hellojade leads submit — post one lead

USAGE
  hellojade leads submit [flags]
  hellojade leads submit --json-file lead.json [flags]
  cat lead.json | hellojade leads submit --json-file - [flags]

Only --first-name, --last-name and --phone are required. Send everything
you have and nothing you do not; empty flags are omitted from the body.

IDEMPOTENCY
  Always send an Idempotency-Key: your OWN stable id for the lead, so a
  retry of the same lead carries the same key. --external-id is used by
  default; --idempotency-key overrides it. Namespace it (acme-leads:1234,
  not 1234) — dedupe is scoped to the whole customer, and a bare id another
  source already used returns 200 pointing at THEIR event.

  202 accepted   new event, committed to disk         exit 0
  200 duplicate  same key seen before, same event_id  exit 0 (success)

LEAD FLAGS
  --external-id ID           your id; default Idempotency-Key
  --first-name S             required
  --last-name S              required
  --phone S                  required; any format, normalized server-side
  --email S
  --street-address S  --city S  --state S  --zip S  --country CC
  --project-area S           see 'hellojade vocabulary'; raw term if unsure
  --project-service S        replacement | repair | remodel | maintain
  --project-material S
  --project-details S        free text; line breaks preserved
  --cost N                   USD, 0.01-999.99; omit when no charge (never 0)
  --extra k=v                unmodeled field, repeatable; preserved server-side
  --json-file FILE|-         JSON object to start from; flags override fields

CONTROL FLAGS
  --idempotency-key K        explicit key (default --external-id)
  --no-idempotency-key       send without one (not recommended)
  --request-id S             X-Request-Id, echoed by the server for support
  --dry-run                  print the request; send nothing; exit 0
  --json                     machine-readable output

RETRIES
  5xx and network errors: retried with backoff up to --max-attempts.
  429: waits Retry-After (at least) and does not consume an attempt.
  Any other 4xx: returned at once with exit 4. Never retried.

EXAMPLES
  hellojade leads submit --external-id acme-leads:1234 \
      --first-name Dana --last-name Whitfield --phone "(630) 555-0142" \
      --project-area roof --project-service replacement --cost 55

  hellojade leads submit --json-file - --dry-run <<'EOF'
  {"external_id":"acme-leads:1234","first_name":"Dana","last_name":"Whitfield","phone":"6305550142"}
  EOF

Do NOT test against production with a made-up name and phone: a real
salesperson gets the call. Use 'hellojade auth check', --dry-run, or ask
hellojade for a sandbox key.
`)
}

// errNotFound is used by tests to assert stdin handling.
var _ = errors.New
