package agent

import (
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/yetone/magpie/internal/catalog"
	"github.com/yetone/magpie/internal/provider"
)

// dshGroupRouteHome is dshGeminiHome with the dsh CLI saying cli, and
// providers and groups whose members are each best on Messages, on
// Gemini's API, or on both:
//   - relay and relay2 serve Messages alone, with two Gemini 4s as agy-cli's
//     plugin serves them, and Claudes that ask adaptively or don't;
//   - gem and gem2 serve Gemini's API alone;
//   - hyb and hyb2 serve Messages and Gemini's, a Gemini 3 there on both.
func dshGroupRouteHome(t *testing.T, cli string) (home, web string) {
	t.Helper()
	home, _, web = dshGeminiHome(t, cli, "")
	for _, p := range []provider.Provider{
		{ID: "relay", Name: "Relay", Key: "k", Anthropic: "http://127.0.0.1:1", Models: []string{"gemini-4-hydrogen", "claude-sonnet-4-6", "claude-sonnet-4-5"}},
		{ID: "relay2", Name: "Relay 2", Key: "k", Anthropic: "http://127.0.0.1:2", Models: []string{"gemini-4-hydrogen", "claude-sonnet-4-5"}},
		{ID: "gem2", Name: "Gemini 2", Key: "k", Gemini: "http://127.0.0.1:2/v1beta", Models: []string{"gemini-3-flash"}},
		{ID: "hyb", Name: "Hyb", Key: "k", Anthropic: "http://127.0.0.1:3", Gemini: "http://127.0.0.1:3/v1beta", Models: []string{"gemini-3-pro"}},
		{ID: "hyb2", Name: "Hyb 2", Key: "k", Anthropic: "http://127.0.0.1:4", Gemini: "http://127.0.0.1:4/v1beta", Models: []string{"gemini-3-pro"}},
	} {
		if err := provider.Save(p); err != nil {
			t.Fatal(err)
		}
	}
	for _, g := range []provider.Group{
		{ID: "hydrogens", Name: "Hydrogens", Members: []string{"relay/gemini-4-hydrogen", "relay2/gemini-4-hydrogen"}},
		{ID: "sonnets", Name: "Sonnets", Members: []string{"relay/claude-sonnet-4-5", "relay2/claude-sonnet-4-5"}},
		// one asked adaptively, one on a budget: pi-ai asks a group one way
		{ID: "split", Name: "Split", Members: []string{"relay/claude-sonnet-4-6", "relay2/claude-sonnet-4-5"}},
		{ID: "flashes", Name: "Flashes", Members: []string{"gem/gemini-3-flash", "gem2/gemini-3-flash"}},
		{ID: "pros", Name: "Pros", Members: []string{"hyb/gemini-3-pro", "hyb2/gemini-3-pro"}},
		// Gemini's API is all the two share
		{ID: "googles", Name: "Googles", Members: []string{"gem/gemini-3-flash", "hyb/gemini-3-pro"}},
		// a member Messages doesn't reach
		{ID: "mixed", Name: "Mixed", Members: []string{"relay/gemini-4-hydrogen", "deepseek/pro"}},
	} {
		if err := provider.SaveGroup(g); err != nil {
			t.Fatal(err)
		}
	}
	if err := dsh(home).Field("model").Set("magpie/group/hydrogens"); err != nil {
		t.Fatal(err)
	}
	return home, web
}

// dshRouteEntry is the route of magpie's id is on in the patch list web,
// and its entry there.
func dshRouteEntry(t *testing.T, web, id string) (string, dshPiModel) {
	t.Helper()
	for key, r := range dshPiProviders(t, web) {
		for _, m := range r.Models {
			if m.ID == id {
				return key, m
			}
		}
	}
	return "", dshPiModel{}
}

