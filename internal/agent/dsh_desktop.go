package agent

// DSH NEXT and DSH Desktop Beta keep their data-folder choice outside the
// folder itself, and keep the active profile in desktop-next.json. Their
// fixed locator file is the only app-selected home magpie trusts; the
// last-home file is a change record, not a selection, and is never read
// here. A malformed locator or profile fails closed.

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/yetone/magpie/internal/appdir"
	"github.com/yetone/magpie/internal/gateway"
)

const (
	dshBetaID               = "dsh-beta"
	dshNextHomeEnv          = "DSH_DESKTOP_NEXT_HOME"
	dshDesktopLocationFile  = "desktop-next-location.json"
	dshDesktopProfileState  = "desktop-next.json"
	dshNextAppID            = "ai.deepseek.dsh.desktop.next"
	dshBetaAppID            = "ai.deepseek.dsh.desktop.beta"
	dshBetaDefaultDirectory = ".dsh-beta"
)

// dshProfileFile is one profile's patch list.
func dshProfileFile(dir, name string) string {
	return filepath.Join(dir, "profiles", name, "cordis.patch.yml")
}

// dshProfileIn is the patch list of the profile named name, "" when the
// profile has none.
func dshProfileIn(name string, files []string) string {
	for _, f := range files {
		if filepath.Base(filepath.Dir(f)) == name {
			return f
		}
	}
	return ""
}

// dshActiveProfile is desktop-next.json's active profile. named is false
// when the file is absent, the default desktop profile applies then. A
// file that exists but cannot be read or names no valid profile is an
// error, never the default.
func dshActiveProfile(dir string) (name string, named bool, err error) {
	if dir == "" {
		return "", false, errors.New("DeepSeekRuntime home is unknown")
	}
	path := filepath.Join(dir, dshDesktopProfileState)
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", true, err
	}
	var state struct {
		Active string `json:"active"`
	}
	if err := json.Unmarshal(b, &state); err != nil {
		return "", true, fmt.Errorf("invalid %s: %w", path, err)
	}
	if !dshValidProfileName(state.Active) {
		return "", true, fmt.Errorf("invalid active profile in %s: %q", path, state.Active)
	}
	return state.Active, true, nil
}

// dshActiveProfileName is dshActiveProfile with def for an absent file.
// Callers that write check the error first, so a malformed file cannot
// turn into def between those checks.
func dshActiveProfileName(dir, def string) string {
	name, named, err := dshActiveProfile(dir)
	if err != nil || !named {
		return def
	}
	return name
}

// dshValidProfileName is DeepSeekRuntime's profileName rule.
func dshValidProfileName(name string) bool {
	if len(name) < 1 || len(name) > 64 {
		return false
	}
	for i, r := range name {
		ok := r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || i > 0 && (r == '_' || r == '-')
		if !ok {
			return false
		}
	}
	switch strings.ToLower(name) {
	case "node_modules", "con", "prn", "aux", "nul",
		"com1", "com2", "com3", "com4", "com5", "com6", "com7", "com8", "com9",
		"lpt1", "lpt2", "lpt3", "lpt4", "lpt5", "lpt6", "lpt7", "lpt8", "lpt9":
		return false
	}
	return true
}

// dshDesktopHomeIn is appName's selected DSH home, with the user's home
// supplied. The app's fixed locator wins; without one
// DSH_DESKTOP_NEXT_HOME applies, then the app's default. A locator that
// exists but is unreadable or holds a relative path returns ok false, and
// no fallback is guessed.
func dshDesktopHomeIn(appName, defaultDirectory, home string) (string, bool) {
	if home == "" {
		return "", false
	}
	path := filepath.Join(dshAppDataDir(home), appName, "home", dshDesktopLocationFile)
	b, err := os.ReadFile(path)
	switch {
	case err == nil:
		var location struct {
			Home string `json:"home"`
		}
		if json.Unmarshal(b, &location) != nil || !dshAbsolutePath(location.Home) {
			return "", false
		}
		return filepath.Clean(location.Home), true
	case !errors.Is(err, os.ErrNotExist):
		return "", false
	}
	if dir, present, valid := dshNextHomeSetting(); present {
		if !valid {
			return "", false
		}
		return filepath.Clean(dir), true
	}
	return filepath.Join(home, defaultDirectory), true
}

// dshNextHomeSetting is DSH_DESKTOP_NEXT_HOME as a path setting. present
// says it was set at all; valid is false for a relative value, which must
// not turn into the default.
func dshNextHomeSetting() (dir string, present, valid bool) {
	if dir := appdir.Getenv(dshNextHomeEnv); dir != "" {
		return dir, true, dshAbsolutePath(dir)
	}
	if raw, ok := os.LookupEnv(dshNextHomeEnv); ok && strings.TrimSpace(raw) != "" {
		raw = strings.TrimSpace(raw)
		return raw, true, dshAbsolutePath(raw)
	}
	return "", false, true
}

// dshNextSelected says DSH NEXT's own home rules apply: its environment
// names a home, or its locator exists (readable or not). An installed NEXT
// with neither still defaults to ~/.dsh, so the row checks that separately.
func dshNextSelected(home string) bool {
	if _, present, _ := dshNextHomeSetting(); present {
		return true
	}
	_, err := os.Stat(filepath.Join(dshAppDataDir(home), "DSH NEXT", "home", dshDesktopLocationFile))
	return err == nil || !errors.Is(err, os.ErrNotExist)
}

