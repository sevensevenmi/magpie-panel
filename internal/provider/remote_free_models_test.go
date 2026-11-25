package provider

import (
	"testing"
	"time"

	"github.com/yetone/magpie/internal/catalog"
)

// A remote card tells the receiving magpie which vendor-marked models are
// free. The receiver scopes whole-key/account windows to the paid models and
// leaves model-specific windows alone. A card from an older sender has no
// FreeModels and keeps the old whole-window behavior.
func TestRemoteCardsCarryFreeModelScope(t *testing.T) {
	now := time.Now()
	at := now.Add(time.Hour)
	q := SubscriptionQuota{
		Provider: "opencode-go",
		Windows: []QuotaWindow{
			{Name: "5 hours", Used: 100, ResetsAt: &at, Span: 5 * time.Hour},
			{Name: "paid only", Used: 100, Model: "glm-5.3", ResetsAt: &at},
		},
		FreeModels: []string{"hy4-flash-free"},
	}
	got := withRouting(ToldRouting([]SubscriptionQuota{q}))
	if len(got) != 1 || len(got[0].FreeModels) != 1 || got[0].FreeModels[0] != "hy4-flash-free" {
		t.Fatalf("FreeModels not told: %+v", got)
	}
	a := allowanceOf(got[0].Windows, now)
	if used, _ := a.For("glm-5.3", now); used != 100 {
		t.Errorf("paid model used %v%%, want 100", used)
	}
	if used, _ := a.For("HY4-FLASH-FREE", now); used != 0 {
		t.Errorf("free model used %v%%, want 0", used)
	}

	old := withRouting(ToldRouting([]SubscriptionQuota{{
		Provider: "opencode-go",
		Windows: []QuotaWindow{
			{Name: "5 hours", Used: 100, ResetsAt: &at, Span: 5 * time.Hour},
		},
	}}))
	oldAllowance := allowanceOf(old[0].Windows, now)
	if used, _ := oldAllowance.For("hy4-flash-free", now); used != 100 {
		t.Errorf("old card free model used %v%%, want old whole-window behavior 100", used)
	}
}

// The magpie that owns a card fills FreeModels before another magpie reads
// it. The remote's provider list, not a zero price, is the source.
func TestCardsOfAddsFreeModels(t *testing.T) {
	keyLimitsHome(t)
	p := Provider{ID: "or-card", Name: "OpenRouter", Preset: "openrouter",
		Key: "sk-or-card", Chat: "https://openrouter.ai/api/v1"}
	if err := catalog.SaveLive(p.ID, p.Chat, []catalog.Model{
		{ID: "paid/model"},
		{ID: "free/model:free"},
		{ID: "zero/model", Price: &catalog.Price{}},
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(catalog.Reset)
	if err := Save(p); err != nil {
		t.Fatal(err)
	}
	cards := cardsOf([]SubscriptionQuota{{
		Provider: p.ID,
		Name:     p.Name,
		Windows:  []QuotaWindow{{Name: "day", Used: 100}},
	}}, time.Now())
	if len(cards) != 1 {
		t.Fatalf("cards: %+v", cards)
	}
	if len(cards[0].FreeModels) != 1 || cards[0].FreeModels[0] != "free/model:free" {
		t.Fatalf("free models: %v, want only free/model:free", cards[0].FreeModels)
	}
}
