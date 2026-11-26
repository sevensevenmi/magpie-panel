package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/yetone/magpie/internal/provider"
)

func dshDesktopTestHome(t *testing.T) (home, cfg string) {
	t.Helper()
	home = t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("DSH_HOME", "")
	t.Setenv("DSH_DESKTOP_NEXT_HOME", "")
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, ".local", "share"))
	t.Setenv("APPDATA", filepath.Join(home, "AppData", "Roaming"))
	t.Setenv("LOCALAPPDATA", filepath.Join(home, "AppData", "Local"))
	t.Setenv("PATH", t.TempDir())
	switch runtime.GOOS {
	case "windows":
		cfg = filepath.Join(home, "AppData", "Roaming")
	case "darwin":
		cfg = filepath.Join(home, "Library", "Application Support")
	default:
		cfg = filepath.Join(home, ".config")
	}
	if err := provider.Save(provider.Provider{ID: "deepseek", Name: "DeepSeek", Chat: "https://api.deepseek.com/v1", Key: "k", Models: []string{"pro", "flash"}}); err != nil {
		t.Fatal(err)
	}
	return home, cfg
}

func writeDshJSON(t *testing.T, path, body string) {
	t.Helper()
	writeTestFile(t, path, body, 0o600)
}

// DSH NEXT's fixed locator wins over its environment and the default. The
// active profile is the one Path, Joined, Check and the serving round use.
func TestDshNextHomeProfileAndRound(t *testing.T) {
	home, cfg := dshDesktopTestHome(t)
	chosen := filepath.Join(home, "chosen")
	environment := filepath.Join(home, "environment")
	stale := filepath.Join(home, "last-home")
	t.Setenv("DSH_DESKTOP_NEXT_HOME", environment)
	writeDshJSON(t, filepath.Join(cfg, "DSH NEXT", "home", dshDesktopLocationFile), `{"home":`+dshJSONString(chosen)+`}`)
	writeDshJSON(t, filepath.Join(cfg, "DSH NEXT", "home", "desktop-next-last-home.json"), `{"home":`+dshJSONString(stale)+`}`)
	work := dshProfileFile(chosen, "work")
	web := dshProfileFile(chosen, "web")
	writeTestFile(t, web, dshProfileTemplate, 0o600)
	writeTestFile(t, work, dshProfileTemplate, 0o600)
	writeDshJSON(t, filepath.Join(chosen, dshDesktopProfileState), `{"version":1,"active":"work"}`)
	a := dsh(home)
	if err := a.Pick("model", "magpie/deepseek/pro"); err != nil {
		t.Fatal(err)
	}
	// The active profile is the one dsh's own editor has just made empty;
	// the serving round fills it from the other wired profile.
	writeTestFile(t, work, dshProfileTemplate, 0o600)

	if trouble := dshWiredOnce(); trouble != "" {
		t.Fatal(trouble)
	}
	if b, err := os.ReadFile(work); err != nil || !strings.Contains(string(b), "provider: "+dshRoute) || !strings.Contains(string(b), "apiKeyEnv: "+dshKeyRef) {
		t.Fatalf("the serving round did not wire the active profile: %v\n%s", err, b)
	}
	a = dsh(home)
	if a.Path != work {
		t.Fatalf("Path = %s, want %s", a.Path, work)
	}
	if !a.Joined() || !a.Wired() {
		t.Fatalf("active profile not joined/wired: joined=%v wired=%v", a.Joined(), a.Wired())
	}
	if got := a.Field("model").Get(); got != "magpie/deepseek/pro" {
		t.Fatalf("model = %q", got)
	}
	if d := a.Check(); d != "" {
		t.Fatalf("check: %s", d)
	}
	for _, unused := range []string{environment, stale} {
		if _, err := os.Stat(unused); !os.IsNotExist(err) {
			t.Fatalf("guessed a home from %s: %v", unused, err)
		}
	}
}

