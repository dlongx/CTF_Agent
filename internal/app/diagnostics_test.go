package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestProviderConnectivitySuccessAndAuthentication(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" || r.Header.Get("Authorization") != "Bearer test-key" {
			http.Error(w, "unexpected request", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer server.Close()
	service := serviceWithProvider(server.URL+"/v1", "test-key")

	response := service.TestProvider(context.Background(), ProviderFormatOpenAICompatible)
	if !response.OK || response.ErrorCode != "" {
		t.Fatalf("provider test=%+v", response)
	}
}

func TestProviderConnectivityReturnsSanitizedStatus(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(522)
		_, _ = w.Write([]byte("secret-test-key must not be returned"))
	}))
	defer server.Close()
	service := serviceWithProvider(server.URL, "secret-test-key")

	response := service.TestProvider(context.Background(), ProviderFormatOpenAICompatible)
	if response.OK || response.ErrorCode != "upstream_http_522" {
		t.Fatalf("provider test=%+v", response)
	}
}

func TestProviderConnectivityHonorsCallerCancellation(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer server.Close()
	service := serviceWithProvider(server.URL, "test-key")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	response := service.TestProvider(ctx, ProviderFormatOpenAICompatible)
	if response.OK || response.ErrorCode != "timeout" {
		t.Fatalf("provider test=%+v", response)
	}
}

func TestProviderConnectivityAnthropicAndInvalidResponses(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") != "anthropic-secret" || r.Header.Get("anthropic-version") == "" {
			t.Errorf("anthropic headers=%v", r.Header)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("not-json"))
	}))
	defer server.Close()
	service := &Service{cfg: Config{OpenCodeProviders: map[string]OpenCodeProviderConfig{
		ProviderFormatAnthropic: {
			Format: ProviderFormatAnthropic, ProviderID: "anthropic", ProviderNPM: "@ai-sdk/anthropic",
			BaseURL: server.URL, APIKey: "anthropic-secret", Model: "model",
		},
	}}}
	response := service.TestProvider(context.Background(), "claude")
	if response.OK || response.Format != ProviderFormatAnthropic || response.ErrorCode != "invalid_response" {
		t.Fatalf("response=%+v", response)
	}
	if got := service.TestProvider(context.Background(), "unknown"); got.ErrorCode != "unsupported_provider" {
		t.Fatalf("unsupported response=%+v", got)
	}
	service.cfg.OpenCodeProviders[ProviderFormatOpenAICompatible] = OpenCodeProviderConfig{Format: ProviderFormatOpenAICompatible}
	if got := service.TestProvider(context.Background(), "openai"); got.ErrorCode != "provider_not_configured" {
		t.Fatalf("unconfigured response=%+v", got)
	}
}

func serviceWithProvider(baseURL string, apiKey string) *Service {
	provider := OpenCodeProviderConfig{
		Format: ProviderFormatOpenAICompatible, ProviderID: "ctf", ProviderNPM: "@ai-sdk/openai-compatible",
		BaseURL: baseURL, APIKey: apiKey, Model: "test-model",
	}
	return &Service{
		cfg:                  Config{OpenCodeProviders: map[string]OpenCodeProviderConfig{ProviderFormatOpenAICompatible: provider}},
		activeProviderFormat: ProviderFormatOpenAICompatible,
	}
}
