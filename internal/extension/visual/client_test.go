package visual

import (
	"context"
	"testing"

	"moonbridge/internal/protocol/anthropic"
)

func TestBridgeClientUsesExistingProvider(t *testing.T) {
	var metrics []ToolCallMetrics
	upstream := &fakeUpstream{responses: []anthropic.MessageResponse{{
		ID:         "msg_visual",
		StopReason: "end_turn",
		Content:    []anthropic.ContentBlock{{Type: "text", Text: "mountain scene"}},
		Usage:      anthropic.Usage{InputTokens: 20, OutputTokens: 5, CacheReadInputTokens: 7},
	}}}
	client := NewBridgeClient(ClientConfig{
		Provider:     upstream,
		Model:        "kimi-for-coding",
		MetricsModel: "moonshot/kimi-for-coding",
		MaxTokens:    512,
		MetricsRecorder: func(metric ToolCallMetrics) {
			metrics = append(metrics, metric)
		},
	})

	text, err := client.Analyze(context.Background(), AnalysisRequest{
		Tool:   ToolVisualBrief,
		Prompt: "describe",
		Images: []ImageInput{{URL: "https://example.test/a.png"}},
	})
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	if text != "mountain scene" {
		t.Fatalf("Analyze() = %q", text)
	}
	if len(upstream.requests) != 1 {
		t.Fatalf("provider requests = %d, want 1", len(upstream.requests))
	}
	req := upstream.requests[0]
	if req.Model != "kimi-for-coding" || req.MaxTokens != 512 {
		t.Fatalf("visual request model/max = %s/%d", req.Model, req.MaxTokens)
	}
	if len(req.System) != 1 || req.System[0].Text == "" {
		t.Fatalf("visual system prompt = %+v", req.System)
	}
	if len(req.Messages) != 1 || len(req.Messages[0].Content) != 2 {
		t.Fatalf("visual messages = %+v", req.Messages)
	}
	image := req.Messages[0].Content[1]
	if image.Type != "image" || image.Source == nil || image.Source.Type != "url" || image.Source.URL != "https://example.test/a.png" {
		t.Fatalf("visual image block = %+v", image)
	}
	if len(metrics) != 1 {
		t.Fatalf("metrics len = %d, want 1", len(metrics))
	}
	metric := metrics[0]
	if metric.Model != "moonshot/kimi-for-coding" || metric.ActualModel != "kimi-for-coding" || metric.Tool != ToolVisualBrief {
		t.Fatalf("metric identity = %+v", metric)
	}
	if metric.Status != "success" || metric.Usage.InputTokens != 20 || metric.Usage.CacheReadInputTokens != 7 || metric.Usage.OutputTokens != 5 {
		t.Fatalf("metric usage = %+v", metric)
	}
}

func TestBridgeClientRecordsProviderConfigErrorMetrics(t *testing.T) {
	var metrics []ToolCallMetrics
	client := NewBridgeClient(ClientConfig{
		Model:        "kimi-for-coding",
		MetricsModel: "moonshot/kimi-for-coding",
		MetricsRecorder: func(metric ToolCallMetrics) {
			metrics = append(metrics, metric)
		},
	})

	_, err := client.Analyze(context.Background(), AnalysisRequest{Tool: ToolVisualQA, Prompt: "question"})
	if err == nil {
		t.Fatal("Analyze() error = nil, want provider error")
	}
	if len(metrics) != 1 {
		t.Fatalf("metrics len = %d, want 1", len(metrics))
	}
	metric := metrics[0]
	if metric.Status != "error" || metric.ErrorMessage != "visual provider is nil" {
		t.Fatalf("metric error = %+v", metric)
	}
	if metric.Model != "moonshot/kimi-for-coding" || metric.ActualModel != "kimi-for-coding" || metric.Tool != ToolVisualQA {
		t.Fatalf("metric identity = %+v", metric)
	}
}
