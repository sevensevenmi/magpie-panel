package library

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// WardLu (#6697): a folder skill can lose library/skills/<name> while its
// source folder and registry entry remain. Sync must rebuild the library
// entry before writing agent or router links, rather than reporting success
// with every link dangling.
func TestSyncRestoresMissingFolderSkillEntry(t *testing.T) {
	for _, router := range []bool{false, true} {
		name := "router-off"
		if router {
			name = "router-on"
		}
		t.Run(name, func(t *testing.T) {
			h := sandbox(t)
			noGoose(t, h)
			src := filepath.Join(h, "skills-source", "restore-me")
			skill(t, src, "restore-me", "Restore this skill")
			ok(t)(InstallSkills(filepath.Dir(src), []string{"restore-me"}, []string{"claude", "codex"}))
			if router {
				ok(t)(SetSkillRouter(true))
			}

			lib := skillDir("restore-me")
			if !linked(lib) || realDir(lib) != realDir(src) {
				t.Fatalf("before removal: library entry %s does not link to %s", realDir(lib), realDir(src))
			}
			if err := os.Remove(lib); err != nil {
				t.Fatal(err)
			}

			res := ok(t)(Sync())
			if len(res.Problems) != 0 {
				t.Fatalf("sync problems: %+v", res.Problems)
			}
			if !linked(lib) || realDir(lib) != realDir(src) {
				t.Fatalf("sync did not restore the library entry: linked=%v target=%s", linked(lib), realDir(lib))
			}
			if _, err := os.Stat(filepath.Join(lib, "SKILL.md")); err != nil {
				t.Fatalf("restored library entry is not readable: %v", err)
			}
			if router {
				at := filepath.Join(routerDir(filepath.Join(h, ".claude", "skills")), ".skills", "restore-me", "SKILL.md")
				if _, err := os.Stat(at); err != nil {
					t.Fatalf("router link still dangles: %v", err)
				}
			}
			for _, id := range []string{".claude", ".codex"} {
				at := filepath.Join(h, id, "skills", "restore-me")
				if router {
					if id == ".codex" {
						if !stubOf(at) {
							t.Fatalf("%s has no router stub", id)
						}
						if _, err := os.Stat(filepath.Join(at, "SKILL.md")); err != nil {
							t.Fatalf("%s's router stub is unreadable: %v", id, err)
						}
						continue
					}
					if _, err := os.Lstat(at); !os.IsNotExist(err) {
						t.Fatalf("%s has a second direct link while the router is on: %v", id, err)
					}
					continue
				}
				if _, err := os.Stat(filepath.Join(at, "SKILL.md")); err != nil {
					t.Fatalf("%s still has a dangling link: %v", id, err)
				}
			}
		})
	}
}

// When the folder source is gone too, sync cannot rebuild the entry. It
// must say which skill is missing and remove links it made earlier instead
// of writing or keeping links to nothing.
func TestSyncReportsUnrestorableMissingSkillEntry(t *testing.T) {
	h := sandbox(t)
	noGoose(t, h)
	src := filepath.Join(h, "skills-source", "lost")
	skill(t, src, "lost", "This source will go")
	ok(t)(InstallSkills(filepath.Dir(src), []string{"lost"}, []string{"claude"}))
	if err := os.RemoveAll(skillDir("lost")); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(src); err != nil {
		t.Fatal(err)
	}
	agentAt := filepath.Join(h, ".claude", "skills", "lost")

	res, err := Sync()
	if err != nil {
		t.Fatal(err)
	}
	i := slices.IndexFunc(res.Problems, func(p Problem) bool { return p.What == "skill:lost" })
	if i < 0 {
		t.Fatalf("sync hid the missing library entry: %+v", res.Problems)
	}
	if !strings.Contains(res.Problems[i].Error, "library entry") {
		t.Fatalf("problem does not name the missing entry: %+v", res.Problems[i])
	}
	if _, err := os.Lstat(agentAt); !os.IsNotExist(err) {
		t.Fatalf("sync kept a dangling agent link: %v", err)
	}
}

// Applied records the skills magpie wrote. Once the library entry is gone,
// a healthy link at that path can be the user's replacement; it isn't
// magpie's dangling entry and must stay.
func TestSyncKeepsAHealthyReplacementForAMissingSkill(t *testing.T) {
	h := sandbox(t)
	noGoose(t, h)
	ok(t)(CreateSkill("notes", "Mine", []string{"claude"}))
	user := filepath.Join(h, "user", "notes")
	skill(t, user, "notes", "Mine")
	at := filepath.Join(h, ".claude", "skills", "notes")
	if err := unlink(at); err != nil {
		t.Fatal(err)
	}
	if err := dirLink(user, at); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(skillDir("notes")); err != nil {
		t.Fatal(err)
	}

	res, err := Sync()
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(res.Problems, func(p Problem) bool { return p.What == "skill:notes" }) {
		t.Fatalf("sync hid the missing library entry: %+v", res.Problems)
	}
	if !linked(at) || realDir(at) != realDir(user) {
		t.Fatalf("sync removed the user's healthy replacement: %s", at)
	}
}
