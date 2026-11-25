package gui

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/yetone/magpie/internal/provider"
	"github.com/yetone/magpie/internal/settings"
)

// A provider that exists is not enough: a Settings pick must name one of its
// models. Saved values survive when their provider or model disappears, so a
// later save of the rest of Settings does not force a replacement.
func TestSettingsModelsMustBeServed(t *testing.T) {
	sandboxHome(t)
	for _, p := range []provider.Provider{
		{ID: "fake", Name: "Fake", Key: "k", Models: []string{"m1", "gpt-image-1"}, Chat: "http://127.0.0.1:9/v1"},
		{ID: "hidden", Name: "Hidden", Key: "k", Unlisted: true, Models: []string{"m1", "gpt-image-1"}, Chat: "http://127.0.0.1:9/v1"},
		{ID: "drawhidden", Name: "Hidden drawer", Key: "k", Models: []string{"gpt-image-1"}, HiddenDrawers: []string{"gpt-image-1"}, Chat: "http://127.0.0.1:9/v1"},
	} {
		if err := provider.Save(p); err != nil {
			t.Fatal(err)
		}
	}
	if err := provider.SaveGroup(provider.Group{ID: "helpers", Name: "Helpers", Members: []string{"fake/m1"}}); err != nil {
		t.Fatal(err)
	}
	if err := provider.SaveGroup(provider.Group{ID: "drawers", Name: "Drawers", Members: []string{"fake/gpt-image-1"}}); err != nil {
		t.Fatal(err)
	}

	picker := map[string]bool{}
	for _, m := range settingsState().ImageGenModels {
		picker[m.ID] = true
	}
	for _, id := range []string{"fake/gpt-image-1", "hidden/gpt-image-1", "group/drawers"} {
		if !picker[id] {
			t.Errorf("Image generation picker has no %s: %v", id, picker)
		}
	}
	for _, id := range []string{"fake/m1", "group/helpers", "drawhidden/gpt-image-1"} {
		if picker[id] {
			t.Errorf("Image generation picker offers %s, which is not a visible drawer: %v", id, picker)
		}
	}
	// A hidden drawer that is already saved stays visible in the picker,
	// even though a new pick of it is refused above.
	st := settings.Load()
	st.ImageGen = "drawhidden/gpt-image-1"
	if err := settings.Save(st); err != nil {
		t.Fatal(err)
	}
	picker = map[string]bool{}
	for _, m := range settingsState().ImageGenModels {
		picker[m.ID] = true
	}
	if !picker["drawhidden/gpt-image-1"] {
		t.Errorf("Image generation picker dropped the saved hidden drawer: %v", picker)
	}
	st.ImageGen = ""
	if err := settings.Save(st); err != nil {
		t.Fatal(err)
	}

	h := Handler(nil, nil)
	for _, tc := range []struct {
		name, path, key string
		field           func(*settings.Settings) *string
	}{
		{"vision", "/api/settings", "vision", func(s *settings.Settings) *string { return &s.Vision }},
		{"imageGen", "/api/settings", "imageGen", func(s *settings.Settings) *string { return &s.ImageGen }},
		{"titles", "/api/settings/codex-titles", "model", func(s *settings.Settings) *string { return &s.CodexTitles }},
		{"descriptions", "/api/settings/codex-descriptions", "model", func(s *settings.Settings) *string { return &s.CodexDescriptions }},
		{"predictions", "/api/settings/codex-predictions", "model", func(s *settings.Settings) *string { return &s.CodexPredictions }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			call := func(model string) *httptest.ResponseRecorder {
				t.Helper()
				body, err := json.Marshal(map[string]string{tc.key: model})
				if err != nil {
					t.Fatal(err)
				}
				r := httptest.NewRecorder()
				h.ServeHTTP(r, httptest.NewRequest("POST", tc.path, bytes.NewReader(body)))
				return r
			}
			valid := []string{"hidden/m1", "group/helpers", "fake/m1"}
			bad := []string{"fake/missing", "fake/", "missing/m1", "group/missing", "m1"}
			if tc.name == "imageGen" {
				valid = []string{"fake/gpt-image-1", "hidden/gpt-image-1", "group/drawers"}
				bad = append(bad, "fake/m1", "group/helpers", "drawhidden/gpt-image-1")
			}
			for _, model := range valid {
				r := call(model)
				s := settings.Load()
				if r.Code != http.StatusOK || *tc.field(&s) != model {
					t.Fatalf("valid pick %q: status %d, saved %q", model, r.Code, *tc.field(&s))
				}
			}
			kept := settings.Load()
			for _, model := range bad {
				if err := settings.Save(kept); err != nil {
					t.Fatal(err)
				}
				before, err := os.ReadFile(settings.Path())
				if err != nil {
					t.Fatal(err)
				}
				r := call(model)
				if r.Code != http.StatusBadRequest {
					t.Errorf("unavailable pick %q: status %d, want 400", model, r.Code)
				}
				after, err := os.ReadFile(settings.Path())
				if err != nil || !bytes.Equal(after, before) {
					t.Errorf("rejecting %q changed the saved settings: %v", model, err)
				}
			}
			// A pick whose provider or model disappeared is kept when the
			// page re-sends it as part of a later save.
			s := settings.Load()
			*tc.field(&s) = "gone/old-model"
			if err := settings.Save(s); err != nil {
				t.Fatal(err)
			}
			r := call("gone/old-model")
			s = settings.Load()
			if r.Code != http.StatusOK || *tc.field(&s) != "gone/old-model" {
				t.Errorf("saved unavailable pick: status %d, saved %q", r.Code, *tc.field(&s))
			}
		})
	}
}