// Discord (01huadalang): 为什么 gemini cli 在 dsh 是 ant 的协议，然后按你说的账号
// 分模型用路由组处理就变成 chat 协议了，路由组不能根据里面的模型协议自动匹配吗.
// A routing group had an API only when its members were on Responses
// (provider.GroupAPI), so a group of agy-cli Gemini models, each on dsh's
// Messages route alone, went on Chat and was translated for every member.
// In dsh a group now goes on the route of the API every member is best on
// and relays as it is, asked adaptively there when every member is; a
// mix stays on Chat, and Gemini's route falls back as a lone Gemini's
// does where dsh can't speak it.
func TestDshGroupOnItsMembersRoute(t *testing.T) {
	adaptive := &dshPiCompat{ForceAdaptiveThinking: true}
	for _, c := range []struct {
		cli  string
		want map[string]string
	}{
		{"0.2.2-rc.5", map[string]string{
			"group/hydrogens": "magpie-anthropic",
			"group/sonnets":   "magpie-anthropic",
			"group/split":     "magpie",
			"group/flashes":   "magpie-gemini",
			"group/pros":      "magpie-gemini",
			"group/googles":   "magpie-gemini",
			"group/mixed":     "magpie",
		}},
		// a dsh that can't take Gemini's API: Messages where every member
		// relays it, Chat where one doesn't
		{"0.2.1", map[string]string{
			"group/hydrogens": "magpie-anthropic",
			"group/flashes":   "magpie",
			"group/pros":      "magpie-anthropic",
			"group/googles":   "magpie",
		}},
	} {
		t.Run(c.cli, func(t *testing.T) {
			_, web := dshGroupRouteHome(t, c.cli)
			for id, want := range c.want {
				route, e := dshRouteEntry(t, web, id)
				if route != want {
					t.Errorf("%s on %q, want %q", id, route, want)
				}
				if route != "magpie-anthropic" {
					continue
				}
				// on Messages, asked adaptively as its members each are
				// (#6778: a budget clamps xhigh and max to high), and a
				// group of Claudes on a budget left on it
				switch id {
				case "group/sonnets":
					if e.Compat != nil {
						t.Errorf("%s compat %+v", id, e.Compat)
					}
				default:
					if e.Compat == nil || *e.Compat != *adaptive {
						t.Errorf("%s compat %+v, want forceAdaptiveThinking", id, e.Compat)
					}
				}
			}
			// the start names the group's route
			if b, _ := os.ReadFile(web); !strings.Contains(string(b), "    provider: magpie-anthropic\n") {
				t.Errorf("start not on the group's route:\n%s", b)
			}
		})
	}
}

