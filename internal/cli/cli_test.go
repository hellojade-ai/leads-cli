package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

// testKey is the value every test configures. No test may ever print it, so
// TestAPIKeyNeverPrinted greps all output for it.
const testKey = "hj_live_SECRET_dead_beef_0123456789"

// stub is a local intake server. Every test in this file runs against one;
// nothing here ever reaches the network.
type stub struct {
	*httptest.Server
	requests atomic.Int64
	last     struct {
		method  string
		path    string
		headers http.Header
		body    []byte
	}
}

// newStub builds a server whose /v1/intake behavior is supplied per test.
// Vocabulary and health are always the documented shapes.
func newStub(t *testing.T, intake http.HandlerFunc) *stub {
	t.Helper()
	s := &stub{}
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/intake", func(w http.ResponseWriter, r *http.Request) {
		s.requests.Add(1)
		body, _ := readAll(r)
		r.Body = io.NopCloser(bytes.NewReader(body)) // let the handler read it too
		s.last.method, s.last.path, s.last.headers, s.last.body = r.Method, r.URL.Path, r.Header.Clone(), body
		w.Header().Set("X-Request-Id", "req_stub_1")
		intake(w, r)
	})
	mux.HandleFunc("/v1/vocabulary", func(w http.ResponseWriter, r *http.Request) {
		writeJSONResp(w, 200, map[string]any{
			"project_area": []map[string]string{
				{"area": "roof", "status": "confirmed"},
				{"area": "solar", "status": "proposed"},
				{"area": "gutters", "status": "confirmed"},
			},
			"project_service": []string{"replacement", "repair", "remodel", "maintain"},
			"required":        []string{"first_name", "last_name", "phone"},
		})
	})
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSONResp(w, 200, map[string]any{
			"ok": true, "store_writable": true, "pending": 0, "dead": 0,
			"oldest_pending_age_s": nil,
		})
	})
	s.Server = httptest.NewServer(mux)
	t.Cleanup(s.Close)
	return s
}

func readAll(r *http.Request) ([]byte, error) {
	buf := new(bytes.Buffer)
	_, err := buf.ReadFrom(r.Body)
	return buf.Bytes(), err
}

