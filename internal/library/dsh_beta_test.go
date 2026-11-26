package library

import (
	"path/filepath"
	"testing"
)

// DSH Desktop Beta's Library target follows its own home and active
// profile, and doesn't turn stable dsh into that home.
func TestDshBetaLibraryTarget(t *testing.T) {
	h := sandbox(t)
	home := filepath.Join(h, ".dsh-beta")
	work := filepath.Join(home, "profiles", "work", "cordis.patch.yml")
	write(t, work, "[]\n")
	write(t, filepath.Join(home, "desktop-next.json"), `{"version":1,"active":"work"}`)

	tg := targetByID("dsh-beta")
	if tg == nil {
		t.Fatal("dsh-beta: no target")
	}
	if tg.MCP == nil || tg.MCP.Path != work || tg.Instructions != filepath.Join(home, "AGENTS.md") || tg.Skills != filepath.Join(home, "skills") {
		t.Fatalf("dsh-beta target: %+v", tg)
	}
	if dsh := targetByID("dsh"); dsh != nil && dsh.MCP != nil && dsh.MCP.Path == work {
		t.Fatal("stable dsh took DSH Desktop Beta's profile")
	}
}
