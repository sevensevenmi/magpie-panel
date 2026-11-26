package gateway

import (
	"encoding/json"
	"strconv"
	"strings"
)

// toolArgsRepair repairs the one malformed JSON shape reported in #7772:
// a model writes a bare string where JSON requires a quoted value. It
// follows the arguments' own JSON structure, so commas inside a brace or
// bracket glob stay part of the bare value. Values already beginning with
// a JSON string, object or array pass through without buffering.
type toolArgsRepair struct {
	mode     toolArgsMode
	stack    []byte // the open containers' closing bytes
	key      bool   // the open string or bare token is an object key
	escaped  bool
	raw      strings.Builder
	rawDepth int
}

type toolArgsMode uint8

const (
	toolArgsValue toolArgsMode = iota
	toolArgsArrayValue
	toolArgsObjectKey
	toolArgsColon
	toolArgsAfterValue
	toolArgsString
	toolArgsRaw
)

// feed reads one arguments fragment and gives back the part that can go
// on now. A bare token is held until its top-level delimiter, which may be
// in a later fragment.
func (r *toolArgsRepair) feed(s string) string {
	var out strings.Builder
	out.Grow(len(s))
	for i := 0; i < len(s); {
		c := s[i]
		switch r.mode {
		case toolArgsString:
			out.WriteByte(c)
			i++
			switch {
			case r.escaped:
				r.escaped = false
			case c == '\\':
				r.escaped = true
			case c == '"':
				if r.key {
					r.mode = toolArgsColon
				} else {
					r.mode = toolArgsAfterValue
				}
			}
		case toolArgsRaw:
			if r.rawDepth == 0 && (c == ',' || c == '}' || c == ']' || r.key && c == ':') {
				out.WriteString(r.endRaw())
				continue
			}
			r.raw.WriteByte(c)
			i++
			switch c {
			case '{', '[':
				r.rawDepth++
			case '}', ']':
				if r.rawDepth > 0 {
					r.rawDepth--
				}
			}
		default:
			switch r.mode {
			case toolArgsValue:
				switch {
				case isJSONSpace(c):
					out.WriteByte(c)
					i++
				case c == '"':
					out.WriteByte(c)
					i++
					r.mode, r.key, r.escaped = toolArgsString, false, false
				case c == '{':
					out.WriteByte(c)
					i++
					r.stack = append(r.stack, '}')
					r.mode = toolArgsObjectKey
				case c == '[':
					out.WriteByte(c)
					i++
					r.stack = append(r.stack, ']')
					r.mode = toolArgsArrayValue
				default:
					r.beginRaw(false)
				}
			case toolArgsArrayValue:
				switch {
				case isJSONSpace(c):
					out.WriteByte(c)
					i++
				case c == ']' && r.closes(']'):
					out.WriteByte(c)
					i++
					r.stack = r.stack[:len(r.stack)-1]
					r.mode = toolArgsAfterValue
				case c == '"':
					out.WriteByte(c)
					i++
					r.mode, r.key, r.escaped = toolArgsString, false, false
				case c == '{':
					out.WriteByte(c)
					i++
					r.stack = append(r.stack, '}')
					r.mode = toolArgsObjectKey
				case c == '[':
					out.WriteByte(c)
					i++
					r.stack = append(r.stack, ']')
					r.mode = toolArgsArrayValue
				default:
					r.beginRaw(false)
				}
			case toolArgsObjectKey:
				switch {
				case isJSONSpace(c):
					out.WriteByte(c)
					i++
				case c == '}' && r.closes('}'):
					out.WriteByte(c)
					i++
					r.stack = r.stack[:len(r.stack)-1]
					r.mode = toolArgsAfterValue
				case c == '"':
					out.WriteByte(c)
					i++
					r.mode, r.key, r.escaped = toolArgsString, true, false
				default:
					r.beginRaw(true)
				}
			case toolArgsColon:
				out.WriteByte(c)
				i++
				if c == ':' {
					r.mode = toolArgsValue
				}
			case toolArgsAfterValue:
				switch {
				case isJSONSpace(c):
					out.WriteByte(c)
					i++
				case c == ',':
					out.WriteByte(c)
					i++
					if len(r.stack) > 0 && r.stack[len(r.stack)-1] == '}' {
						r.mode = toolArgsObjectKey
					} else {
						r.mode = toolArgsArrayValue
					}
				case c == '}' && r.closes('}'):
					out.WriteByte(c)
					i++
					r.stack = r.stack[:len(r.stack)-1]
				case c == ']' && r.closes(']'):
					out.WriteByte(c)
					i++
					r.stack = r.stack[:len(r.stack)-1]
				default:
					out.WriteByte(c)
					i++
				}
			}
		}
	}
	return out.String()
}

// finish gives back a bare token still held when the arguments end.
func (r *toolArgsRepair) finish() string {
	if r.mode != toolArgsRaw {
		return ""
	}
	return r.endRaw()
}

