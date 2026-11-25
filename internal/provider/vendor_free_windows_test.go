package provider

import (
	"testing"
	"time"

	"github.com/yetone/magpie/internal/catalog"
	"github.com/yetone/magpie/internal/plugin"
)

// Cline, Kilo and OpenRouter each have a vendor marker for a model served
// outside the paid window. Their whole account/key window must count the
// paid model and leave the marked model alone; an ordinary $0 model is not
// a vendor free marker.
func TestVendorFreeWindows(t *testing.T) {
	now := time.Now()
	at := now.Add(time.Hour)
	for _, c := range []struct {
		name       string
		p          Provider
		live       []catalog.Model
		paid, free string
		zero       string
	}{
		{
			name: "Cline feed",
			p: Provider{ID: "cline-free", Name: "Cline", Preset: "clinepass",
				Chat: "https://api.cline.bot/api/v1"},
			live: []catalog.Model{
				{ID: "deepseek/deepseek-v4-pro"},
				{ID: "cline-free/deepseek-v4.1-flash", Free: true},
			},
			paid: "deepseek/deepseek-v4-pro",
			free: "cline-free/deepseek-v4.1-flash",
		},
		{
			name: "Cline fallback id",
			p: Provider{ID: "cline-fallback", Name: "Cline", Preset: "clinepass",
				Chat: "https://api.cline.bot/api/v1"},
			live: []catalog.Model{
				{ID: "deepseek/deepseek-v4-pro"},
				{ID: "cline-free/mimo-v2.6-flash"},
			},
			paid: "deepseek/deepseek-v4-pro",
			free: "cline-free/mimo-v2.6-flash",
		},
		{
			name: "Kilo isFree",
			p: Provider{ID: "kilo-free", Name: "Kilo", Preset: "kilo",
				Chat: "https://api.kilo.ai/api/openrouter"},
			live: []catalog.Model{
				{ID: "anthropic/claude-sonnet-5"},
				{ID: "qwen/qwen3.8-27b:free", Free: true},
			},
			paid: "anthropic/claude-sonnet-5",
			free: "qwen/qwen3.8-27b:free",
		},
		{
			name: "Kilo id marker",
			p: Provider{ID: "kilo-id", Name: "Kilo", Preset: "kilo",
				Chat: "https://api.kilo.ai/api/openrouter"},
			live: []catalog.Model{
				{ID: "anthropic/claude-sonnet-5"},
				{ID: "kilo-auto/free"},
			},
			paid: "anthropic/claude-sonnet-5",
			free: "kilo-auto/free",
		},
		{
			name: "OpenRouter suffix",
			p: Provider{ID: "openrouter-free", Name: "OpenRouter", Preset: "openrouter",
				Chat: "https://openrouter.ai/api/v1"},
			live: []catalog.Model{
				{ID: "anthropic/claude-sonnet-5"},
				{ID: "qwen/qwen3.8-27b:free"},
				{ID: "vendor/zero", Price: &catalog.Price{}},
			},
			paid: "anthropic/claude-sonnet-5",
			free: "qwen/qwen3.8-27b:free",
			zero: "vendor/zero",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			keyLimitsHome(t)
			if err := catalog.SaveLive(c.p.ID, c.p.Chat, c.live); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(catalog.Reset)
			a := allowanceOf(c.p.paidOnly([]QuotaWindow{{
				Name: "window", Used: 100, ResetsAt: &at, Span: time.Hour,
			}}), now)
			if used, _ := a.For(c.paid, now); used != 100 {
				t.Errorf("paid model used %v%%, want 100", used)
			}
			if used, _ := a.For(c.free, now); used != 0 {
				t.Errorf("free model used %v%%, want 0", used)
			}
			if c.zero != "" {
				if c.p.costsNothing(catalog.Model{ID: c.zero, Price: &catalog.Price{}}) {
					t.Errorf("ordinary $0 model %q was taken as vendor-free", c.zero)
				}
				if c.p.FreeBesidePaid(c.zero) {
					t.Errorf("ordinary $0 model %q was taken as a free refusal", c.zero)
				}
				if used, _ := a.For(c.zero, now); used != 100 {
					t.Errorf("ordinary $0 model used %v%%, want 100", used)
				}
			}
			if !c.p.PaidBesideFree(c.paid) {
				t.Errorf("paid model beside free is not marked: %s", c.paid)
			}
			if !c.p.FreeBesidePaid(c.free) {
				t.Errorf("free model beside paid is not marked: %s", c.free)
			}
		})
	}
}

// A Cline/Kilo plugin provider has no Cline/Kilo host to identify it. Its
// plugin id is enough for the fallback free-model markers.
func TestPluginVendorFreeMarkers(t *testing.T) {
	cline := Provider{ID: "cline-plugin", Account: &Account{plugin: &plugin.Provider{ID: "cline"}}}
	if !cline.clineWindowed() || !cline.freeByVendorID("cline-free/x") {
		t.Error("Cline plugin free marker not recognized")
	}
	kilo := Provider{ID: "kilo-plugin", Account: &Account{plugin: &plugin.Provider{ID: "kilo"}}}
	if !kilo.kiloWindowed() || !kilo.freeByVendorID("qwen/x:free") {
		t.Error("Kilo plugin free marker not recognized")
	}
}