// dshNextInstalled is DSH NEXT among dsh's builds, without asking for the
// agent row (which is what asks this).
func dshNextInstalled() bool {
	for _, b := range desktopApps["dsh"].more {
		if b.id == dshNextAppID {
			return b.installed()
		}
	}
	return false
}

// dshBetaDataDir is DSH Desktop Beta's selected home, "" when it can't be
// told. The folder is recorded for dshListFor so its routes use Beta's own
// model choices.
func dshBetaDataDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return dshBetaDataDirIn(home)
}

func dshBetaDataDirIn(home string) string {
	dir, ok := dshDesktopHomeIn("DSH Desktop Beta", dshBetaDefaultDirectory, home)
	if !ok || dir == "" {
		return ""
	}
	if dsh := dshHomeFor(home); dsh != "" && dshSameDir(dir, dsh) {
		return ""
	}
	dshApps.Store(filepath.Clean(dir), dshBetaID)
	return dir
}

// dshBetaRoundHere says there is a Beta home to keep wired: its environment
// or locator is present, or its default home has profiles. An installed
// Beta that has never opened has no profile for a round to write.
func dshBetaRoundHere() bool {
	if _, present, _ := dshNextHomeSetting(); present {
		return true
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return false
	}
	if isDir(filepath.Join(home, dshBetaDefaultDirectory, "profiles")) {
		return true
	}
	_, err = os.Stat(filepath.Join(dshAppDataDir(home), "DSH Desktop Beta", "home", dshDesktopLocationFile))
	return err == nil || !errors.Is(err, os.ErrNotExist)
}

func dshBeta(home string) *Agent { return dshBetaAt(here(home)) }

// dshBetaAt is DSH Desktop Beta. It is an app-only row: the beta shell
// runs its own dsh and keeps a home apart from the stable app's.
func dshBetaAt(at place) *Agent {
	app := func() bool { return (&Agent{ID: dshBetaID}).HasApp() }
	dir := dshBetaDataDirIn(at.home)
	if dir == "" {
		return dshUnavailable(dshBetaID, "DSH Desktop Beta", "", "deepseek-color", app)
	}
	if _, _, err := dshActiveProfile(dir); err != nil {
		return dshUnavailable(dshBetaID, "DSH Desktop Beta", dir, "deepseek-color", app)
	}
	profile := func() string { return dshActiveProfileName(dir, "desktop") }
	fresh := func() string { return dshProfileFile(dir, profile()) }
	a := dshOnProfile(at, dir, dshBetaID, fresh, profile, true)
	a.ID, a.Name, a.Icon = dshBetaID, "DSH Desktop Beta", "deepseek-color"
	a.sign = app
	a.Notice = func() string {
		var notes []string
		if app() && dshUnbooted(fresh()) {
			notes = append(notes, noticeDsh5.String())
		} else if len(dshProfiles(dir)) > 0 {
			notes = append(notes, noticeDsh1.String())
		}
		if dshSearchesHere(dir) && !gateway.CanSearch() {
			notes = append(notes, noticeDsh3.String())
		}
		return strings.Join(notes, " ")
	}
	a.Restore = func() error {
		if err := dshTidyMade(dir, fresh()); err != nil {
			return err
		}
		return dshTidyUnbooted(dir)
	}
	return a
}

// dshBetaVersion is the dsh DSH Desktop Beta bundles, read from its Mac
// manifest; "unknown" where that is inside app.asar, and none when the app
// isn't installed.
func dshBetaVersion() []string {
	app, ok := desktopApps[dshBetaID]
	if !ok || !app.installed() {
		return nil
	}
	if v := dshBundledVersion(app); v != "" {
		return []string{v}
	}
	return []string{"unknown"}
}

// dshUnavailable is a detected app whose config cannot be located safely.
// It has no fields, so nothing can be written through it.
func dshUnavailable(id, name, dir, icon string, app func() bool) *Agent {
	return &Agent{ID: id, Name: name, Icon: icon, Dir: dir, detect: app}
}

func dshAbsolutePath(path string) bool {
	if path == "" || strings.IndexByte(path, 0) >= 0 {
		return false
	}
	if filepath.IsAbs(path) {
		return true
	}
	return runtime.GOOS == "windows" && (strings.HasPrefix(path, `\`) || strings.HasPrefix(path, "/"))
}

func dshSameDir(a, b string) bool {
	a, b = filepath.Clean(a), filepath.Clean(b)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

// dshAppDataDir is Electron's appData folder, from the home magpie is
// building an agent for. It avoids os.UserConfigDir's repeated lookup in
// the serving round, which runs every 30 seconds.
func dshAppDataDir(home string) string {
	switch runtime.GOOS {
	case "windows":
		if dir := appdir.Getenv("APPDATA"); dir != "" {
			return dir
		}
		return filepath.Join(home, "AppData", "Roaming")
	case "darwin":
		return filepath.Join(home, "Library", "Application Support")
	default:
		if dir := appdir.Getenv("XDG_CONFIG_HOME"); dir != "" {
			return dir
		}
		return filepath.Join(home, ".config")
	}
}