func writeJSONResp(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

// accepted is the documented 202 body.
func accepted(w http.ResponseWriter, r *http.Request) {
	writeJSONResp(w, 202, map[string]any{
		"event_id": "evt_0198f2c1a4b00000a3d19f4c2b7e", "status": "accepted",
		"received_at": "2026-08-21T14:03:22Z", "source": "acme-leads", "flags": []string{},
	})
}

// emptyBodyValidator answers the §1 key check: 401 without the key, 422 for
// an empty object, 202 otherwise. It is the default intake handler.
func emptyBodyValidator(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("X-API-Key") != testKey {
		writeJSONResp(w, 401, map[string]any{"error": "unauthorized", "request_id": "req_stub_1"})
		return
	}
	body, _ := readAll(r)
	if strings.TrimSpace(string(body)) == "{}" {
		writeJSONResp(w, 422, map[string]any{
			"error": "validation_failed", "request_id": "req_stub_1",
			"fields": map[string]string{"first_name": "required", "last_name": "required", "phone": "required"},
		})
		return
	}
	accepted(w, r)
}

// run drives Main with a stub base URL and the test key, and returns the
// exit code plus both streams.
type result struct {
	code   int
	stdout string
	stderr string
}

func (r result) all() string { return r.stdout + r.stderr }

func run(t *testing.T, s *stub, stdin string, args ...string) result {
	t.Helper()
	env := map[string]string{"HELLOJADE_API_KEY": testKey}
	if s != nil {
		env["HELLOJADE_BASE_URL"] = s.URL
	}
	return runEnv(t, env, stdin, args...)
}

func runEnv(t *testing.T, env map[string]string, stdin string, args ...string) result {
	t.Helper()
	var out, errOut bytes.Buffer
	code := Main(args, strings.NewReader(stdin), &out, &errOut, func(k string) string { return env[k] })
	return result{code, out.String(), errOut.String()}
}

// -------------------------------------------------------------------------

func TestVersion(t *testing.T) {
	for _, args := range [][]string{{"version"}, {"--version"}, {"-v"}} {
		got := runEnv(t, nil, "", args...)
		if got.code != ExitOK {
			t.Fatalf("%v: exit %d, want 0 (%s)", args, got.code, got.all())
		}
		if !strings.Contains(got.stdout, Version) {
			t.Errorf("%v: stdout %q does not contain version %q", args, got.stdout, Version)
		}
	}
	got := runEnv(t, nil, "", "--json", "version")
	var v map[string]string
	if err := json.Unmarshal([]byte(got.stdout), &v); err != nil {
		t.Fatalf("--json version: %v (%q)", err, got.stdout)
	}
	if v["hellojade_cli"] != Version || v["hellojade_go"] == "" {
		t.Errorf("--json version: %v", v)
	}
}

func TestNoArgsIsUsage(t *testing.T) {
	got := runEnv(t, nil, "")
	if got.code != ExitUsage {
		t.Fatalf("exit %d, want %d", got.code, ExitUsage)
	}
	if !strings.Contains(got.stdout, "auth check") {
		t.Errorf("bare invocation should print the command list, got %q", got.stdout)
	}
}

func TestHelpTopics(t *testing.T) {
	for _, topic := range []string{"auth", "leads", "vocabulary", "health", "completion", "exit-codes"} {
		got := runEnv(t, nil, "", "help", topic)
		if got.code != ExitOK {
			t.Errorf("help %s: exit %d (%s)", topic, got.code, got.all())
		}
		if len(got.stdout) < 200 {
			t.Errorf("help %s: only %d bytes of help", topic, len(got.stdout))
		}
	}
	if got := runEnv(t, nil, "", "help", "nonsense"); got.code != ExitUsage {
		t.Errorf("help nonsense: exit %d, want %d", got.code, ExitUsage)
	}
}

func TestUnknownCommand(t *testing.T) {
	got := runEnv(t, nil, "", "frobnicate")
	if got.code != ExitUsage {
		t.Fatalf("exit %d, want %d", got.code, ExitUsage)
	}
	if !strings.Contains(got.stderr, "frobnicate") {
		t.Errorf("stderr should name the command: %q", got.stderr)
	}
}

func TestAuthCheckValidKey(t *testing.T) {
	s := newStub(t, emptyBodyValidator)
	got := run(t, s, "", "auth", "check")
	if got.code != ExitOK {
		t.Fatalf("exit %d, want 0 (%s)", got.code, got.all())
	}
	if s.requests.Load() != 1 {
		t.Errorf("made %d requests, want 1", s.requests.Load())
	}
	if b := strings.TrimSpace(string(s.last.body)); b != "{}" {
		t.Errorf("key check body = %q, want {} — it must store nothing", b)
	}
	if s.last.headers.Get("X-API-Key") != testKey {
		t.Errorf("key not sent in X-API-Key")
	}
}

func TestAuthCheckJSON(t *testing.T) {
	s := newStub(t, emptyBodyValidator)
	got := run(t, s, "", "auth", "check", "--json")
	var v map[string]any
	if err := json.Unmarshal([]byte(got.stdout), &v); err != nil {
		t.Fatalf("not JSON: %v (%q)", err, got.stdout)
	}
	if v["valid"] != true || v["ok"] != true {
		t.Errorf("want valid+ok true, got %v", v)
	}
}

func TestAuthCheckBadKeyExits3(t *testing.T) {
	s := newStub(t, emptyBodyValidator)
	got := runEnv(t, map[string]string{"HELLOJADE_API_KEY": "wrong", "HELLOJADE_BASE_URL": s.URL},
		"", "auth", "check")
	if got.code != ExitAuth {
		t.Fatalf("exit %d, want %d (%s)", got.code, ExitAuth, got.all())
	}
	if !strings.Contains(got.stderr, "configuration") {
		t.Errorf("a 401 should be explained as configuration: %q", got.stderr)
	}
}

func TestAuthCheckNoKeyIsUsage(t *testing.T) {
	s := newStub(t, emptyBodyValidator)
	got := runEnv(t, map[string]string{"HELLOJADE_BASE_URL": s.URL}, "", "auth", "check")
	if got.code != ExitUsage {
		t.Fatalf("exit %d, want %d (%s)", got.code, ExitUsage, got.all())
	}
	if s.requests.Load() != 0 {
		t.Errorf("a missing key must be caught locally; %d requests were made", s.requests.Load())
	}
}

func TestAuthCheckAPIKeyFile(t *testing.T) {
	s := newStub(t, emptyBodyValidator)
	path := filepath.Join(t.TempDir(), "key")
	// A trailing newline is the normal shape of a secret file and must be
	// trimmed: a newline in the header is a classic silent 401.
	if err := os.WriteFile(path, []byte(testKey+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got := runEnv(t, map[string]string{"HELLOJADE_BASE_URL": s.URL}, "",
		"--api-key-file", path, "auth", "check")
	if got.code != ExitOK {
		t.Fatalf("exit %d, want 0 (%s)", got.code, got.all())
	}
}

func TestSubmitFlags(t *testing.T) {
	s := newStub(t, emptyBodyValidator)
	got := run(t, s, "", "leads", "submit",
		"--external-id", "acme-leads:1234",
		"--first-name", "Dana", "--last-name", "Whitfield",
		"--phone", "(630) 555-0142",
		"--project-area", "roof", "--project-service", "replacement",
		"--cost", "55",
		"--extra", "partner_job_id=XZ-1",
		"--extra", "score=91",
		"--request-id", "acme-leads/1234/attempt-1",
	)
	if got.code != ExitOK {
		t.Fatalf("exit %d, want 0 (%s)", got.code, got.all())
	}
	var sent map[string]any
	if err := json.Unmarshal(s.last.body, &sent); err != nil {
		t.Fatalf("body not JSON: %v", err)
	}
	for k, want := range map[string]any{
		"first_name": "Dana", "last_name": "Whitfield", "phone": "(630) 555-0142",
		"project_area": "roof", "project_service": "replacement",
		"external_id": "acme-leads:1234", "partner_job_id": "XZ-1",
	} {
		if sent[k] != want {
			t.Errorf("body[%q] = %v, want %v", k, sent[k], want)
		}
	}
	// A JSON-looking --extra value keeps its type.
	if sent["score"] != float64(91) {
		t.Errorf("body[score] = %#v, want the number 91", sent["score"])
	}
	if sent["cost"] != float64(55) {
		t.Errorf("body[cost] = %#v", sent["cost"])
	}
	// Rule 6: source is never a request field.
	if _, ok := sent["source"]; ok {
		t.Error("the client sent a source field; it is not one")
	}
	if h := s.last.headers.Get("Idempotency-Key"); h != "acme-leads:1234" {
		t.Errorf("Idempotency-Key = %q, want the external_id", h)
	}
	if h := s.last.headers.Get("X-Request-Id"); h != "acme-leads/1234/attempt-1" {
		t.Errorf("X-Request-Id = %q", h)
	}
	if !strings.Contains(got.stdout, "evt_0198f2c1a4b00000a3d19f4c2b7e") {
		t.Errorf("stdout should carry the event_id: %q", got.stdout)
	}
}

func TestSubmitJSONStdinAndFlagOverride(t *testing.T) {
	s := newStub(t, emptyBodyValidator)
	in := `{"external_id":"acme-leads:9","first_name":"Dana","last_name":"Whitfield",
	        "phone":"6305550142","city":"Naperville","partner_job_id":"XZ-9"}`
	got := run(t, s, in, "leads", "submit", "--json-file", "-", "--city", "Aurora")
	if got.code != ExitOK {
		t.Fatalf("exit %d (%s)", got.code, got.all())
	}
	var sent map[string]any
	if err := json.Unmarshal(s.last.body, &sent); err != nil {
		t.Fatal(err)
	}
	if sent["city"] != "Aurora" {
		t.Errorf("an explicitly set flag must beat the JSON source: city = %v", sent["city"])
	}
	if sent["first_name"] != "Dana" {
		t.Errorf("an unset flag must not blank a JSON field: first_name = %v", sent["first_name"])
	}
	if sent["partner_job_id"] != "XZ-9" {
		t.Errorf("unmodeled fields from the JSON source must survive: %v", sent)
	}
	if h := s.last.headers.Get("Idempotency-Key"); h != "acme-leads:9" {
		t.Errorf("Idempotency-Key = %q", h)
	}
}

func TestSubmitJSONFile(t *testing.T) {
	s := newStub(t, emptyBodyValidator)
	path := filepath.Join(t.TempDir(), "lead.json")
	if err := os.WriteFile(path, []byte(`{"external_id":"acme:7","first_name":"A","last_name":"B","phone":"1"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := run(t, s, "", "leads", "submit", "--json-file", path); got.code != ExitOK {
		t.Fatalf("exit %d (%s)", got.code, got.all())
	}
	got := run(t, s, "", "leads", "submit", "--json-file", filepath.Join(t.TempDir(), "missing.json"))
	if got.code != ExitUsage {
		t.Errorf("missing file: exit %d, want %d", got.code, ExitUsage)
	}
}

func TestSubmitDryRunSendsNothing(t *testing.T) {
	s := newStub(t, emptyBodyValidator)
	got := run(t, s, "", "leads", "submit", "--dry-run",
		"--external-id", "acme-leads:1", "--first-name", "Dana",
		"--last-name", "Whitfield", "--phone", "6305550142")
	if got.code != ExitOK {
		t.Fatalf("exit %d (%s)", got.code, got.all())
	}
	if n := s.requests.Load(); n != 0 {
		t.Fatalf("--dry-run made %d requests; it must make none", n)
	}
	if !strings.Contains(got.stdout, "POST "+s.URL+"/v1/intake") {
		t.Errorf("dry run should print the target URL: %q", got.stdout)
	}
	if !strings.Contains(got.stdout, "Idempotency-Key: acme-leads:1") {
		t.Errorf("dry run should print the Idempotency-Key: %q", got.stdout)
	}
	if !strings.Contains(got.stdout, `"first_name": "Dana"`) {
		t.Errorf("dry run should pretty-print the body: %q", got.stdout)
	}
	if strings.Contains(got.all(), testKey) {
		t.Error("the dry run printed the API key")
	}
	if !strings.Contains(got.stdout, "redacted") {
		t.Errorf("dry run should show the key as redacted: %q", got.stdout)
	}
}

func TestSubmitDryRunJSON(t *testing.T) {
	s := newStub(t, emptyBodyValidator)
	got := run(t, s, "", "--json", "leads", "submit", "--dry-run",
		"--external-id", "acme:1", "--first-name", "A", "--last-name", "B", "--phone", "1")
	var v map[string]any
	if err := json.Unmarshal([]byte(got.stdout), &v); err != nil {
		t.Fatalf("not JSON: %v (%q)", err, got.stdout)
	}
	if v["dry_run"] != true || v["method"] != "POST" {
		t.Errorf("unexpected dry-run JSON: %v", v)
	}
	body, _ := v["body"].(map[string]any)
	if body["first_name"] != "A" {
		t.Errorf("dry-run JSON body = %v", body)
	}
	if strings.Contains(got.all(), testKey) {
		t.Error("the JSON dry run printed the API key")
	}
}

func TestSubmitRefusesLocally(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"no idempotency key",
			[]string{"leads", "submit", "--first-name", "A", "--last-name", "B", "--phone", "1"},
			"Idempotency-Key"},
		{"cost out of range",
			[]string{"leads", "submit", "--external-id", "acme:1", "--first-name", "A",
				"--last-name", "B", "--phone", "1", "--cost", "1500"},
			"0.01-999.99"},
		{"source in extra",
			[]string{"leads", "submit", "--external-id", "acme:1", "--first-name", "A",
				"--last-name", "B", "--phone", "1", "--extra", "source=acme"},
			"not a field"},
		{"unexpected positional",
			[]string{"leads", "submit", "oops"},
			"unexpected argument"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := newStub(t, emptyBodyValidator)
			got := run(t, s, "", tc.args...)
			if got.code != ExitUsage {
				t.Fatalf("exit %d, want %d (%s)", got.code, ExitUsage, got.all())
			}
			if s.requests.Load() != 0 {
				t.Errorf("%d requests made; this must be caught before sending", s.requests.Load())
			}
			if !strings.Contains(got.stderr, tc.want) {
				t.Errorf("stderr %q does not mention %q", got.stderr, tc.want)
			}
		})
	}
}

func TestSubmitWarnsOnUnnamespacedKey(t *testing.T) {
	s := newStub(t, emptyBodyValidator)
	got := run(t, s, "", "leads", "submit", "--external-id", "1234",
		"--first-name", "A", "--last-name", "B", "--phone", "1")
	if got.code != ExitOK {
		t.Fatalf("exit %d (%s)", got.code, got.all())
	}
	if !strings.Contains(got.stderr, "namespace") {
		t.Errorf("a bare integer key should draw the namespacing warning: %q", got.stderr)
	}
	// The warning is advisory: the lead still goes.
	if s.requests.Load() != 1 {
		t.Errorf("the warning must not block the send")
	}
}

func TestSubmitDuplicateIsSuccess(t *testing.T) {
	s := newStub(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSONResp(w, 200, map[string]any{
			"event_id": "evt_original", "status": "duplicate",
			"received_at": "2026-08-21T14:03:22Z", "source": "acme-leads", "flags": []string{},
		})
	})
	got := run(t, s, "", "leads", "submit", "--external-id", "acme:1",
		"--first-name", "A", "--last-name", "B", "--phone", "1")
	if got.code != ExitOK {
		t.Fatalf("a 200 duplicate is success; exit %d (%s)", got.code, got.all())
	}
	if !strings.Contains(got.stdout, "duplicate evt_original") {
		t.Errorf("stdout = %q", got.stdout)
	}
	if !strings.Contains(got.stderr, "This is success") {
		t.Errorf("the duplicate note should say so: %q", got.stderr)
	}
}

func TestSubmitFlagsAreNotErrors(t *testing.T) {
	s := newStub(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSONResp(w, 202, map[string]any{
			"event_id": "evt_1", "status": "accepted", "received_at": "2026-08-21T14:03:22Z",
			"source": "acme-leads",
			"flags":  []string{"phone_unnormalized", "project_area_unknown"},
		})
	})
	got := run(t, s, "", "leads", "submit", "--external-id", "acme:1",
		"--first-name", "A", "--last-name", "B", "--phone", "xyz")
	if got.code != ExitOK {
		t.Fatalf("flags are not errors; exit %d (%s)", got.code, got.all())
	}
	if !strings.Contains(got.stdout, "flags=phone_unnormalized,project_area_unknown") {
		t.Errorf("stdout = %q", got.stdout)
	}
}

func TestSubmitValidationExits4AndListsEveryField(t *testing.T) {
	s := newStub(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSONResp(w, 422, map[string]any{
			"error": "validation_failed", "request_id": "req_abc",
			"fields": map[string]string{"first_name": "required", "last_name": "required", "phone": "required"},
		})
	})
	got := run(t, s, "", "leads", "submit", "--external-id", "acme:1",
		"--first-name", "A", "--last-name", "B", "--phone", "1")
	if got.code != ExitRejected {
		t.Fatalf("exit %d, want %d (%s)", got.code, ExitRejected, got.all())
	}
	for _, f := range []string{"first_name", "last_name", "phone", "req_abc"} {
		if !strings.Contains(got.stderr, f) {
			t.Errorf("stderr must name %q (all failing fields at once): %q", f, got.stderr)
		}
	}
	if s.requests.Load() != 1 {
		t.Errorf("a 422 must never be retried; %d requests made", s.requests.Load())
	}
}

func TestSubmitValidationJSON(t *testing.T) {
	s := newStub(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSONResp(w, 422, map[string]any{
			"error": "validation_failed", "request_id": "req_abc",
			"fields": map[string]string{"phone": "required"},
		})
	})
	got := run(t, s, "", "--json", "leads", "submit", "--external-id", "acme:1",
		"--first-name", "A", "--last-name", "B", "--phone", "1")
	if got.code != ExitRejected {
		t.Fatalf("exit %d, want %d", got.code, ExitRejected)
	}
	var v map[string]any
	if err := json.Unmarshal([]byte(got.stdout), &v); err != nil {
		t.Fatalf("not JSON: %v (%q)", err, got.stdout)
	}
	fields, _ := v["fields"].(map[string]any)
	if fields["phone"] != "required" {
		t.Errorf("fields = %v", v["fields"])
	}
	if v["request_id"] != "req_abc" {
		t.Errorf("request_id = %v", v["request_id"])
	}
}

func TestSubmitServerErrorRetriesThenExits6(t *testing.T) {
	s := newStub(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSONResp(w, 503, map[string]any{"error": "not_accepting", "request_id": "req_abc"})
	})
	got := run(t, s, "", "--max-attempts", "3", "--retry-backoff", "1ms",
		"leads", "submit", "--external-id", "acme:1",
		"--first-name", "A", "--last-name", "B", "--phone", "1")
	if got.code != ExitServer {
		t.Fatalf("exit %d, want %d (%s)", got.code, ExitServer, got.all())
	}
	if n := s.requests.Load(); n != 3 {
		t.Errorf("made %d attempts, want 3 (--max-attempts)", n)
	}
	if !strings.Contains(got.stderr, "server's side") {
		t.Errorf("a 503 should be explained as theirs: %q", got.stderr)
	}
}

func TestSubmitNoRetryMakesOneAttempt(t *testing.T) {
	s := newStub(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSONResp(w, 503, map[string]any{"error": "not_accepting"})
	})
	got := run(t, s, "", "--no-retry", "leads", "submit", "--external-id", "acme:1",
		"--first-name", "A", "--last-name", "B", "--phone", "1")
	if got.code != ExitServer {
		t.Fatalf("exit %d, want %d", got.code, ExitServer)
	}
	if n := s.requests.Load(); n != 1 {
		t.Errorf("--no-retry made %d attempts, want 1", n)
	}
}

func TestSubmitRateLimitDoesNotConsumeAttempts(t *testing.T) {
	s := newStub(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "0")
		writeJSONResp(w, 429, map[string]any{"error": "rate_limited"})
	})
	got := run(t, s, "", "--max-attempts", "2", "--retry-backoff", "1ms",
		"leads", "submit", "--external-id", "acme:1",
		"--first-name", "A", "--last-name", "B", "--phone", "1")
	if got.code != ExitRateLimited {
		t.Fatalf("exit %d, want %d (%s)", got.code, ExitRateLimited, got.all())
	}
	// A 429 does not consume a delivery attempt, so the client waits it out
	// MaxRateLimitWaits times — far more than --max-attempts=2.
	if n := s.requests.Load(); n <= 2 {
		t.Errorf("made %d requests; a 429 must not consume a delivery attempt", n)
	}
}

func TestSubmitBodyTooLargeExits4(t *testing.T) {
	s := newStub(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Request-Id", "req_413")
		writeJSONResp(w, 413, map[string]any{"error": "body_too_large"})
	})
	got := run(t, s, "", "leads", "submit", "--external-id", "acme:1",
		"--first-name", "A", "--last-name", "B", "--phone", "1")
	if got.code != ExitRejected {
		t.Fatalf("exit %d, want %d (%s)", got.code, ExitRejected, got.all())
	}
	// 413 carries no request_id in the body; the header must be used.
	if !strings.Contains(got.stderr, "req_413") {
		t.Errorf("the X-Request-Id header must be surfaced: %q", got.stderr)
	}
	if s.requests.Load() != 1 {
		t.Errorf("a 413 must not be retried")
	}
}

func TestNetworkFailureExits7(t *testing.T) {
	// A port nothing listens on: a connection error, the shape of the
	// http:// mistake the brief warns about.
	got := runEnv(t, map[string]string{
		"HELLOJADE_API_KEY": testKey, "HELLOJADE_BASE_URL": "http://127.0.0.1:1",
	}, "", "--max-attempts", "2", "--retry-backoff", "1ms", "--timeout", "2s", "auth", "check")
	if got.code != ExitNetwork {
		t.Fatalf("exit %d, want %d (%s)", got.code, ExitNetwork, got.all())
	}
	if !strings.Contains(got.stderr, "port 80") {
		t.Errorf("an http:// base URL should draw the port-80 note: %q", got.stderr)
	}
}

func TestVocabulary(t *testing.T) {
	s := newStub(t, emptyBodyValidator)
	got := run(t, s, "", "vocabulary")
	if got.code != ExitOK {
		t.Fatalf("exit %d (%s)", got.code, got.all())
	}
	for _, want := range []string{"roof", "solar", "confirmed", "proposed", "first_name"} {
		if !strings.Contains(got.stdout, want) {
			t.Errorf("stdout missing %q: %q", want, got.stdout)
		}
	}

	areas := run(t, s, "", "vocabulary", "--areas")
	lines := strings.Fields(strings.TrimSpace(areas.stdout))
	if len(lines) != 3 {
		t.Errorf("--areas printed %d terms, want 3: %q", len(lines), areas.stdout)
	}

	j := run(t, s, "", "--json", "vocabulary")
	var v struct {
		ProjectArea []struct{ Area, Status string } `json:"project_area"`
		Required    []string                        `json:"required"`
	}
	if err := json.Unmarshal([]byte(j.stdout), &v); err != nil {
		t.Fatalf("not JSON: %v", err)
	}
	if len(v.ProjectArea) != 3 || len(v.Required) != 3 {
		t.Errorf("json vocabulary = %+v", v)
	}
}

func TestVocabularyNeedsNoKey(t *testing.T) {
	s := newStub(t, emptyBodyValidator)
	got := runEnv(t, map[string]string{"HELLOJADE_BASE_URL": s.URL}, "", "vocabulary")
	if got.code != ExitOK {
		t.Fatalf("vocabulary is unauthenticated; exit %d (%s)", got.code, got.all())
	}
}

func TestHealth(t *testing.T) {
	s := newStub(t, emptyBodyValidator)
	got := run(t, s, "", "health")
	if got.code != ExitOK {
		t.Fatalf("exit %d (%s)", got.code, got.all())
	}
	if !strings.Contains(got.stdout, "store_writable=true") {
		t.Errorf("stdout = %q", got.stdout)
	}
}

func TestHealthDegradedExits6(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSONResp(w, 200, map[string]any{
			"ok": false, "store_writable": false, "pending": 12, "dead": 3,
			"oldest_pending_age_s": 900,
		})
	}))
	defer srv.Close()
	got := runEnv(t, map[string]string{"HELLOJADE_BASE_URL": srv.URL}, "", "health")
	if got.code != ExitServer {
		t.Fatalf("exit %d, want %d (%s)", got.code, ExitServer, got.all())
	}
	if !strings.Contains(got.stdout, "DEGRADED") || !strings.Contains(got.stdout, "oldest_pending_age_s=900") {
		t.Errorf("stdout = %q", got.stdout)
	}
}

func TestCompletionScripts(t *testing.T) {
	for _, shell := range []string{"bash", "zsh", "fish"} {
		got := runEnv(t, nil, "", "completion", shell)
		if got.code != ExitOK {
			t.Fatalf("completion %s: exit %d (%s)", shell, got.code, got.all())
		}
		if len(got.stdout) < 300 {
			t.Errorf("completion %s: only %d bytes", shell, len(got.stdout))
		}
		// Every command in the tree must appear in every script.
		for _, cmd := range commandTree[""] {
			if !strings.Contains(got.stdout, cmd) {
				t.Errorf("completion %s omits command %q", shell, cmd)
			}
		}
	}
	if got := runEnv(t, nil, "", "completion", "powershell"); got.code != ExitUsage {
		t.Errorf("unsupported shell: exit %d, want %d", got.code, ExitUsage)
	}
	if got := runEnv(t, nil, "", "completion"); got.code != ExitUsage {
		t.Errorf("bare completion: exit %d, want %d", got.code, ExitUsage)
	}
}

// The completion scripts are generated from these tables; if a command or a
// submit flag is added and the table is not updated, completion silently
// goes stale. Assert the tables against the real help output instead.
func TestCompletionTablesMatchHelp(t *testing.T) {
	help := runEnv(t, nil, "", "help").stdout
	for _, cmd := range commandTree[""] {
		if !strings.Contains(help, cmd) {
			t.Errorf("commandTree lists %q but the top-level help does not mention it", cmd)
		}
	}
	submitHelp := runEnv(t, nil, "", "help", "leads").stdout
	for _, f := range submitFlagNames {
		if !strings.Contains(submitHelp, f) {
			t.Errorf("submitFlagNames lists %q but 'help leads' does not document it", f)
		}
	}
}

func TestGlobalFlagsWorkAfterTheCommand(t *testing.T) {
	s := newStub(t, emptyBodyValidator)
	got := run(t, s, "", "auth", "check", "--json")
	if got.code != ExitOK {
		t.Fatalf("exit %d (%s)", got.code, got.all())
	}
	if !strings.HasPrefix(strings.TrimSpace(got.stdout), "{") {
		t.Errorf("--json after the command should still take effect: %q", got.stdout)
	}
}

func TestQuietSuppressesNotes(t *testing.T) {
	s := newStub(t, emptyBodyValidator)
	loud := run(t, s, "", "auth", "check")
	quiet := run(t, s, "", "--quiet", "auth", "check")
	if loud.stderr == "" {
		t.Fatal("expected an advisory note without --quiet")
	}
	if quiet.stderr != "" {
		t.Errorf("--quiet should silence notes, got %q", quiet.stderr)
	}
}

func TestBadGlobalFlagValues(t *testing.T) {
	for _, args := range [][]string{
		{"--timeout", "0", "health"},
		{"--max-attempts", "0", "health"},
		{"--timeout", "nonsense", "health"},
		{"--base-url"},
	} {
		if got := runEnv(t, nil, "", args...); got.code != ExitUsage {
			t.Errorf("%v: exit %d, want %d", args, got.code, ExitUsage)
		}
	}
}

// The one invariant worth a test of its own: the key never reaches any
// stream, on any path, in either output mode.
func TestAPIKeyNeverPrinted(t *testing.T) {
	s := newStub(t, emptyBodyValidator)
	invocations := [][]string{
		{"auth", "check"},
		{"--json", "auth", "check"},
		{"leads", "submit", "--dry-run", "--external-id", "acme:1", "--first-name", "A", "--last-name", "B", "--phone", "1"},
		{"--json", "leads", "submit", "--dry-run", "--external-id", "acme:1", "--first-name", "A", "--last-name", "B", "--phone", "1"},
		{"leads", "submit", "--external-id", "acme:1", "--first-name", "A", "--last-name", "B", "--phone", "1"},
		{"vocabulary"}, {"health"}, {"version"}, {"help"},
	}
	for _, args := range invocations {
		got := run(t, s, "", args...)
		if strings.Contains(got.all(), testKey) {
			t.Errorf("%v printed the API key", args)
		}
	}
	// And on the failure paths, where an error message is most likely to
	// carry it.
	bad := newStub(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSONResp(w, 401, map[string]any{"error": "unauthorized"})
	})
	for _, args := range [][]string{{"auth", "check"}, {"--json", "auth", "check"}} {
		got := run(t, bad, "", args...)
		if strings.Contains(got.all(), testKey) {
			t.Errorf("%v printed the API key on the 401 path", args)
		}
	}
}

func TestExitCodeForNil(t *testing.T) {
	if got := exitCodeFor(nil); got != ExitOK {
		t.Errorf("exitCodeFor(nil) = %d", got)
	}
}

func TestSplitGlobals(t *testing.T) {
	cases := []struct {
		in            []string
		globals, rest []string
		wantErr       bool
	}{
		{in: []string{"leads", "submit", "--json"}, globals: []string{"--json"}, rest: []string{"leads", "submit"}},
		{in: []string{"--base-url", "http://x", "health"}, globals: []string{"--base-url", "http://x"}, rest: []string{"health"}},
		{in: []string{"--base-url=http://x", "health"}, globals: []string{"--base-url=http://x"}, rest: []string{"health"}},
		{in: []string{"leads", "submit", "--first-name", "Dana"}, rest: []string{"leads", "submit", "--first-name", "Dana"}},
		{in: []string{"--", "--json"}, rest: []string{"--json"}},
		{in: []string{"--base-url"}, wantErr: true},
	}
	for _, tc := range cases {
		g, r, err := splitGlobals(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Errorf("%v: want error", tc.in)
			}
			continue
		}
		if err != nil {
			t.Errorf("%v: %v", tc.in, err)
			continue
		}
		if fmt.Sprint(g) != fmt.Sprint(tc.globals) || fmt.Sprint(r) != fmt.Sprint(tc.rest) {
			t.Errorf("%v: globals %v rest %v, want globals %v rest %v", tc.in, g, r, tc.globals, tc.rest)
		}
	}
}