func (r *toolArgsRepair) beginRaw(key bool) {
	r.mode, r.key = toolArgsRaw, key
	r.raw.Reset()
	r.rawDepth = 0
}

func (r *toolArgsRepair) endRaw() string {
	raw := r.raw.String()
	trimmed := strings.TrimSpace(raw)
	suffix := raw[len(trimmed):]
	r.raw.Reset()
	r.rawDepth = 0
	r.mode = toolArgsAfterValue
	if r.key {
		r.mode = toolArgsColon
	}
	if r.key || !json.Valid([]byte(trimmed)) {
		quoted, _ := json.Marshal(trimmed)
		return string(quoted) + suffix
	}
	return raw
}

func (r *toolArgsRepair) closes(c byte) bool {
	return len(r.stack) > 0 && r.stack[len(r.stack)-1] == c
}

// repairToolArgs repairs a complete arguments string. A valid string
// comes back unchanged without entering the state machine.
func repairToolArgs(s string) (string, bool) {
	if json.Valid([]byte(s)) {
		return s, false
	}
	var r toolArgsRepair
	out := r.feed(s) + r.finish()
	return out, out != s
}

// repairChatToolArgs repairs the streamed calls in one Chat delta and
// reports whether the calls changed.
func (t *chatTidy) repairChatToolArgs(choice int, raw json.RawMessage) (json.RawMessage, bool) {
	var calls []map[string]json.RawMessage
	if decodeJSON(raw, &calls) != nil {
		return raw, false
	}
	changed := false
	for _, call := range calls {
		var index int
		if decodeJSON(call["index"], &index) != nil {
			continue
		}
		var function map[string]json.RawMessage
		if decodeJSON(call["function"], &function) != nil {
			continue
		}
		var arguments string
		if decodeJSON(function["arguments"], &arguments) != nil {
			continue
		}
		key := [2]int{choice, index}
		r := t.toolArgs[key]
		if r == nil {
			r = &toolArgsRepair{}
			if t.toolArgs == nil {
				t.toolArgs = map[[2]int]*toolArgsRepair{}
			}
			t.toolArgs[key] = r
		}
		mended := r.feed(arguments)
		if mended == arguments {
			continue
		}
		function["arguments"], _ = json.Marshal(mended)
		call["function"], _ = json.Marshal(function)
		changed = true
	}
	if !changed {
		return raw, false
	}
	out, err := json.Marshal(calls)
	if err != nil {
		return raw, false
	}
	return out, true
}

// finishChatToolArgs gives a call's held bare token to a fragment that
// ends the stream. Normally the closing brace or comma carries it; this
// covers a reply that ends immediately after the token.
func (t *chatTidy) finishChatToolArgs(choice int, delta map[string]json.RawMessage, raw json.RawMessage) (json.RawMessage, bool) {
	var calls []map[string]json.RawMessage
	if len(raw) > 0 {
		decodeJSON(raw, &calls)
	}
	changed := false
	for key, r := range t.toolArgs {
		if key[0] != choice {
			continue
		}
		tail := r.finish()
		delete(t.toolArgs, key)
		if tail == "" {
			continue
		}
		found := false
		for _, call := range calls {
			var index int
			if decodeJSON(call["index"], &index) != nil || index != key[1] {
				continue
			}
			var function map[string]json.RawMessage
			if decodeJSON(call["function"], &function) != nil {
				continue
			}
			var arguments string
			decodeJSON(function["arguments"], &arguments)
			function["arguments"], _ = json.Marshal(arguments + tail)
			call["function"], _ = json.Marshal(function)
			found = true
			break
		}
		if !found {
			call := map[string]json.RawMessage{"index": json.RawMessage(strconv.Itoa(key[1]))}
			function, _ := json.Marshal(map[string]string{"arguments": tail})
			call["function"] = function
			calls = append(calls, call)
		}
		changed = true
	}
	if !changed {
		return raw, false
	}
	out, err := json.Marshal(calls)
	if err != nil {
		return raw, false
	}
	delta["tool_calls"] = out
	return out, true
}

// repairWholeChatToolArgs repairs the calls of a non-streamed Chat reply.
func repairWholeChatToolArgs(msg map[string]json.RawMessage) bool {
	var calls []map[string]json.RawMessage
	if decodeJSON(msg["tool_calls"], &calls) != nil {
		return false
	}
	changed := false
	for _, call := range calls {
		var function map[string]json.RawMessage
		if decodeJSON(call["function"], &function) != nil {
			continue
		}
		var arguments string
		if decodeJSON(function["arguments"], &arguments) != nil {
			continue
		}
		mended, ok := repairToolArgs(arguments)
		if !ok {
			continue
		}
		function["arguments"], _ = json.Marshal(mended)
		call["function"], _ = json.Marshal(function)
		changed = true
	}
	if !changed {
		return false
	}
	msg["tool_calls"], _ = json.Marshal(calls)
	return true
}
