package agent

import (
	"os"
	"path"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/yetone/magpie/internal/proc"
)

// linuxApp is how a desktop app whose vendor ships no Linux package is
// found on Linux, where others package it (DeepSeek Harness Desktop:
// the AUR, a .deb, an AppImage). It is installed when one of dirs, the
// folders packagers install it to, holds exe and an Electron app's
// resources/app.asar, or when a desktop entry in the XDG data folders'
// applications/ is the app's (its Name is name and it handles links of
// scheme, the app's own) and its program is there: what a package puts
// in /usr/share/applications, and what AppImageLauncher, Gear Lever or
// the user writes in ~/.local/share/applications for an AppImage
// anywhere. Apps with no scheme may name exact desktop ids and exact
// release-artifact shapes; an AppImage still needs that entry and an
// executable file.
type linuxApp struct {
	dirs             []string
	exe              string
	name             string
	scheme           string
	desktopIDs       []string
	appImagePatterns []string
}

// linuxAppOS is the system linuxApp looks on; tests stand in Linux.
var linuxAppOS = runtime.GOOS

// installed reports whether l is installed here (see linuxApp); false
// for no linuxApp, and on macOS and Windows.
func (l *linuxApp) installed() bool {
	if l == nil || linuxAppOS == "darwin" || linuxAppOS == "windows" {
		return false
	}
	// through proc.SystemDirs, so a test binary never reads the machine's
	for _, dir := range proc.SystemDirs(l.dirs...) {
		if isFile(filepath.Join(dir, l.exe)) && isFile(filepath.Join(dir, "resources", "app.asar")) {
			return true
		}
	}
	return slices.ContainsFunc(appEntryPrograms(l), programThere)
}

// programThere reports whether a desktop entry's program is there: an
// executable file at its path, or one on PATH by its name.
func programThere(prog string) bool {
	if !filepath.IsAbs(prog) {
		_, err := proc.LookPath(prog)
		return err == nil
	}
	st, err := os.Stat(prog)
	return err == nil && !st.IsDir() && st.Mode()&0o111 != 0
}

// appEntryDirs are the folders desktop entries are read from, the
// user's first (xdgDataDirs' applications/). The system's go through
// proc.SystemDirs: testenv sets the user's data folder to the test's own
// but leaves $XDG_DATA_DIRS, which names the machine's.
func appEntryDirs() []string {
	data := xdgDataDirs()
	if len(data) == 0 {
		return nil
	}
	dirs := []string{filepath.Join(data[0], "applications")}
	for _, d := range proc.SystemDirs(data[1:]...) {
		dirs = append(dirs, filepath.Join(d, "applications"))
	}
	return dirs
}

// appEntryPrograms are the programs of the desktop entries named name
// that handle scheme's links, as the Desktop Entry Specification finds
// entries: the first folder with an entry of a file name has the one that
// counts, and one that is Hidden hides it. Each folder is read again
// when its time changed (an entry added, removed or renamed over), else
// once a minute: this runs whenever dsh's row is read.
func appEntryPrograms(l *linuxApp) []string {
	var progs []string
	seen := map[string]bool{}
	for _, dir := range appEntryDirs() {
		for file, prog := range entriesIn(dir, l) {
			if seen[file] {
				continue
			}
			seen[file] = true
			if prog != "" {
				progs = append(progs, prog)
			}
		}
	}
	return progs
}

// entryScan is what one applications/ folder held at its time, read at
// read: each entry's file name, with the program of one that is the
// app's ("" for another app's, or a hidden one).
type entryScan struct {
	at, read time.Time
	entries  map[string]string
}

var entryScans = struct {
	sync.Mutex
	m map[string]entryScan
}{m: map[string]entryScan{}}

