package provider

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// cardReadNow says cs has provider's account user's card, as read now: not a
// kept one standing in (AsOf), and with no error.
func cardReadNow(cs []SubscriptionQuota, provider, user string) bool {
	q := cardOf(cs, provider, user)
	return q != nil && q.AsOf == nil && q.Error == ""
}

// quotaOf is the card qs has of provider, nil when none.
func quotaOf(qs []Quota, provider string) *Quota {
	for i := range qs {
		if qs[i].Provider == provider {
			return &qs[i]
		}
	}
	return nil
}

// #7742 (zleo-ai on GitHub): the first reading of the accounts' allowances
// waits for every account at once, and told a caller whose own deadline ran
// out first nothing, the accounts that had answered included — magpie
// quota, on a machine where one account's read outlasts its 15 seconds.
// LandedQuotaReport tells such a caller each card the read had landed, and
// for an account still being read what was last read of it, as of when.
func TestLandedQuotaReportTellsWhatTheReadHadLanded(t *testing.T) {
	signIn(t) // Codex on me@example.com, Copilot on octocat
	rememberLogins(true)
	dialed := dialsOut(t)
	// the cards kept on disk are read once per process: these are this
	// home's, and no test after this one has them
	ForgetKeptCardsForTest()
	c := &subscriptionUsageCache
	t.Cleanup(func() {
		ForgetKeptCardsForTest()
		for {
			c.Lock()
			p := c.pending
			if p == nil {
				c.data, c.at, c.landing = nil, time.Time{}, nil
				c.Unlock()
				return
			}
			c.Unlock()
			<-p
		}
	})
	chatgpt := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/backend-api/wham/usage" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		io.WriteString(w, `{"plan_type":"pro","rate_limit":{"primary_window":{"used_percent":12,"limit_window_seconds":18000}}}`)
	}))
	t.Cleanup(chatgpt.Close)
	// Copilot's second read is held at the vendor until let go. Held rather
	// than timed, because on a loaded machine a wall-clock bound says
	// nothing either way.
	var hold atomic.Bool
	held := make(chan struct{})
	var once sync.Once
	release := func() { once.Do(func() { close(held) }) }
	copilot := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hold.Load() {
			<-held
		}
		io.WriteString(w, `{"copilot_plan":"individual","token_based_billing":true,"quota_snapshots":`+
			`{"premium_interactions":{"has_quota":true,"unlimited":false,"entitlement":"300","percent_remaining":50}}}`)
	}))
	t.Cleanup(func() { release(); copilot.Close() })
	oldBase, oldUser := CodexBase, CopilotUserURL
	CodexBase, CopilotUserURL = chatgpt.URL+"/backend-api/codex", copilot.URL
	t.Cleanup(func() { CodexBase, CopilotUserURL = oldBase, oldUser })

	// a reading of both, kept on disk as every good reading is
	good := SubscriptionUsage(context.Background())
	codexGood, copilotGood := cardOf(good, "codex", "me@example.com"), cardOf(good, "copilot", "octocat")
	if codexGood == nil || len(codexGood.Windows) != 1 || codexGood.Windows[0].Used != 12 || codexGood.Error != "" ||
		copilotGood == nil || len(copilotGood.Windows) == 0 || copilotGood.Error != "" {
		t.Fatalf("the whole reading: %+v", good)
	}

	// as magpie quota starts, a process of its own: nothing read yet, and
	// the vendor of one account slow
	loginUsageCache.Lock()
	loginUsageCache.m = nil
	loginUsageCache.Unlock()
	c.Lock()
	c.data, c.at, c.asked, c.landing = nil, time.Time{}, false, nil
	c.Unlock()
	hold.Store(true)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	first := make(chan []SubscriptionQuota, 1)
	go func() { first <- SubscriptionUsage(ctx) }()
	// the deadline runs out once Codex's card has landed, Copilot's read
	// waiting at the vendor
	by := time.Now().Add(time.Minute)
	var cut []Quota
	for {
		cut = LandedQuotaReport(time.Now())
		if q := quotaOf(cut, "codex"); q != nil && q.AsOf == nil && len(q.Windows) > 0 && q.Windows[0].Used == 12 {
			break
		}
		if time.Now().After(by) {
			t.Fatalf("Codex's card didn't land while Copilot's was held: %+v", cut)
		}
		time.Sleep(5 * time.Millisecond)
	}
	cancel()
	if got := <-first; got != nil {
		t.Fatalf("SubscriptionUsage told a caller that stopped waiting %d cards; the first reading comes all at once", len(got))
	}
	if len(cut) != 2 {
		t.Fatalf("the cards as far as the read had landed: %+v", cut)
	}
	codexCard, copilotCard := quotaOf(cut, "codex"), quotaOf(cut, "copilot")
	if codexCard == nil || codexCard.Kind != "subscription" || codexCard.ReadAt == nil || codexCard.Error != "" || len(codexCard.Windows) == 0 || codexCard.Windows[0].Used != 12 {
		t.Errorf("the card the read had landed isn't told as read now: %+v", cut)
	}
	if copilotCard == nil || copilotCard.Kind != "subscription" || copilotCard.AsOf == nil || copilotCard.Error != "" ||
		len(copilotCard.Windows) == 0 || copilotCard.Windows[0].Used != copilotGood.Windows[0].Used {
		t.Errorf("the account still being read isn't told as last read: %+v", cut)
	}

	// the read lands: every card as read now, and the same as kept
	release()
	for by = time.Now().Add(time.Minute); SubscriptionUsageReading(); time.Sleep(5 * time.Millisecond) {
		if time.Now().After(by) {
			t.Fatal("the read didn't land once the vendor let go")
		}
	}
	if final := SubscriptionUsage(context.Background()); len(final) != 2 || !cardReadNow(final, "codex", "me@example.com") || !cardReadNow(final, "copilot", "octocat") {
		t.Errorf("the reading that landed: %+v", final)
	}
	kept := KeptQuotaReport(time.Now())
	if landed := LandedQuotaReport(time.Now()); len(landed) != 2 {
		t.Errorf("no read under way: %+v", landed)
	} else {
		for _, q := range landed {
			k := quotaOf(kept, q.Provider)
			if q.AsOf != nil || q.Kind != "subscription" || k == nil || k.AsOf != nil || k.Error != "" || len(k.Windows) != len(q.Windows) {
				t.Errorf("%s: with no read under way, LandedQuotaReport %+v and KeptQuotaReport %+v", q.Provider, q, k)
			}
		}
	}
	if out := dialed(); len(out) > 0 {
		t.Fatalf("asked %v, off this machine", out)
	}
}

// SignedInProvider is asked by magpie quota: a subscription with an
// account to read is one, and a key's plan or balance isn't.
func TestSignedInProviderIsASubscriptionWithAnAccount(t *testing.T) {
	signIn(t)
	rememberLogins(true)
	for id, want := range map[string]bool{"codex": true, "copilot": true, "claude": false, "gemini": false, "cursor": false, "zcode": false, "nosuch": false} {
		if got := SignedInProvider(id); got != want {
			t.Errorf("SignedInProvider(%q) = %v, want %v", id, got, want)
		}
	}
	if err := Save(Provider{ID: "a", Name: "A", Key: "ka", Models: []string{"m"}, Chat: "http://127.0.0.1:1/v1"}); err != nil {
		t.Fatal(err)
	}
	if SignedInProvider("a") {
		t.Error("a key's provider is signed in")
	}
}
