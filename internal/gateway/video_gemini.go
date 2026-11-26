package gateway

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/yetone/magpie/internal/catalog"
	"github.com/yetone/magpie/internal/provider"
)

// Gemini Omni makes videos through the Interactions API:
// POST <Gemini base>/interactions starts one in the background, and
// GET <Gemini base>/interactions/<id> says how it goes and carries the
// video as base64 in a model_output step's video content.

const geminiOmniFamily = "gemini-omni-"

// geminiInteractions is whether p is Google's Gemini API reached with an
// API key, where Gemini Omni's videos are made. A subscription account, a
// plugin and Vertex AI have their own paths.
func geminiInteractions(p provider.Provider) bool {
	return p.Account == nil && !p.IsRemoteMagpie() && !p.IsPlugin() && !p.IsVertex() &&
		p.Base(provider.Gemini) != "" && (p.Preset == "google" || isGoogle(p))
}

func geminiOmniModel(model string) bool {
	return strings.HasPrefix(strings.TrimPrefix(model, "models/"), geminiOmniFamily)
}

// makesGeminiOmniVideo is whether p makes model through the Interactions
// API. The id is checked as well as the provider so a Google key's other
// models keep their own routes.
func makesGeminiOmniVideo(p provider.Provider, model string) bool {
	return geminiInteractions(p) && geminiOmniModel(model)
}

// geminiOmniVideomakersOf adds the Omni models a Gemini API key lists to
// its video makers. Gemini's model list doesn't say which output modality
// a model makes, so the documented gemini-omni-* ids are the evidence.
func geminiOmniVideomakersOf(p provider.Provider, avail func() []catalog.Model) []catalog.Model {
	if !geminiInteractions(p) {
		return nil
	}
	var out []catalog.Model
	seen := map[string]bool{}
	add := func(m catalog.Model) {
		id := strings.TrimPrefix(strings.TrimSpace(m.ID), "models/")
		if !geminiOmniModel(id) || seen[id] {
			return
		}
		seen[id] = true
		m.ID, m.Provider, m.Films = id, p.ID, true
		if m.Name == "" {
			m.Name = id
		}
		out = append(out, m)
	}
	if avail != nil {
		for _, m := range avail() {
			add(m)
		}
	}
	for _, id := range p.Models {
		add(catalog.Model{ID: id, Name: id})
	}
	return out
}

// geminiInteractionsURL is where p creates an Interaction.
func geminiInteractionsURL(p provider.Provider) string {
	return strings.TrimRight(p.Base(provider.Gemini), "/") + "/interactions"
}

// geminiInteractionURL is where p says how one is going. stream=false asks
// for the whole stored Interaction rather than an event stream.
func geminiInteractionURL(p provider.Provider, id string) string {
	return geminiInteractionsURL(p) + "/" + url.PathEscape(id) + "?stream=false"
}

// geminiVideoID is the id of a video p started as an Interaction. The
// Interaction id is base64url so it fits between the dots of magpie's video
// id, and the key's mark keeps every poll on the key that started it.
func geminiVideoID(p provider.Provider, interaction string, t time.Time) string {
	theirs := "go_" + base64.RawURLEncoding.EncodeToString([]byte(interaction))
	return "video_" + p.ID + "." + theirs + "." + strconv.FormatInt(t.Unix(), 10) + "." + keyMark(p)
}

// geminiInteractionID is the vendor's Interaction id inside a video id.
func geminiInteractionID(vendorID string) (string, bool) {
	rest, ok := strings.CutPrefix(vendorID, "go_")
	if !ok {
		return "", false
	}
	b, err := base64.RawURLEncoding.DecodeString(rest)
	if err != nil || len(b) == 0 {
		return "", false
	}
	return string(b), true
}