// A group every member of which is best on Messages or Gemini's now goes
// on that route in every agent's list, not only dsh's. An agent that can't
// speak Gemini's API keeps the common Messages route; one that can speak
// neither stays on Chat. A member alone keeps its own APIs.
func TestGroupAPIsReachEveryAgentsList(t *testing.T) {
	dshGroupRouteHome(t, "0.2.2-rc.5")
	for _, agent := range []string{"codex", "claude", "cursor"} {
		if groupRouteAgent(agent) {
			t.Errorf("%s took the per-API group route", agent)
		}
	}
	for _, list := range []string{"pi", "omp", "droid", "hermes", "grok", "liveagent", "dsh", dshBetaID, dsbotID} {
		var found bool
		for _, m := range magpieModels(list) {
			switch m.ID {
			case "group/hydrogens":
				found = true
				if !slices.Equal(m.APIs, []string{"anthropic"}) || !m.AsksAdaptively {
					t.Errorf("%s: %s APIs %v adaptive %v", list, m.ID, m.APIs, m.AsksAdaptively)
				}
			case "group/flashes":
				if !slices.Equal(m.APIs, []string{"gemini"}) || m.AsksAdaptively {
					t.Errorf("%s: %s APIs %v adaptive %v", list, m.ID, m.APIs, m.AsksAdaptively)
				}
			case "group/split", "group/mixed":
				if m.APIs != nil || m.AsksAdaptively {
					t.Errorf("%s: %s APIs %v adaptive %v", list, m.ID, m.APIs, m.AsksAdaptively)
				}
			case "relay/gemini-4-hydrogen":
				if !slices.Equal(m.APIs, []string{"anthropic"}) {
					t.Errorf("%s: %s APIs %v", list, m.ID, m.APIs)
				}
			}
		}
		if !found {
			t.Errorf("%s: no group/hydrogens", list)
		}
	}

	hydrogens := catalogModel(t, "group/hydrogens")
	if e := piModelJSON(hydrogens, "http://127.0.0.1:3425", true); e["api"] != "anthropic-messages" || e["compat"] == nil {
		t.Errorf("Pi group/hydrogens: %+v", e)
	}
	if e := piModelOn(hydrogens, "http://127.0.0.1:3425", true, "anthropic-messages"); e["compat"] == nil {
		t.Errorf("Pi protocol row group/hydrogens: %+v", e)
	}
	if e := ompModelFor(t, "group/hydrogens"); e.API != "anthropic-messages" {
		t.Errorf("omp group/hydrogens: %+v", e)
	}
	protocol := ompOn(ompProviderEntry{Models: []ompModel{{
		ID: "group/hydrogens", AsksAdaptively: true, Thinking: &ompThinking{Mode: "budget", Efforts: []string{"low", "high"}},
	}}}, "anthropic-messages")
	if got := protocol.Models[0].Thinking; got == nil || got.Mode != "anthropic-adaptive" {
		t.Errorf("omp protocol row group/hydrogens: %+v", got)
	}
	if e := droidEntryFor(t, "group/hydrogens"); e.Provider != "anthropic" {
		t.Errorf("droid group/hydrogens provider %q", e.Provider)
	}
	if !hermesOnMessages(hydrogens) {
		t.Error("Hermes group/hydrogens is not on Messages")
	}
	if got := grokBackend(hydrogens); got != "messages" {
		t.Errorf("Grok group/hydrogens backend %q", got)
	}
	if got := liveAgentKindOf(hydrogens).key; got != "magpie-anthropic" {
		t.Errorf("LiveAgent group/hydrogens key %q", got)
	}

	flashes := catalogModel(t, "group/flashes")
	if e := piModelJSON(flashes, "http://127.0.0.1:3425", true); e["api"] != "google-generative-ai" {
		t.Errorf("Pi group/flashes: %+v", e)
	}
	if e := ompModelFor(t, "group/flashes"); e.API != "google-generative-ai" {
		t.Errorf("omp group/flashes: %+v", e)
	}
	if e := droidEntryFor(t, "group/flashes"); e.Provider != "generic-chat-completion-api" {
		t.Errorf("droid group/flashes provider %q", e.Provider)
	}
	if hermesOnMessages(flashes) {
		t.Error("Hermes group/flashes is on Messages")
	}
	if got := grokBackend(flashes); got != "chat_completions" {
		t.Errorf("Grok group/flashes backend %q", got)
	}
	if got := liveAgentKindOf(flashes).key; got != "magpie" {
		t.Errorf("LiveAgent group/flashes key %q", got)
	}

	pros := catalogModel(t, "group/pros")
	if e := droidEntryFor(t, "group/pros"); e.Provider != "anthropic" {
		t.Errorf("droid group/pros provider %q", e.Provider)
	}
	if !hermesOnMessages(pros) {
		t.Error("Hermes group/pros is not on Messages")
	}
	if got := grokBackend(pros); got != "messages" {
		t.Errorf("Grok group/pros backend %q", got)
	}
	if got := liveAgentKindOf(pros).key; got != "magpie-anthropic" {
		t.Errorf("LiveAgent group/pros key %q", got)
	}
}

func catalogModel(t *testing.T, id string) catalog.Model {
	t.Helper()
	for _, m := range magpieModels("pi") {
		if m.ID == id {
			return m
		}
	}
	t.Fatalf("no catalog model %s", id)
	return catalog.Model{}
}

func ompModelFor(t *testing.T, id string) ompModel {
	t.Helper()
	for _, m := range ompProviderAt("http://127.0.0.1:3425", "18.14.0").Models {
		if m.ID == id {
			return m
		}
	}
	t.Fatalf("no omp model %s", id)
	return ompModel{}
}

func droidEntryFor(t *testing.T, id string) droidEntry {
	t.Helper()
	for _, e := range droidEntriesAt("http://127.0.0.1:3425") {
		if e.Model == id {
			return e
		}
	}
	t.Fatalf("no droid model %s", id)
	return droidEntry{}
}
