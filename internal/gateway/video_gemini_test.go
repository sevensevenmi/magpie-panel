package gateway

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/yetone/magpie/internal/provider"
)

// geminiInteractionsVideos is a fake Gemini API serving the documented
// Interactions video flow: create returns an in-progress interaction, the
// first poll is still in progress, and the next answers the model_output
// video as base64.
type geminiInteractionsVideos struct {
	*httptest.Server
	mu     sync.Mutex
	paths  []string
	bodies []string
	posts  []http.Header
	polls  int
}

func newGeminiInteractionsVideos(t *testing.T) *geminiInteractionsVideos {
	t.Helper()
	video := []byte("\x00\x00\x00\x18ftypmp42gemini-omni-video")
	u := &geminiInteractionsVideos{}
	u.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		u.mu.Lock()
		defer u.mu.Unlock()
		u.paths = append(u.paths, r.Method+" "+r.URL.RequestURI())
		if r.Method == http.MethodPost {
			u.bodies = append(u.bodies, string(b))
			u.posts = append(u.posts, r.Header.Clone())
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1beta/openai/models":
			io.WriteString(w, `{"object":"list","data":[{"id":"gemini-omni-1.2-flash","object":"model"},{"id":"gemini-3.8-flash","object":"model"}]}`)
		case r.Method == http.MethodGet && r.URL.Path == "/v1beta/models":
			io.WriteString(w, `{"models":[{"name":"models/gemini-omni-1.2-flash","displayName":"Gemini Omni 1.2 Flash","supportedGenerationMethods":["generateContent"]},{"name":"models/gemini-3.8-flash","displayName":"Gemini 3.8 Flash","supportedGenerationMethods":["generateContent"]}]}`)
		case r.Method == http.MethodPost && r.URL.Path == "/v1beta/interactions":
			io.WriteString(w, `{"id":"v1_omni","object":"interaction","status":"in_progress","model":"gemini-omni-1.2-flash"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/v1beta/interactions/v1_omni":
			u.polls++
			if u.polls == 1 {
				io.WriteString(w, `{"id":"v1_omni","object":"interaction","status":"in_progress","model":"gemini-omni-1.2-flash","response_format":{"type":"video","duration":"4s","aspect_ratio":"16:9","resolution":"720p"}}`)
				return
			}
			io.WriteString(w, `{"id":"v1_omni","object":"interaction","status":"completed","model":"gemini-omni-1.2-flash","response_format":{"type":"video","duration":"4s","aspect_ratio":"16:9","resolution":"720p"},"steps":[{"type":"model_output","content":[{"type":"video","mime_type":"video/mp4","data":"`+base64.StdEncoding.EncodeToString(video)+`"}]}]}`)
		default:
			w.WriteHeader(http.StatusNotFound)
			io.WriteString(w, `{"error":{"code":"not_found","message":"no such path"}}`)
		}
	}))
	t.Cleanup(u.Close)
	return u
}

func (u *geminiInteractionsVideos) log() []string {
	u.mu.Lock()
	defer u.mu.Unlock()
	return slices.Clone(u.paths)
}

func TestGeminiOmniVideoUsesInteractions(t *testing.T) {
	fresh(t)
	up := newGeminiInteractionsVideos(t)
	p := provider.Provider{
		ID:     "google",
		Name:   "Google Gemini",
		Preset: "google",
		Chat:   up.URL + "/v1beta/openai",
		Gemini: up.URL + "/v1beta",
		Key:    "AIza-test",
		Models: []string{"gemini-omni-1.2-flash", "gemini-3.8-flash"},
	}
	p = saveFetched(t, p)
	var makers []string
	for _, model := range Videomakers(p) {
		makers = append(makers, model.ID)
	}
	if !slices.Equal(makers, []string{"gemini-omni-1.2-flash"}) {
		t.Fatalf("video makers: %v", makers)
	}

	h := New().Handler()
	image := base64.StdEncoding.EncodeToString([]byte("fake-png"))
	requestBody := map[string]any{
		"model":           "google/gemini-omni-1.2-flash",
		"prompt":          "a kite over a lake",
		"seconds":         4,
		"size":            "1280x720",
		"input_reference": "data:" + "image/png;base64," + image,
	}
	encoded, err := json.Marshal(requestBody)
	if err != nil {
		t.Fatal(err)
	}
	code, obj, raw := asKey(t, h, "", http.MethodPost, "/v1/videos", string(encoded))
	id, _ := obj["id"].(string)
	if code != http.StatusOK || id == "" || obj["model"] != "google/gemini-omni-1.2-flash" || obj["status"] != "in_progress" || obj["seconds"] != "4" {
		t.Fatalf("start: %d %s", code, raw)
	}

	up.mu.Lock()
	sentRaw := up.bodies[0]
	createHead := up.posts[0].Clone()
	up.mu.Unlock()
	var sent map[string]any
	if err := json.Unmarshal([]byte(sentRaw), &sent); err != nil {
		t.Fatal(err)
	}
	if sent["model"] != "gemini-omni-1.2-flash" || sent["background"] != true || sent["store"] != true || sent["stream"] != false {
		t.Fatalf("sent: %v", sent)
	}
	format, _ := sent["response_format"].(map[string]any)
	if format["type"] != "video" || format["duration"] != "4s" || format["aspect_ratio"] != "16:9" || format["resolution"] != "720p" {
		t.Fatalf("response_format: %v", format)
	}
	input, _ := sent["input"].([]any)
	if len(input) != 2 {
		t.Fatalf("input: %v", sent["input"])
	}
	picture, _ := input[0].(map[string]any)
	text, _ := input[1].(map[string]any)
	if picture["type"] != "image" || picture["mime_type"] != "image/png" || picture["data"] != image ||
		text["type"] != "text" || text["text"] != "a kite over a lake" {
		t.Fatalf("input: %v", input)
	}
	if createHead.Get("x-goog-api-key") != "AIza-test" || createHead.Get("Authorization") != "" {
		t.Fatalf("headers: %v", createHead)
	}

	if code, obj, raw = asKey(t, h, "", http.MethodGet, "/v1/videos/"+id, ""); code != http.StatusOK || obj["status"] != "in_progress" {
		t.Fatalf("first poll: %d %s", code, raw)
	}
	if code, obj, raw = asKey(t, h, "", http.MethodGet, "/v1/videos/"+id, ""); code != http.StatusOK || obj["status"] != "completed" || obj["seconds"] != "4" {
		t.Fatalf("second poll: %d %s", code, raw)
	}
	code, _, content, head := request(t, New(), http.MethodGet, "/v1/videos/"+id+"/content", "", "")
	if code != http.StatusOK || string(content) != "\x00\x00\x00\x18ftypmp42gemini-omni-video" || head.Get("Content-Type") != "video/mp4" {
		t.Fatalf("content: %d %q %v", code, content, head)
	}
	for _, path := range up.log() {
		if strings.Contains(path, "/videos") {
			t.Fatalf("Gemini Omni went to an OpenAI videos route: %v", up.log())
		}
	}
}

func TestGeminiOmniRefusesWhatTheModelCannotTake(t *testing.T) {
	for _, test := range []struct {
		name string
		f    filming
		want string
	}{
		{
			name: "a square",
			f:    filming{Prompt: "a kite", Size: "1024x1024"},
			want: "16:9 or 9:16",
		},
		{
			name: "too short",
			f:    filming{Prompt: "a kite", Seconds: "2"},
			want: "3 to 15",
		},
		{
			name: "an unsupported resolution",
			f:    filming{Prompt: "a kite", Extra: map[string]any{"resolution": "2k"}},
			want: "360p, 720p, 1080p or 4k",
		},
		{
			name: "too many references",
			f:    filming{Prompt: "a kite", References: make([]picture, 7)},
			want: "at most 6",
		},
		{
			name: "two frames too long",
			f: filming{
				Prompt:  "a kite",
				Seconds: "15",
				Start:   &picture{Mime: "image/png", Data: []byte("first")},
				End:     &picture{Mime: "image/png", Data: []byte("last")},
			},
			want: "at most 10 seconds",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := geminiOmniVideoBody("gemini-omni-1.2-flash", test.f); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestGeminiOmniMapsPortraitResolution(t *testing.T) {
	body, err := geminiOmniVideoBody("gemini-omni-1.2-flash", filming{
		Prompt:  "a kite",
		Seconds: "8",
		Size:    "720x1280",
		Extra:   map[string]any{"resolution": "1080p"},
	})
	if err != nil {
		t.Fatal(err)
	}
	var sent map[string]any
	if err := json.Unmarshal(body, &sent); err != nil {
		t.Fatal(err)
	}
	format, _ := sent["response_format"].(map[string]any)
	if format["duration"] != "8s" || format["aspect_ratio"] != "9:16" || format["resolution"] != "1080p" {
		t.Fatalf("response_format: %v", format)
	}
}

func TestGeminiOmniReadsTheLastVideoAndNeedsItsData(t *testing.T) {
	last, err := geminiVideoState([]byte(`{
		"id":"v1_last",
		"status":"completed",
		"model":"gemini-omni-1.2-flash",
		"steps":[
			{"type":"model_output","content":[{"type":"video","mime_type":"video/mp4","data":"Zmlyc3Q="}]},
			{"type":"model_output","content":[{"type":"video","mime_type":"video/mp4","data":"bGFzdA=="}]}
		]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if last.state.Status != "done" || last.data != "bGFzdA==" || last.mime != "video/mp4" {
		t.Fatalf("last video: %+v", last)
	}

	missing, err := geminiVideoState([]byte(`{"id":"v1_missing","status":"completed","model":"gemini-omni-1.2-flash","steps":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	if missing.state.Status != "failed" || missing.state.Error.Code != "missing_video" {
		t.Fatalf("completed without video data: %+v", missing)
	}
}