// geminiOmniVideoBody is f as a Gemini Omni Interaction takes it. It keeps
// the prompt and images in the documented input order, and maps the videos
// route's seconds and size onto response_format's duration, aspect ratio and
// resolution. A request the model can't take is refused before Google is
// asked.
func geminiOmniVideoBody(model string, f filming) ([]byte, error) {
	if len(f.References) > 6 {
		return nil, fmt.Errorf("Gemini Omni takes at most 6 reference images, not %d", len(f.References))
	}
	seconds := 0
	if f.Seconds != "" {
		var err error
		seconds, err = strconv.Atoi(f.Seconds)
		if err != nil || seconds < 3 || seconds > 15 {
			return nil, fmt.Errorf("Gemini Omni takes a whole duration from 3 to 15 seconds, not %q", f.Seconds)
		}
	}
	if f.Start != nil && f.End != nil && seconds > 10 {
		return nil, errors.New("Gemini Omni's first and last frame interpolation takes at most 10 seconds")
	}

	input := make([]map[string]any, 0, 2+len(f.References))
	addPicture := func(pic picture) {
		input = append(input, geminiImageContent(pic))
	}
	if f.Start != nil {
		addPicture(*f.Start)
	}
	if f.End != nil {
		addPicture(*f.End)
	}
	for _, pic := range f.References {
		addPicture(pic)
	}
	if strings.TrimSpace(f.Prompt) != "" {
		input = append(input, map[string]any{"type": "text", "text": f.Prompt})
	}

	req := map[string]any{
		"model":      model,
		"background": true,
		"store":      true,
		"stream":     false,
	}
	if len(input) == 1 && input[0]["type"] == "text" {
		req["input"] = f.Prompt
	} else if len(input) > 0 {
		req["input"] = input
	}

	format := map[string]any{"type": "video"}
	if seconds > 0 {
		format["duration"] = strconv.Itoa(seconds) + "s"
	}
	ratio, resolution, err := geminiVideoShape(f)
	if err != nil {
		return nil, err
	}
	if ratio != "" {
		format["aspect_ratio"] = ratio
	}
	if resolution != "" {
		format["resolution"] = resolution
	}
	req["response_format"] = format
	if previous, ok := f.Extra["previous_interaction_id"].(string); ok && strings.TrimSpace(previous) != "" {
		req["previous_interaction_id"] = strings.TrimSpace(previous)
	}
	return json.Marshal(req)
}

func geminiImageContent(pic picture) map[string]any {
	if pic.URL != "" {
		return map[string]any{"type": "image", "uri": pic.URL}
	}
	mimeType := pic.Mime
	if mimeType == "" {
		mimeType = http.DetectContentType(pic.Data)
	}
	return map[string]any{
		"type":      "image",
		"data":      base64.StdEncoding.EncodeToString(pic.Data),
		"mime_type": mimeType,
	}
}

// geminiVideoShape maps the videos route's size and the caller's own
// aspect_ratio and resolution fields to Gemini Omni's documented values.
func geminiVideoShape(f filming) (ratio, resolution string, err error) {
	if r, ok := f.Extra["aspect_ratio"].(string); ok && strings.TrimSpace(r) != "" {
		ratio = strings.ToLower(strings.TrimSpace(r))
		if ratio != "16:9" && ratio != "9:16" {
			return "", "", fmt.Errorf("Gemini Omni takes aspect_ratio 16:9 or 9:16, not %q", r)
		}
	} else if strings.TrimSpace(f.Size) != "" {
		ratio, err = geminiAspect(f.Size)
		if err != nil {
			return "", "", err
		}
	}

	if r, ok := f.Extra["resolution"].(string); ok && strings.TrimSpace(r) != "" {
		resolution = strings.ToLower(strings.TrimSpace(r))
		switch resolution {
		case "360p", "720p", "1080p", "4k":
		default:
			return "", "", fmt.Errorf("Gemini Omni takes resolution 360p, 720p, 1080p or 4k, not %q", r)
		}
	} else if strings.TrimSpace(f.Size) != "" {
		resolution, err = geminiResolution(f.Size)
		if err != nil {
			return "", "", err
		}
	}
	return ratio, resolution, nil
}

