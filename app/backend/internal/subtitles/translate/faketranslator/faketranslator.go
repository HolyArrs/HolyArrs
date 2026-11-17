// Package faketranslator holds stand-in translators for tests: a small
// server with the OpenAI chat API shape and one with the Lingarr content
// route. They "translate" by putting a marker in front of each line, so a
// test can see which lines came back. Nothing here calls a real service.
package faketranslator

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
)

type line struct {
	N    int    `json:"n"`
	Text string `json:"text"`
}

// OpenAI answers /chat/completions and /models like an OpenAI-style server.
type OpenAI struct {
	mu sync.Mutex
	// Key, when set, must arrive as a Bearer token.
	Key string
	// Prefix goes in front of every translated line; "PT: " when empty.
	Prefix string
	// Drop leaves these line numbers out of the next answers, once each.
	Drop map[int]int
	// Extra adds a line with a number nobody asked for.
	Extra bool
	// Fence wraps the JSON answer in a code fence.
	Fence bool
	// Busy answers this many calls with 429 first.
	Busy int
	// Fail, when set, is the status of every answer.
	Fail int
	// CutOver cuts every answer with more lines than this in the middle, as a
	// model that ran out of room does.
	CutOver int
	// Calls counts chat calls; Sizes holds the line count of each.
	Calls int
	Sizes []int
	// Models are the names /models lists.
	Models []string
	// Seen holds the last system and user messages.
	System, User string
}

func (o *OpenAI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.Key != "" && r.Header.Get("Authorization") != "Bearer "+o.Key {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": map[string]any{"message": "Incorrect API key provided: " + r.Header.Get("Authorization")}})
		return
	}
	if o.Fail != 0 {
		writeJSON(w, o.Fail, map[string]any{"error": map[string]any{"message": "model \"nope\" not found"}})
		return
	}
	switch {
	case strings.HasSuffix(r.URL.Path, "/models"):
		data := []any{}
		for _, m := range o.Models {
			data = append(data, map[string]any{"id": m, "object": "model"})
		}
		writeJSON(w, http.StatusOK, map[string]any{"object": "list", "data": data})
	case strings.HasSuffix(r.URL.Path, "/chat/completions"):
		o.chat(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (o *OpenAI) chat(w http.ResponseWriter, r *http.Request) {
	o.Calls++
	if o.Busy > 0 {
		o.Busy--
		w.Header().Set("Retry-After", "1")
		writeJSON(w, http.StatusTooManyRequests, map[string]any{"error": map[string]any{"message": "slow down"}})
		return
	}
	var in struct {
		Model    string `json:"model"`
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<20)).Decode(&in); err != nil || len(in.Messages) != 2 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": map[string]any{"message": "bad request"}})
		return
	}
	o.System, o.User = in.Messages[0].Content, in.Messages[1].Content
	var asked struct {
		Lines []line `json:"lines"`
	}
	if err := json.Unmarshal([]byte(o.User), &asked); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": map[string]any{"message": "user message is not JSON"}})
		return
	}
	o.Sizes = append(o.Sizes, len(asked.Lines))
	prefix := o.Prefix
	if prefix == "" {
		prefix = "PT: "
	}
	out := []line{}
	for _, l := range asked.Lines {
		if o.Drop[l.N] > 0 {
			o.Drop[l.N]--
			continue
		}
		out = append(out, line{l.N, prefix + l.Text})
	}
	if o.Extra {
		out = append(out, line{N: 99999, Text: "not asked"})
	}
	b, _ := json.Marshal(map[string]any{"lines": out})
	content, finish := string(b), "stop"
	if o.CutOver > 0 && len(asked.Lines) > o.CutOver {
		content, finish = content[:len(content)/2], "length"
	}
	if o.Fence {
		content = "Here you go:\n```json\n" + content + "\n```"
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": "chat-1", "object": "chat.completion", "model": in.Model,
		"choices": []any{map[string]any{"index": 0, "message": map[string]any{"role": "assistant", "content": content}, "finish_reason": finish}}})
}

// Lingarr answers POST /api/translate/content like a Lingarr server.
type Lingarr struct {
	mu    sync.Mutex
	Key   string // when set, must arrive as X-Api-Key
	Calls int
	// Body holds the last request.
	Body map[string]any
}

func (l *Lingarr) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if r.Method != http.MethodPost || r.URL.Path != "/api/translate/content" {
		http.NotFound(w, r)
		return
	}
	if l.Key != "" && r.Header.Get("X-Api-Key") != l.Key {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	l.Calls++
	var in map[string]any
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<20)).Decode(&in); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	l.Body = in
	raw, _ := in["lines"].([]any)
	out := []any{}
	for _, x := range raw {
		m, _ := x.(map[string]any)
		out = append(out, map[string]any{"position": m["position"], "line": fmt.Sprintf("%v: %v", in["targetLanguage"], m["line"])})
	}
	writeJSON(w, http.StatusOK, out)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
