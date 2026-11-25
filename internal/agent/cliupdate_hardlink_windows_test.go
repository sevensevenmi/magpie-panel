package agent

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/yetone/magpie/internal/update"
)

// npm's native .exe can stand beside its .cmd in a Windows prefix, and
// PATHEXT finds the .exe first. The .cmd still names the package.
func TestHowInstalledWindowsNpmExe(t *testing.T) {
	prefix := filepath.Join(t.TempDir(), "npm")
	file(t, codexVendor(prefix), "")
	npm := file(t, filepath.Join(prefix, "npm.cmd"), "")
	file(t, filepath.Join(prefix, "codex.cmd"), "@ECHO off\r\n\"%dp0%\\node_modules\\@openai\\codex\\bin\\codex.js\" %*\r\n")
	exe := file(t, filepath.Join(prefix, "CODEX.EXE"), strings.Repeat("MZ", 40<<10))

	u := howInstalled(cliSpecs["codex"], exe)
	if u == nil || u.via != "npm" || u.pkgDir != filepath.Join(prefix, "node_modules", "@openai", "codex") {
		t.Fatalf("npm's codex.exe: %+v", u)
	}
	if want := []string{npm, "install", "-g", "--prefix", prefix, "@openai/codex@latest"}; !reflect.DeepEqual(u.cmd, want) {
		t.Errorf("command: %q, want %q", u.cmd, want)
	}

	// An .exe with no shim beside it says nothing sure.
	other := file(t, filepath.Join(prefix, "gemini.exe"), "MZ")
	if u := howInstalled(cliSpecs["gemini"], other); u != nil {
		t.Errorf("an .exe with no .cmd: %+v", u)
	}
}

// npm replaces the package's vendor file, while a prefix .exe that is a
// hard link to it keeps running the old bytes. UpdateCLI must relink the
// prefix .exe to what npm installed, and leave a copy alone.
func TestUpdateCLIWindowsHardLink(t *testing.T) {
	const older, newer = "codex-cli 0.146.0\n", "codex-cli 0.160.0\n"
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("APPDATA", filepath.Join(home, "AppData", "Roaming"))
	t.Setenv("LOCALAPPDATA", filepath.Join(home, "AppData", "Local"))

	tmp := t.TempDir()
	prefix := newCodexPrefix(t, tmp, "npm", older)
	vendor := codexVendor(prefix)
	exe := filepath.Join(prefix, "codex.exe")
	if err := os.Link(vendor, exe); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", prefix)

	reg := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/@openai/codex/latest" {
			http.NotFound(w, r)
			return
		}
		w.Write([]byte(`{"name":"@openai/codex","version":"0.160.0"}`))
	}))
	defer reg.Close()

	oldReg, oldRun, oldVersion, oldLetGo := npmRegistry, runUpdate, runVersion, letGoBeforeUpdate
	t.Cleanup(func() {
		npmRegistry, runUpdate, runVersion, letGoBeforeUpdate = oldReg, oldRun, oldVersion, oldLetGo
		versions, latests = memo{}, memo{}
	})
	npmRegistry = reg.URL + "/"
	versions, latests = memo{}, memo{}
	runVersion = func(bin string) string {
		b, _ := os.ReadFile(bin)
		return string(b)
	}
	letGoBeforeUpdate = func(context.Context, string) {}

	replacing := vendor
	ran := 0
	runUpdate = func(_ context.Context, u *updater) ([]byte, error) {
		ran++
		if err := os.WriteFile(replacing+".new", []byte(newer), 0o755); err != nil {
			return nil, err
		}
		return []byte("added 2 packages"), os.Rename(replacing+".new", replacing)
	}

	a := &Agent{ID: "codex", Name: "Codex", Bin: "codex"}
	c, ok := a.CLI()
	if !ok || c.Version != "0.146.0" || c.Latest != "0.160.0" || !c.Update || c.Via != "npm" {
		t.Fatalf("before: %+v, %v", c, ok)
	}
	c, err := a.UpdateCLI()
	if err != nil || c.Version != "0.160.0" || c.Update {
		t.Fatalf("after: %+v, %v", c, err)
	}
	if got, _ := os.ReadFile(exe); string(got) != newer {
		t.Errorf("the .exe on PATH runs %q, want %q", got, newer)
	}
	if !sameFile(exe, vendor) {
		t.Error("the .exe on PATH is not the file npm installed")
	}
	if _, err := os.Stat(exe + ".old"); !os.IsNotExist(err) {
		t.Errorf("the old .exe was left behind at %s.old", exe)
	}
	if ran != 1 {
		t.Errorf("the update ran %d times", ran)
	}

	// A copy is not a hard link and is not magpie's to replace. The update
	// still ran, but the CLI on PATH is unchanged and says so.
	copyPrefix := newCodexPrefix(t, tmp, "npm2", older)
	copied := file(t, filepath.Join(copyPrefix, "codex.exe"), older)
	replacing = codexVendor(copyPrefix)
	t.Setenv("PATH", copyPrefix)
	versions, latests = memo{}, memo{}
	b := &Agent{ID: "codex", Name: "Codex", Bin: "codex"}
	if _, err := b.UpdateCLI(); err == nil || !strings.Contains(err.Error(), "still 0.146.0") {
		t.Errorf("a copy: %v", err)
	}
	if got, _ := os.ReadFile(copied); string(got) != older {
		t.Errorf("a copy was rewritten: %q", got)
	}
}

