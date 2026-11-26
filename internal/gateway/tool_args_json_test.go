package gateway

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestToolArgsRepairLeavesValidJSONAlone(t *testing.T) {
	for _, args := range []string{
		`{"pattern":"*.md","limit":2,"recursive":true,"nested":{"x":null},"items":["a","b"]}`,
		`["a",1,true,null,{"x":"a,b}"}]`,
		`"root"`,
		`true`,
		`123`,
		`null`,
		`{}`,
		`[]`,
	} {
		t.Run(args, func(t *testing.T) {
			if got, changed := repairToolArgs(args); changed || got != args {
				t.Fatalf("valid arguments changed: %s, want %s", got, args)
			}
			for i := 0; i <= len(args); i++ {
				var r toolArgsRepair
				got := r.feed(args[:i]) + r.feed(args[i:]) + r.finish()
				if got != args {
					t.Fatalf("split %d changed valid arguments: %s, want %s", i, got, args)
				}
			}
		})
	}
}

func TestToolArgsRepairUnquotedStrings(t *testing.T) {
	for _, c := range []struct {
		in, want string
	}{
		{`{"pattern": *.md}`, `{"pattern":"*.md"}`},
		{`{"pattern": *.{py,md,sh}, "path": "src"}`, `{"pattern":"*.{py,md,sh}","path":"src"}`},
		{`{"patterns":[*.md, src/*.{go,ts}]}`, `{"patterns":["*.md","src/*.{go,ts}"]}`},
		{`{"pattern": C:\Users\example\.agents\skills\truthguard\**}`, `{"pattern":"C:\\Users\\example\\.agents\\skills\\truthguard\\**"}`},
		{`{"nested":{"pattern": foo/bar}}`, `{"nested":{"pattern":"foo/bar"}}`},
	} {
		t.Run(c.in, func(t *testing.T) {
			got, changed := repairToolArgs(c.in)
			if !changed {
				t.Fatalf("arguments were not changed: %s", c.in)
			}
			var g, w any
			if err := json.Unmarshal([]byte(got), &g); err != nil {
				t.Fatalf("repaired arguments: %v: %s", err, got)
			}
			if err := json.Unmarshal([]byte(c.want), &w); err != nil {
				t.Fatalf("want: %v: %s", err, c.want)
			}
			if !reflect.DeepEqual(g, w) {
				t.Fatalf("repaired %s, want %s", got, c.want)
			}
		})
	}
}

func TestToolArgsRepairAcrossEverySplit(t *testing.T) {
	in := `{"pattern": changes/*/planning-context.md, "nested": [src/*.{go,ts}, C:\Users\example]}`
	want := `{"pattern":"changes/*/planning-context.md","nested":["src/*.{go,ts}","C:\\Users\\example"]}`
	for i := 0; i <= len(in); i++ {
		var r toolArgsRepair
		got := r.feed(in[:i]) + r.feed(in[i:]) + r.finish()
		var g, w any
		if err := json.Unmarshal([]byte(got), &g); err != nil {
			t.Fatalf("split %d: %v: %s", i, err, got)
		}
		if err := json.Unmarshal([]byte(want), &w); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(g, w) {
			t.Fatalf("split %d: %s, want %s", i, got, want)
		}
	}
}

func TestToolArgsRepairPreservesTrailingWhitespace(t *testing.T) {
	in := "{\"pattern\": *.md  }"
	got, changed := repairToolArgs(in)
	if !changed || got != "{\"pattern\": \"*.md\"  }" {
		t.Fatalf("got %q", got)
	}
	if strings.TrimSpace(got) == strings.TrimSpace(in) {
		t.Fatal("the bare value was not quoted")
	}
}
