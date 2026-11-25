package cli

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yetone/magpie/internal/agentenv"
	"github.com/yetone/magpie/internal/provider"
	"github.com/yetone/magpie/internal/testenv"
)

// magpie quota --json, its deadline running out while the first reading of
// the subscriptions' allowances was still under way, told every account
// nothing — a signed-in subscription included — as a balance "not
// configured · no balance endpoint known" row (#7742, zleo-ai). What the
// read had landed is told instead, what was last read of the rest, and an
// account read of neither says "not read in time" rather than sending the
// user to set a balance URL a subscription has no use for.
func TestQuotaCutTellsWhatWasRead(t *testing.T) {
	groupsHome(t) // a machine of its own; the Codex home below is the only account
	for _, k := range agentenv.Vars {
		t.Setenv(k, "")
		os.Unsetenv(k)
	}
	bin := t.TempDir()
	for _, name := range []string{"security", "secret-tool", "claude", "codex", "cursor-agent", "devin", "grok", "kiro-cli"} {
		testenv.Program(t, filepath.Join(bin, name), "#!/bin/sh\nexit 1\n")
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	jwt := func(m map[string]any) string {
		b, _ := json.Marshal(m)
		return "h." + base64.RawURLEncoding.EncodeToString(b) + ".s"
	}
	auth, _ := json.Marshal(map[string]any{"auth_mode": "chatgpt", "tokens": map[string]any{
		"id_token":      jwt(map[string]any{"email": "me@example.com"}),
		"access_token":  jwt(map[string]any{"exp": time.Now().Add(time.Hour).Unix()}),
		"refresh_token": "r", "account_id": "acct-1"}})
	if err := os.MkdirAll(filepath.Join(os.Getenv("HOME"), ".codex"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(os.Getenv("HOME"), ".codex", "auth.json"), auth, 0o600); err != nil {
		t.Fatal(err)
	}
	provider.ForgetAccounts()
	t.Cleanup(provider.ForgetAccounts)
	provider.ForgetKeptCardsForTest()
	t.Cleanup(func() { // whatever read the cut run left under way ends here
		for provider.SubscriptionUsageReading() {
			time.Sleep(5 * time.Millisecond)
		}
		provider.ForgetKeptCardsForTest()
	})
	// the vendor holds a first reading past magpie quota's deadline, as the
	// reporter's machine did. Held rather than timed, because on a loaded
	// machine a wall-clock bound says nothing either way.
	var hold atomic.Bool
	held := make(chan struct{})
	var once sync.Once
	release := func() { once.Do(func() { close(held) }) }
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/backend-api/wham/usage" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if hold.Load() {
			<-held
		}
		io.WriteString(w, `{"plan_type":"pro","rate_limit":{"primary_window":{"used_percent":12,"limit_window_seconds":18000}}}`)
	}))
	t.Cleanup(func() { release(); fake.Close() })
	oldBase, oldDeadline := provider.CodexBase, quotaDeadline
	t.Cleanup(func() { provider.CodexBase, quotaDeadline = oldBase, oldDeadline })
	quota := func(t *testing.T) []map[string]any {
		t.Helper()
		out, err := stdoutOf(t, func() error { return quotaCmd([]string{"quota", "codex", "--json"}) })
		if err != nil {
			t.Fatal(err)
		}
		var qs []map[string]any
		if err := json.Unmarshal([]byte(out), &qs); err != nil {
			t.Fatal(err, out)
		}
		return qs
	}
	settle := func(t *testing.T) { // the held read ends before the next step
		t.Helper()
		release()
		for by := time.Now().Add(time.Minute); provider.SubscriptionUsageReading(); time.Sleep(5 * time.Millisecond) {
			if time.Now().After(by) {
				t.Fatal("the read didn't land once the vendor let go")
			}
		}
	}

	// the reporter's bad run: nothing read of the account yet, and the
	// vendor slow — a row that says so, not "not configured"
	provider.CodexBase, quotaDeadline = fake.URL+"/backend-api/codex", 250*time.Millisecond
	hold.Store(true)
	qs := quota(t)
	if len(qs) != 1 || qs[0]["kind"] != "subscription" {
		t.Fatalf("the cut run's rows: %v", qs)
	}
	if e, _ := qs[0]["error"].(string); !strings.Contains(e, "not read in time") || strings.Contains(e, "not configured") {
		t.Fatalf("a subscription cut off mid-read says %q", e)
	}
	settle(t)

	// a good run reads the card; held again past the deadline, that
	// reading is told rather than nothing
	hold.Store(false)
	if qs = quota(t); len(qs) != 1 || qs[0]["kind"] != "subscription" {
		t.Fatalf("the good run's rows: %v", qs)
	}
	if _, told := qs[0]["error"]; told {
		t.Fatalf("a card read whole carries an error: %v", qs[0])
	}
	ws, _ := qs[0]["windows"].([]any)
	if len(ws) == 0 {
		t.Fatalf("no windows: %v", qs[0])
	}
	hold.Store(true)
	qs = quota(t)
	if len(qs) != 1 || qs[0]["kind"] != "subscription" {
		t.Fatalf("the second cut run's rows: %v", qs)
	}
	if _, told := qs[0]["error"]; told {
		t.Fatalf("the card read before was told as an error: %v", qs[0])
	}
	ws, _ = qs[0]["windows"].([]any)
	if len(ws) == 0 || ws[0].(map[string]any)["used"] != 12.0 {
		t.Fatalf("the card read before wasn't told: %v", qs[0])
	}
	settle(t)
}
