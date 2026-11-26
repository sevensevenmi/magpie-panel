package gateway

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/yetone/magpie/internal/provider"
)

// chatArgsLine is one Chat stream chunk carrying a tool-call argument
// fragment. The arguments string is the model's own text, which may not be
// valid JSON yet.
func chatArgsLine(t *testing.T, arguments string) string {
	t.Helper()
	line, err := json.Marshal(map[string]any{
		"id":     "c1",
		"object": "chat.completion.chunk",
		"choices": []any{map[string]any{
			"index": 0,
			"delta": map[string]any{
				"tool_calls": []any{map[string]any{
					"index": 0,
					"id":    "call_1",
					"type":  "function",
					"function": map[string]any{
						"name":      "ops_glob",
						"arguments": arguments,
					},
				}},
			},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return "data: " + string(line)
}

func sameToolArgsJSON(t *testing.T, got, want string) {
	t.Helper()
	if !json.Valid([]byte(got)) {
		t.Fatalf("tool arguments are not JSON: %s", got)
	}
	var g, w any
	if err := json.Unmarshal([]byte(got), &g); err != nil {
		t.Fatalf("tool arguments: %v: %s", err, got)
	}
	if err := json.Unmarshal([]byte(want), &w); err != nil {
		t.Fatalf("want: %v: %s", err, want)
	}
	if !reflect.DeepEqual(g, w) {
		t.Fatalf("tool arguments %s, want %s", got, want)
	}
}

// #7772: glm-5.3-flash and deepseek-v4.1-flash omit quotes around some
// string values. A Chat client concatenates the arguments fragments and
// parses them as JSON, so every fragment must still add up to valid JSON.
func TestChatRelayRepairsUnquotedToolArgs(t *testing.T) {
	for _, c := range []struct {
		name      string
		fragments []string
		want      string
	}{
		{
			name:      "bare glob",
			fragments: []string{`{"pattern": *.md, "path": "C:/path/to/dir"}`},
			want:      `{"pattern":"*.md","path":"C:/path/to/dir"}`,
		},
		{
			name:      "fragmented path",
			fragments: []string{"", `{"pattern": `, "changes/*/", "pl", "anning-context.md", "}"},
			want:      `{"pattern":"changes/*/planning-context.md"}`,
		},
		{
			name:      "windows path",
			fragments: []string{`{"pattern": C:\Users\example\.agents\skills\truthguard\**}`},
			want:      `{"pattern":"C:\\Users\\example\\.agents\\skills\\truthguard\\**"}`,
		},
		{
			name:      "brace glob comma",
			fragments: []string{`{"pattern": *.{py,md,sh}, "path": "src"}`},
			want:      `{"pattern":"*.{py,md,sh}","path":"src"}`,
		},
		{
			name:      "array value",
			fragments: []string{`{"patterns":[*.md, src/*.{go,ts}],"path":"src"}`},
			want:      `{"patterns":["*.md","src/*.{go,ts}"],"path":"src"}`,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			fresh(t)
			lines := make([]string, 0, len(c.fragments)+2)
			for _, fragment := range c.fragments {
				lines = append(lines, chatArgsLine(t, fragment))
			}
			lines = append(lines,
				`data: {"id":"c1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`,
				`data: [DONE]`,
			)
			f := &fake{t: t, reply: sse(lines...)}
			up := setup(t, provider.Chat, f)
			crModel(t, provider.Chat, up, "glm-5.3-flash")

			code, body := post(t, "/v1/chat/completions",
				`{"model":"glm-5.3-flash","stream":true,"messages":[{"role":"user","content":"hi"}]}`)
			if code != 200 {
				t.Fatalf("status %d: %s", code, body)
			}
			sameToolArgsJSON(t, crChatArgs(t, body), c.want)
		})
	}
}

// Valid arguments pass through exactly, including across fragments. This
// is the zero-buffering path for values that already begin as JSON.
func TestChatRelayKeepsValidToolArgs(t *testing.T) {
	fresh(t)
	fragments := []string{
		`{"pattern":"*.md","limit":2,`,
		`"recursive":true,"nested":{"x":null},"items":["a","b"]}`,
	}
	lines := make([]string, 0, len(fragments)+2)
	for _, fragment := range fragments {
		lines = append(lines, chatArgsLine(t, fragment))
	}
	lines = append(lines,
		`data: {"id":"c1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`,
		`data: [DONE]`,
	)
	f := &fake{t: t, reply: sse(lines...)}
	up := setup(t, provider.Chat, f)
	crModel(t, provider.Chat, up, "glm-5.3-flash")

	code, body := post(t, "/v1/chat/completions",
		`{"model":"glm-5.3-flash","stream":true,"messages":[{"role":"user","content":"hi"}]}`)
	if code != 200 {
		t.Fatalf("status %d: %s", code, body)
	}
	if got, want := crChatArgs(t, body), strings.Join(fragments, ""); got != want {
		t.Fatalf("valid arguments changed: %s, want %s", got, want)
	}
}

// The same Chat upstream can serve a Responses client through a
// translation. That decoder must repair the arguments before the encoder
// turns them into Responses deltas.
func TestChatUpstreamRepairsUnquotedToolArgsForResponses(t *testing.T) {
	fresh(t)
	fragments := []string{"", `{"pattern": `, "changes/*/", "pl", "anning-context.md", "}"}
	lines := make([]string, 0, len(fragments)+2)
	for _, fragment := range fragments {
		lines = append(lines, chatArgsLine(t, fragment))
	}
	lines = append(lines,
		`data: {"id":"c1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`,
		`data: [DONE]`,
	)
	f := &fake{t: t, reply: sse(lines...)}
	up := setup(t, provider.Chat, f)
	crModel(t, provider.Chat, up, "glm-5.3-flash")

	code, body := post(t, "/v1/responses",
		`{"model":"glm-5.3-flash","stream":true,"input":"hi"}`)
	if code != 200 {
		t.Fatalf("status %d: %s", code, body)
	}
	sameToolArgsJSON(t, responsesArgs(t, body), `{"pattern":"changes/*/planning-context.md"}`)
}

// A non-streamed Chat reply carries the same malformed arguments string.
func TestChatReplyRepairsUnquotedToolArgs(t *testing.T) {
	fresh(t)
	reply := map[string]any{
		"id":     "c1",
		"object": "chat.completion",
		"model":  "glm-5.3-flash",
		"choices": []any{map[string]any{
			"index": 0,
			"message": map[string]any{
				"role":    "assistant",
				"content": nil,
				"tool_calls": []any{map[string]any{
					"id":   "call_1",
					"type": "function",
					"function": map[string]any{
						"name":      "ops_glob",
						"arguments": `{"pattern": *.{py,md,sh}, "path": C:\Users\example\.agents}`,
					},
				}},
			},
			"finish_reason": "tool_calls",
		}},
	}
	raw, err := json.Marshal(reply)
	if err != nil {
		t.Fatal(err)
	}
	f := &fake{t: t, ctype: "application/json", reply: string(raw)}
	up := setup(t, provider.Chat, f)
	crModel(t, provider.Chat, up, "glm-5.3-flash")

	code, body := post(t, "/v1/chat/completions",
		`{"model":"glm-5.3-flash","messages":[{"role":"user","content":"hi"}]}`)
	if code != 200 {
		t.Fatalf("status %d: %s", code, body)
	}
	var got struct {
		Choices []struct {
			Message struct {
				ToolCalls []struct {
					Function struct {
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatalf("reply: %v: %s", err, body)
	}
	if len(got.Choices) == 0 || len(got.Choices[0].Message.ToolCalls) == 0 {
		t.Fatalf("no tool call in %s", body)
	}
	sameToolArgsJSON(t, got.Choices[0].Message.ToolCalls[0].Function.Arguments,
		`{"pattern":"*.{py,md,sh}","path":"C:\\Users\\example\\.agents"}`)
}
