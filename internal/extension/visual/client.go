package visual

import (
	"context"
	"fmt"
	"strings"
	"time"

	"moonbridge/internal/protocol/anthropic"
)

const visualSystemPrompt = "You are Kimi running behind Moon Bridge Visual. Analyze images carefully, state uncertainty, and do not invent visual facts."

type ClientConfig struct {
	Provider        Provider
	Model           string
	MetricsModel    string
	MaxTokens       int
	MetricsRecorder MetricsRecorder
}

type ImageInput struct {
	URL      string `json:"url,omitempty"`
	Data     string `json:"data,omitempty"`
	MimeType string `json:"mime_type,omitempty"`
	Detail   string `json:"detail,omitempty"`
}

type ConversationTurn struct {
	Role    string `json:"role,omitempty"`
	Content string `json:"content,omitempty"`
}

type AnalysisRequest struct {
	Tool   string
	Prompt string
	Images []ImageInput
}

// ToolCallMetrics captures telemetry for one Visual provider call.
type ToolCallMetrics struct {
	Tool         string
	Model        string
	ActualModel  string
	Usage        anthropic.Usage
	Duration     time.Duration
	Status       string
	ErrorMessage string
}

type MetricsRecorder func(ToolCallMetrics)

type VisionClient interface {
	Analyze(context.Context, AnalysisRequest) (string, error)
}

type BridgeClient struct {
	provider        Provider
	model           string
	metricsModel    string
	maxTokens       int
	metricsRecorder MetricsRecorder
}

func NewBridgeClient(cfg ClientConfig) *BridgeClient {
	maxTokens := cfg.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 2048
	}
	model := strings.TrimSpace(cfg.Model)
	metricsModel := strings.TrimSpace(cfg.MetricsModel)
	if metricsModel == "" {
		metricsModel = model
	}
	return &BridgeClient{
		provider:        cfg.Provider,
		model:           model,
		metricsModel:    metricsModel,
		maxTokens:       maxTokens,
		metricsRecorder: cfg.MetricsRecorder,
	}
}

func (client *BridgeClient) Analyze(ctx context.Context, request AnalysisRequest) (string, error) {
	if client == nil {
		return "", fmt.Errorf("visual bridge client is nil")
	}
	if client.provider == nil {
		err := fmt.Errorf("visual provider is nil")
		client.recordMetrics(request, anthropic.MessageResponse{}, 0, "error", err.Error())
		return "", err
	}
	if client.model == "" {
		err := fmt.Errorf("visual model is required")
		client.recordMetrics(request, anthropic.MessageResponse{}, 0, "error", err.Error())
		return "", err
	}

	start := time.Now()
	resp, err := client.provider.CreateMessage(ctx, anthropic.MessageRequest{
		Model:     client.model,
		MaxTokens: client.maxTokens,
		System: []anthropic.ContentBlock{{
			Type: "text",
			Text: visualSystemPrompt,
		}},
		Messages: []anthropic.Message{{
			Role:    "user",
			Content: anthropicContentParts(request),
		}},
	})
	duration := time.Since(start)
	if err != nil {
		client.recordMetrics(request, resp, duration, "error", err.Error())
		return "", err
	}
	text := textFromContent(resp.Content)
	if text == "" {
		err := fmt.Errorf("visual provider returned empty content")
		client.recordMetrics(request, resp, duration, "error", err.Error())
		return "", err
	}
	client.recordMetrics(request, resp, duration, "success", "")
	return text, nil
}

func (client *BridgeClient) recordMetrics(request AnalysisRequest, resp anthropic.MessageResponse, duration time.Duration, status string, errMsg string) {
	if client == nil || client.metricsRecorder == nil {
		return
	}
	actualModel := client.model
	if strings.TrimSpace(resp.Model) != "" {
		actualModel = strings.TrimSpace(resp.Model)
	}
	client.metricsRecorder(ToolCallMetrics{
		Tool:         request.Tool,
		Model:        client.metricsModel,
		ActualModel:  actualModel,
		Usage:        resp.Usage,
		Duration:     duration,
		Status:       status,
		ErrorMessage: errMsg,
	})
}

func anthropicContentParts(request AnalysisRequest) []anthropic.ContentBlock {
	parts := []anthropic.ContentBlock{{Type: "text", Text: request.Prompt}}
	for _, image := range request.Images {
		source := image.AnthropicSource()
		if source == nil {
			continue
		}
		parts = append(parts, anthropic.ContentBlock{
			Type:   "image",
			Source: source,
		})
	}
	return parts
}

func textFromContent(blocks []anthropic.ContentBlock) string {
	var b strings.Builder
	for _, block := range blocks {
		if block.Type != "text" || strings.TrimSpace(block.Text) == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(strings.TrimSpace(block.Text))
	}
	return strings.TrimSpace(b.String())
}

func (image ImageInput) HasAnthropicSource() bool {
	return image.AnthropicSource() != nil
}

func (image ImageInput) AnthropicSource() *anthropic.ImageSource {
	if strings.TrimSpace(image.URL) != "" {
		url := strings.TrimSpace(image.URL)
		if !isSupportedImageURL(url) {
			return nil
		}
		if strings.HasPrefix(url, "data:") {
			return dataURLSource(url)
		}
		return &anthropic.ImageSource{Type: "url", URL: url}
	}
	data := strings.TrimSpace(image.Data)
	if data == "" {
		return nil
	}
	if strings.HasPrefix(data, "data:") {
		return dataURLSource(data)
	}
	mimeType := strings.TrimSpace(image.MimeType)
	if mimeType == "" {
		mimeType = "image/png"
	}
	return &anthropic.ImageSource{Type: "base64", MediaType: mimeType, Data: data}
}

func isSupportedImageURL(value string) bool {
	lower := strings.ToLower(strings.TrimSpace(value))
	return strings.HasPrefix(lower, "http://") ||
		strings.HasPrefix(lower, "https://") ||
		strings.HasPrefix(lower, "data:")
}

func dataURLSource(value string) *anthropic.ImageSource {
	header, data, ok := strings.Cut(value, ",")
	if !ok {
		return nil
	}
	mediaType := strings.TrimPrefix(header, "data:")
	if semicolon := strings.IndexByte(mediaType, ';'); semicolon >= 0 {
		mediaType = mediaType[:semicolon]
	}
	if mediaType == "" {
		mediaType = "image/png"
	}
	return &anthropic.ImageSource{Type: "base64", MediaType: mediaType, Data: data}
}