// A failed link leaves the old .exe in place, and removes the temporary
// .old name on rollback.
func TestRelinkExeRestoresOldOnLinkFailure(t *testing.T) {
	tmp := t.TempDir()
	exe := file(t, filepath.Join(tmp, "prefix", "codex.exe"), "old")
	src := file(t, filepath.Join(tmp, "package", "vendor", "codex.exe"), "new")
	file(t, exe+".old", "stale")

	oldLink := linkFile
	t.Cleanup(func() { linkFile = oldLink })
	linkFile = func(string, string) error { return errors.New("access denied") }

	if err := relinkExe(exe, src); err == nil || !strings.Contains(err.Error(), "access denied") {
		t.Fatalf("link failure: %v", err)
	}
	if got, _ := os.ReadFile(exe); string(got) != "old" {
		t.Errorf("the old .exe was not put back: %q", got)
	}
	if _, err := os.Stat(exe + ".old"); !os.IsNotExist(err) {
		t.Errorf("rollback left %s.old", exe)
	}
}

// A permission failure while renaming leaves the .exe untouched.
func TestRelinkExeKeepsExeWhenRenameFails(t *testing.T) {
	tmp := t.TempDir()
	exe := file(t, filepath.Join(tmp, "prefix", "codex.exe"), "old")
	src := file(t, filepath.Join(tmp, "package", "vendor", "codex.exe"), "new")

	oldRename := renameFile
	t.Cleanup(func() { renameFile = oldRename })
	renameFile = func(string, string) error { return errors.New("rename denied") }

	if err := relinkExe(exe, src); err == nil || !strings.Contains(err.Error(), "rename denied") {
		t.Fatalf("rename failure: %v", err)
	}
	if got, _ := os.ReadFile(exe); string(got) != "old" {
		t.Errorf("the .exe changed after a failed rename: %q", got)
	}
	if _, err := os.Stat(exe + ".old"); !os.IsNotExist(err) {
		t.Errorf("a failed rename left %s.old", exe)
	}
}

// Windows allows a running image to be renamed aside. The old process can
// keep .old until it exits, but the path PATH runs must already point at
// the new file.
func TestRelinkExeWhileExeOpen(t *testing.T) {
	tmp := t.TempDir()
	systemCmd := filepath.Join(os.Getenv("SystemRoot"), "System32", "cmd.exe")
	b, err := os.ReadFile(systemCmd)
	if err != nil {
		t.Skip("no cmd.exe to run:", err)
	}
	exe := file(t, filepath.Join(tmp, "prefix", "codex.exe"), string(b))
	src := file(t, filepath.Join(tmp, "package", "vendor", "codex.exe"), "new")
	src2 := file(t, filepath.Join(tmp, "package", "vendor", "codex-2.exe"), "newer")
	run := exec.Command(exe, "/d", "/c", "ping -n 30 127.0.0.1 >nul")
	run.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if err := run.Start(); err != nil {
		t.Fatal(err)
	}
	stopped := false
	t.Cleanup(func() {
		if !stopped {
			run.Process.Kill()
			run.Wait()
		}
		update.RemoveOld(exe)
	})
	time.Sleep(100 * time.Millisecond)

	if err := relinkExe(exe, src); err != nil {
		t.Fatalf("relink with the old .exe open: %v", err)
	}
	if got, _ := os.ReadFile(exe); string(got) != "new" {
		t.Errorf("the .exe on PATH did not move to the new file: %q", got)
	}
	// The first old image still holds .old; a second update while it runs
	// uses the next name rather than failing on the occupied one.
	if err := relinkExe(exe, src2); err != nil {
		t.Fatalf("second relink with the old image open: %v", err)
	}
	if got, _ := os.ReadFile(exe); string(got) != "newer" {
		t.Errorf("the second .exe update did not take: %q", got)
	}
	old := exe + ".old"
	if _, err := os.Stat(old); err == nil {
		t.Log("the running old image still holds .old")
	}
	if err := run.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
		t.Fatal(err)
	}
	run.Wait()
	stopped = true
	update.RemoveOld(exe)
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Errorf("the old image remained after the process exited: %v", err)
	}
}

// newCodexPrefix is an npm prefix holding @openai/codex as npm installs it:
// the .cmd that names it, its package, and npm.cmd beside them.
func newCodexPrefix(t *testing.T, tmp, name, version string) string {
	t.Helper()
	prefix := filepath.Join(tmp, name)
	file(t, codexVendor(prefix), version)
	file(t, filepath.Join(prefix, "npm.cmd"), "")
	file(t, filepath.Join(prefix, "codex.cmd"), "@ECHO off\r\n\"%dp0%\\node_modules\\@openai\\codex\\bin\\codex.js\" %*\r\n")
	return prefix
}

// codexVendor is the native binary inside @openai/codex's platform package,
// which is what an .exe in the prefix is a hard link to.
func codexVendor(prefix string) string {
	return filepath.Join(prefix, "node_modules", "@openai", "codex", "node_modules", "@openai", "codex-win32-x64", "vendor", "x86_64-pc-windows-msvc", "bin", "codex.exe")
}
