package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"
)

var writeupPattern = regexp.MustCompile(`/workspace/([A-Za-z0-9._-]+-wp\.md)`)

type completionRequest struct {
	Stream   bool             `json:"stream"`
	Messages []map[string]any `json:"messages"`
	Tools    []map[string]any `json:"tools"`
}

func main() {
	addr := flag.String("addr", "127.0.0.1:8317", "listen address")
	flag.Parse()
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/models", modelsHandler)
	mux.HandleFunc("/v1/chat/completions", completionHandler)
	server := &http.Server{
		Addr:              *addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       30 * time.Second,
	}
	slog.Info("fake provider listening", "addr", *addr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("fake provider stopped", "error", err)
		os.Exit(1)
	}
}

func modelsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"object": "list",
		"data":   []map[string]any{{"id": "ctf-smoke", "object": "model", "owned_by": "ctf-agent"}},
	})
}

func completionHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 4<<20))
	if err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	var request completionRequest
	if err := json.Unmarshal(body, &request); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if hasToolResult(request.Messages) {
		respondCompletion(w, request.Stream, finalMessage())
		return
	}
	toolName, arguments := writeupToolCall(request.Tools, body)
	if toolName == "" {
		respondCompletion(w, request.Stream, finalMessage())
		return
	}
	respondToolCall(w, request.Stream, toolName, arguments)
}

func hasToolResult(messages []map[string]any) bool {
	for _, message := range messages {
		if role, _ := message["role"].(string); role == "tool" {
			return true
		}
		if _, ok := message["tool_call_id"]; ok {
			return true
		}
	}
	return false
}

func writeupToolCall(tools []map[string]any, rawBody []byte) (string, string) {
	writeup := "challenge-wp.md"
	if match := writeupPattern.FindSubmatch(rawBody); len(match) == 2 {
		writeup = string(match[1])
	}
	content := "# 自动化烟测WP\n\n读取附件后得到Flag：`flag{ctf_agent_smoke_ok}`。\n"
	for _, preferred := range []string{"write", "bash"} {
		for _, tool := range tools {
			function, _ := tool["function"].(map[string]any)
			name, _ := function["name"].(string)
			if name == "" {
				name, _ = tool["name"].(string)
			}
			if !strings.EqualFold(name, preferred) {
				continue
			}
			arguments := map[string]any{}
			if preferred == "write" {
				arguments["filePath"] = "/workspace/" + writeup
				arguments["content"] = content
			} else {
				command := fmt.Sprintf("python -c %q", "from pathlib import Path; Path('/workspace/"+writeup+"').write_text("+fmt.Sprintf("%q", content)+", encoding='utf-8')")
				arguments["command"] = command
				arguments["description"] = "写入自动化烟测WP"
			}
			encoded, _ := json.Marshal(arguments)
			return name, string(encoded)
		}
	}
	return "", ""
}

func finalMessage() map[string]any {
	return map[string]any{
		"role":    "assistant",
		"content": "这道题目已经解出\nflag{ctf_agent_smoke_ok}",
	}
}

func respondCompletion(w http.ResponseWriter, stream bool, message map[string]any) {
	if !stream {
		writeJSON(w, http.StatusOK, completionEnvelope(message, "stop"))
		return
	}
	writeSSE(w, map[string]any{
		"id": "chatcmpl-ctf-agent", "object": "chat.completion.chunk", "created": time.Now().Unix(),
		"model": "ctf-smoke", "choices": []map[string]any{{"index": 0, "delta": message, "finish_reason": "stop"}},
	})
}

func respondToolCall(w http.ResponseWriter, stream bool, toolName string, arguments string) {
	toolCall := map[string]any{
		"id": "call_ctf_agent_writeup", "type": "function",
		"function": map[string]any{"name": toolName, "arguments": arguments},
	}
	if !stream {
		writeJSON(w, http.StatusOK, completionEnvelope(map[string]any{
			"role": "assistant", "content": nil, "tool_calls": []map[string]any{toolCall},
		}, "tool_calls"))
		return
	}
	toolCall["index"] = 0
	writeSSE(w, map[string]any{
		"id": "chatcmpl-ctf-agent", "object": "chat.completion.chunk", "created": time.Now().Unix(),
		"model": "ctf-smoke", "choices": []map[string]any{{
			"index":         0,
			"delta":         map[string]any{"role": "assistant", "tool_calls": []map[string]any{toolCall}},
			"finish_reason": "tool_calls",
		}},
	})
}

func completionEnvelope(message map[string]any, finishReason string) map[string]any {
	return map[string]any{
		"id": "chatcmpl-ctf-agent", "object": "chat.completion", "created": time.Now().Unix(), "model": "ctf-smoke",
		"choices": []map[string]any{{"index": 0, "message": message, "finish_reason": finishReason}},
		"usage":   map[string]int{"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2},
	}
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeSSE(w http.ResponseWriter, payload any) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	encoded, _ := json.Marshal(payload)
	_, _ = fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", encoded)
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
}
