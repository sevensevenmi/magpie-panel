package gateway

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const agyTestPNG = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAusB9Y9Zl1sAAAAASUVORK5CYII="

func TestAGYPluginInlinesURLImages(t *testing.T) {
	png, err := base64.StdEncoding.DecodeString(agyTestPNG)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(png)
	}))
	defer server.Close()
	old := publicClient
	publicClient = server.Client()
	t.Cleanup(func() { publicClient = old })

	req := &Request{Messages: []Message{
		{Role: "user", Parts: []Part{{Kind: Image, URL: server.URL + "/user.png"}}},
		{Role: "user", Parts: []Part{{Kind: ToolResult, Images: []Part{{Kind: Image, URL: server.URL + "/tool.png"}}}}},
	}}
	got, err := inlinePluginURLImages(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if req.Messages[0].Parts[0].Data != "" || req.Messages[1].Parts[0].Images[0].Data != "" {
		t.Fatal("the original request was changed")
	}
	for _, part := range []Part{got.Messages[0].Parts[0], got.Messages[1].Parts[0].Images[0]} {
		if part.MediaType != "image/png" || part.Data != agyTestPNG {
			t.Errorf("inlined image = %s %d bytes", part.MediaType, len(part.Data))
		}
	}
	body := string(anthropicBody(got, "gemini-4-hydrogen", "", false))
	if strings.Contains(body, `"type":"url"`) || strings.Count(body, `"type":"base64"`) != 2 {
		t.Errorf("Messages body did not inline both images: %s", body)
	}
}

func TestAGYPluginURLImageGuardAndContentType(t *testing.T) {
	old := publicClient
	publicClient = guardedClient()
	t.Cleanup(func() { publicClient = old })
	if _, err := inlinePluginURLImages(context.Background(), &Request{Messages: []Message{{Role: "user", Parts: []Part{{Kind: Image, URL: "http://127.0.0.1:1/private.png"}}}}}); err == nil {
		t.Fatal("a loopback image URL was fetched")
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("not an image"))
	}))
	defer server.Close()
	publicClient = server.Client()
	_, err := inlinePluginURLImages(context.Background(), &Request{Messages: []Message{{Role: "user", Parts: []Part{{Kind: Image, URL: server.URL + "/text"}}}}})
	if err == nil || !strings.Contains(err.Error(), "isn't an image") {
		t.Fatalf("non-image URL error = %v", err)
	}
}