// entriesIn is the desktop entries in dir (see entryScan); none when it
// can't be read.
func entriesIn(dir string, l *linuxApp) map[string]string {
	st, err := os.Stat(dir)
	if err != nil || !st.IsDir() {
		return nil
	}
	key := dir + "\x00" + l.name + "\x00" + l.scheme + "\x00" + strings.Join(l.desktopIDs, "\x00") + "\x00" + strings.Join(l.appImagePatterns, "\x00")
	entryScans.Lock()
	scan, ok := entryScans.m[key]
	entryScans.Unlock()
	// an entry written over in place leaves the folder's time: read
	// again after a minute. A folder changed within a second of the read
	// is read again too: Linux stamps times by the kernel's tick, so a
	// change soon after the read can leave the time as it was.
	if ok && scan.at.Equal(st.ModTime()) && scan.read.Sub(scan.at) > time.Second && time.Since(scan.read) < time.Minute {
		return scan.entries
	}
	files, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	entries := map[string]string{}
	for _, f := range files {
		if f.IsDir() || !strings.HasSuffix(f.Name(), ".desktop") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, f.Name()))
		if err != nil {
			continue
		}
		entries[f.Name()] = appEntryProgram(string(b), f.Name(), l)
	}
	entryScans.Lock()
	entryScans.m[key] = entryScan{at: st.ModTime(), read: time.Now(), entries: entries}
	entryScans.Unlock()
	return entries
}

// appEntryProgram is the program a desktop entry runs when it is the
// app's: its Name is name, it handles scheme's links and isn't Hidden;
// "" otherwise. A program run through env (Exec=env VAR=x prog) is prog.
func appEntryProgram(entry, file string, l *linuxApp) string {
	keys := map[string]string{}
	in := false
	for _, line := range strings.Split(entry, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "[") {
			in = line == "[Desktop Entry]"
			continue
		}
		if k, v, ok := strings.Cut(line, "="); in && ok {
			if k = strings.TrimSpace(k); keys[k] == "" {
				keys[k] = strings.TrimSpace(v)
			}
		}
	}
	if keys["Hidden"] == "true" || keys["Type"] != "" && keys["Type"] != "Application" {
		return ""
	}
	argv := desktopArgs(entry, "")
	if len(argv) > 0 && argv[0] == "env" {
		argv = argv[1:]
		for len(argv) > 0 && strings.Contains(argv[0], "=") {
			argv = argv[1:]
		}
	}
	if len(argv) == 0 {
		return ""
	}
	prog := argv[0]
	if l.appImage(file, keys, prog) {
		return prog
	}
	if keys["Name"] != l.name || !slices.Contains(strings.Split(keys["MimeType"], ";"), "x-scheme-handler/"+l.scheme) {
		return ""
	}
	return prog
}

// appImage reports whether file is an AppImage entry this linuxApp owns:
// its Exec is an absolute executable whose exact artifact family matches,
// and the entry is either the vendor's exact desktop id or carries
// AppImage integration's identifier. A correct-looking artifact with no
// desktop entry is never found: appEntryDirs only reads XDG applications.
func (l *linuxApp) appImage(file string, keys map[string]string, prog string) bool {
	if len(l.appImagePatterns) == 0 || !filepath.IsAbs(prog) || !strings.HasSuffix(strings.ToLower(prog), ".appimage") {
		return false
	}
	if !l.appImageName(filepath.Base(prog)) || !appImageThere(prog) {
		return false
	}
	return slices.Contains(l.desktopIDs, file) || keys["X-AppImage-Identifier"] != ""
}

// appImageName says name is one of this app's exact release artifact
// shapes, not merely an AppImage with a similar product name.
func (l *linuxApp) appImageName(name string) bool {
	return slices.ContainsFunc(l.appImagePatterns, func(pattern string) bool {
		ok, err := path.Match(pattern, name)
		return err == nil && ok
	})
}

// appImageThere is an AppImage at an absolute path: a regular executable
// file. PATH is never consulted for one.
func appImageThere(prog string) bool {
	st, err := os.Stat(prog)
	return err == nil && st.Mode().IsRegular() && st.Mode()&0o111 != 0
}