func geminiAspect(size string) (string, error) {
	size = strings.ToLower(strings.TrimSpace(size))
	if size == "16:9" || size == "9:16" {
		return size, nil
	}
	w, h, ok := strings.Cut(size, "x")
	if !ok {
		return "", fmt.Errorf("Gemini Omni needs a 16:9 or 9:16 size, not %q", size)
	}
	width, errW := strconv.ParseFloat(w, 64)
	height, errH := strconv.ParseFloat(h, 64)
	if errW != nil || errH != nil || width <= 0 || height <= 0 {
		return "", fmt.Errorf("Gemini Omni needs a 16:9 or 9:16 size, not %q", size)
	}
	value := width / height
	switch {
	case math.Abs(value-16.0/9.0) < 0.001:
		return "16:9", nil
	case math.Abs(value-9.0/16.0) < 0.001:
		return "9:16", nil
	}
	return "", fmt.Errorf("Gemini Omni takes a 16:9 or 9:16 video, not size %q", size)
}

func geminiResolution(size string) (string, error) {
	size = strings.ToLower(strings.TrimSpace(size))
	if size == "360p" || size == "720p" || size == "1080p" || size == "4k" {
		return size, nil
	}
	w, h, ok := strings.Cut(size, "x")
	if !ok {
		return "", nil
	}
	width, errW := strconv.ParseFloat(w, 64)
	height, errH := strconv.ParseFloat(h, 64)
	if errW != nil || errH != nil || width <= 0 || height <= 0 {
		return "", nil
	}
	short := math.Min(width, height)
	switch {
	case short >= 2160:
		return "4k", nil
	case short >= 1080:
		return "1080p", nil
	case short >= 720:
		return "720p", nil
	case short >= 360:
		return "360p", nil
	}
	return "", fmt.Errorf("Gemini Omni's smallest resolution is 360p, not size %q", size)
}