// A locator that exists but names a relative home, or an active profile
// that isn't valid, is unknown: the fallback is not wired and nothing is
// created there.
func TestDshDesktopHomesFailClosed(t *testing.T) {
	t.Run("relative locator", func(t *testing.T) {
		home, cfg := dshDesktopTestHome(t)
		fallback := filepath.Join(home, "fallback")
		t.Setenv("DSH_DESKTOP_NEXT_HOME", fallback)
		writeDshJSON(t, filepath.Join(cfg, "DSH NEXT", "home", dshDesktopLocationFile), `{"home":"relative/home"}`)
		legacy := dshProfileFile(filepath.Join(home, ".dsh"), "web")
		writeTestFile(t, legacy, dshProfileTemplate, 0o600)
		before, _ := os.ReadFile(legacy)

		if dir := dshHome(); dir != "" {
			t.Fatalf("dshHome guessed %q", dir)
		}
		a := dsh(home)
		if a.Path != "" || len(a.Fields) != 0 {
			t.Fatalf("unavailable row has path %q and %d fields", a.Path, len(a.Fields))
		}
		if trouble := dshWiredOnce(); trouble != "" {
			t.Fatal(trouble)
		}
		if after, _ := os.ReadFile(legacy); string(after) != string(before) {
			t.Fatalf("a fallback profile was written:\n%s", after)
		}
		if _, err := os.Stat(fallback); !os.IsNotExist(err) {
			t.Fatalf("the environment fallback was created: %v", err)
		}
	})

	t.Run("active profile", func(t *testing.T) {
		home, cfg := dshDesktopTestHome(t)
		chosen := filepath.Join(home, "chosen")
		writeDshJSON(t, filepath.Join(cfg, "DSH NEXT", "home", dshDesktopLocationFile), `{"home":`+dshJSONString(chosen)+`}`)
		writeTestFile(t, dshProfileFile(chosen, "web"), dshProfileTemplate, 0o600)
		writeDshJSON(t, filepath.Join(chosen, dshDesktopProfileState), `{"version":1,"active":"../outside"}`)

		a := dsh(home)
		if a.Path != "" || len(a.Fields) != 0 {
			t.Fatalf("unavailable row has path %q and %d fields", a.Path, len(a.Fields))
		}
		if trouble := dshWiredOnce(); trouble != "" {
			t.Fatal(trouble)
		}
		if b, _ := os.ReadFile(dshProfileFile(chosen, "web")); string(b) != dshProfileTemplate {
			t.Fatalf("a profile was written despite the invalid active profile:\n%s", b)
		}
	})

	t.Run("relative environment", func(t *testing.T) {
		home, _ := dshDesktopTestHome(t)
		t.Setenv("DSH_DESKTOP_NEXT_HOME", "relative/home")
		if dir := dshHome(); dir != "" {
			t.Fatalf("dshHome guessed %q", dir)
		}
		a := dsh(home)
		if a.Path != "" || len(a.Fields) != 0 {
			t.Fatalf("unavailable row has path %q and %d fields", a.Path, len(a.Fields))
		}
		if _, err := os.Stat(filepath.Join(home, ".dsh")); !os.IsNotExist(err) {
			t.Fatalf("the default home was made: %v", err)
		}
	})
}

// DSH Desktop Beta is its own row and home. Its active profile is wired,
// shown and restored, while stable dsh stays on ~/.dsh.
func TestDshBetaHomeProfileAndRound(t *testing.T) {
	home, cfg := dshDesktopTestHome(t)
	betaHome := filepath.Join(home, "beta-home")
	writeDshJSON(t, filepath.Join(cfg, "DSH Desktop Beta", "home", dshDesktopLocationFile), `{"home":`+dshJSONString(betaHome)+`}`)
	work := dshProfileFile(betaHome, "work")
	writeTestFile(t, work, dshProfileTemplate, 0o600)
	writeDshJSON(t, filepath.Join(betaHome, dshDesktopProfileState), `{"version":1,"active":"work"}`)

	apps := t.TempDir()
	was := appFolders
	appFolders = func() []string { return []string{apps} }
	t.Cleanup(func() { appFolders = was })
	installApp(t, apps, "DSH Desktop Beta.app", strings.NewReplacer(
		"ai.deepseek.dsh.desktop.next", dshBetaAppID,
		"DSH NEXT", "DSH Desktop Beta",
		"2.0.24-next", "2.0.24-beta.2").Replace(dshNextPlist))
	writeTestFile(t, filepath.Join(apps, "DSH Desktop Beta.app", "Contents", "Resources", "app", "package.json"), dshNextManifest, 0o644)

	a := dshBeta(home)
	if a.ID != dshBetaID || a.Name != "DSH Desktop Beta" || !a.HasApp() || !a.Detected() {
		t.Fatalf("Beta row: %+v", a)
	}
	if a.AppVersion() != "2.0.24-beta.2" {
		t.Fatalf("Beta version = %q", a.AppVersion())
	}
	if a.Path != work {
		t.Fatalf("Beta Path = %s, want %s", a.Path, work)
	}
	if stable := dsh(home); stable.Path == work {
		t.Fatal("stable dsh followed Beta's home")
	}
	if err := a.Restore(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Dir(work)); !os.IsNotExist(err) {
		t.Fatalf("Restore left the active Beta profile: %v", err)
	}

	writeTestFile(t, work, dshProfileTemplate, 0o600)
	dshKeyFixture(betaHome)
	web := dshProfileFile(betaHome, "web")
	writeTestFile(t, web, dshProfileTemplate, 0o600)
	a = dshBeta(home)
	if err := a.Pick("model", "magpie/deepseek/pro"); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, work, dshProfileTemplate, 0o600)
	if trouble := dshWiredOnce(); trouble != "" {
		t.Fatal(trouble)
	}
	if b, err := os.ReadFile(work); err != nil || !strings.Contains(string(b), "provider: "+dshRoute) {
		t.Fatalf("Beta round did not wire its active profile: %v\n%s", err, b)
	}
	a = dshBeta(home)
	if !a.Joined() || !a.Wired() || a.Field("model").Get() != "magpie/deepseek/pro" {
		t.Fatalf("Beta active profile not read: joined=%v wired=%v model=%q", a.Joined(), a.Wired(), a.Field("model").Get())
	}
}

func dshJSONString(s string) string {
	b, err := json.Marshal(s)
	if err != nil {
		panic(err)
	}
	return string(b)
}