type geminiInteraction struct {
	Status string `json:"status"`
	Model  string `json:"model"`
	Steps  []struct {
		Type    string `json:"type"`
		Content []struct {
			Type     string `json:"type"`
			MimeType string `json:"mime_type"`
			Data     string `json:"data"`
		} `json:"content"`
	} `json:"steps"`
	ResponseFormat json.RawMessage `json:"response_format"`
	Error          struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
	Errors []struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"errors"`
}

type geminiVideoResult struct {
	state videoState
	data  string
	mime  string
}

// geminiVideoState reads the status and the last model_output video from an
// Interaction answer. The video can be in any model_output step, so no step
// index is assumed.
func geminiVideoState(b []byte) (geminiVideoResult, error) {
	var in geminiInteraction
	if err := json.Unmarshal(b, &in); err != nil {
		return geminiVideoResult{}, fmt.Errorf("the answer isn't a Gemini Interaction: %v", err)
	}
	var out geminiVideoResult
	out.state.Model = strings.TrimPrefix(in.Model, "models/")
	switch in.Status {
	case "completed":
		out.state.Status, out.state.Progress = "done", 100
	case "in_progress":
		out.state.Status = "running"
	case "queued":
		out.state.Status = "queued"
	case "failed", "incomplete", "budget_exceeded":
		out.state.Status = "failed"
	case "cancelled":
		out.state.Status = "cancelled"
	case "requires_action", "requires_continuation":
		out.state.Status = "failed"
		out.state.Error.Message = "Gemini Omni needs another interaction to finish this video"
	default:
		out.state.Status = in.Status
	}
	if in.Error.Message != "" {
		out.state.Error.Code, out.state.Error.Message = in.Error.Code, in.Error.Message
	}
	if out.state.Error.Message == "" && len(in.Errors) > 0 {
		out.state.Error.Code, out.state.Error.Message = in.Errors[0].Code, in.Errors[0].Message
	}
	if duration := geminiResponseDuration(in.ResponseFormat); duration > 0 {
		out.state.Duration = duration
	}
	for _, step := range in.Steps {
		if step.Type != "model_output" {
			continue
		}
		for _, content := range step.Content {
			if content.Type != "video" {
				continue
			}
			out.data, out.mime = content.Data, content.MimeType
		}
	}
	if in.Status == "completed" && out.data == "" {
		out.state.Status = "failed"
		out.state.Progress = 0
		out.state.Error.Code = "missing_video"
		out.state.Error.Message = "Gemini Omni completed the interaction without video data"
	}
	return out, nil
}

func geminiResponseDuration(raw json.RawMessage) float64 {
	if len(raw) == 0 {
		return 0
	}
	var one struct {
		Duration string `json:"duration"`
	}
	if json.Unmarshal(raw, &one) == nil {
		return secondsOf(one.Duration)
	}
	var many []struct {
		Duration string `json:"duration"`
	}
	if json.Unmarshal(raw, &many) == nil {
		for _, format := range many {
			if d := secondsOf(format.Duration); d > 0 {
				return d
			}
		}
	}
	return 0
}

func secondsOf(duration string) float64 {
	duration = strings.TrimSuffix(strings.TrimSpace(duration), "s")
	if duration == "" {
		return 0
	}
	n, _ := strconv.ParseFloat(duration, 64)
	return n
}

func (s *Server) geminiVideoStatus(ctx context.Context, p provider.Provider, interaction string) (geminiVideoResult, int, error) {
	b, code, err := s.sendAs(ctx, p, http.MethodGet, geminiInteractionURL(p, interaction), "", nil, false)
	if err != nil {
		return geminiVideoResult{}, code, err
	}
	out, err := geminiVideoState(b)
	if err != nil {
		return geminiVideoResult{}, http.StatusBadGateway, fmt.Errorf("%s: %v", p.Name, err)
	}
	return out, code, nil
}

// geminiVideoContent serves the MP4 from a completed Interaction's inline
// base64 data, as GET /v1beta/interactions/{id} returns it.
func (s *Server) geminiVideoContent(r *http.Request, w http.ResponseWriter, p provider.Provider, interaction string) {
	ctx, cancel := drawWithin(r.Context(), videoTimeout)
	defer cancel()
	out, code, err := s.geminiVideoStatus(ctx, p, interaction)
	if err != nil {
		writeError(w, provider.Chat, code, err.Error())
		return
	}
	if held := videoModelHeld(r, p, out.state.Model); held != "" {
		writeError(w, provider.Chat, http.StatusForbidden, held)
		return
	}
	if out.state.Status != "done" && out.state.Status != "succeeded" {
		obj := videoObject("", "", time.Time{}, out.state, filming{})
		msg := fmt.Sprintf("the video isn't ready: it is %v", obj["status"])
		if e, ok := obj["error"].(map[string]string); ok {
			msg += ": " + e["message"]
		}
		writeError(w, provider.Chat, http.StatusConflict, msg)
		return
	}
	if out.data != "" {
		decoder := base64.NewDecoder(base64.StdEncoding, strings.NewReader(out.data))
		video, err := io.ReadAll(io.LimitReader(decoder, maxVideoBytes+1))
		if err != nil {
			writeError(w, provider.Chat, http.StatusBadGateway, "Gemini Omni's video isn't base64: "+err.Error())
			return
		}
		if len(video) > maxVideoBytes {
			writeError(w, provider.Chat, http.StatusBadGateway, "Gemini Omni's video is larger than magpie passes on")
			return
		}
		contentType := out.mime
		if !strings.HasPrefix(strings.ToLower(contentType), "video/") {
			contentType = "video/mp4"
		}
		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Content-Length", strconv.Itoa(len(video)))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(video)
		return
	}
	writeError(w, provider.Chat, http.StatusBadGateway, p.Name+" completed the video but gave no video data")
}
