// Package metrics implements a Moon Bridge extension that persists per-request
// usage metrics to a SQLite database for historical query and analysis.
//
// It implements:
//   - RequestCompletionHook: records each request result to SQLite
//   - RouteRegistrar: exposes query endpoints (e.g. GET /v1/admin/metrics)
//
// Configuration (in extensions.metrics):
//
//	extensions:
//	  metrics:
//	    config:
//	      sqlite_path: "metrics.db"   # relative to CWD, or absolute path
package metrics

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"time"
	"strconv"

	"moonbridge/internal/extension/plugin"
	"moonbridge/internal/foundation/config"
	"moonbridge/internal/service/metrics"
)

const PluginName = "metrics"

// Config holds the metrics extension configuration.
type Config struct {
	// SQLitePath is the path to the SQLite database file.
	// If empty, metrics persistence is disabled.
	// If relative, it is resolved relative to the current working directory.
	SQLitePath string `json:"sqlite_path,omitempty" yaml:"sqlite_path"`
}

// Plugin implements the metrics extension, managing a SQLite-backed metrics
// store lifecycle. It records per-request metrics and serves queries via
// registered HTTP endpoints.
type Plugin struct {
	plugin.BasePlugin
	store *metrics.Store
	appCfg config.Config
}

// NewPlugin creates a new metrics extension plugin.
func NewPlugin() *Plugin {
	return &Plugin{}
}

func (p *Plugin) Name() string { return PluginName }

// ConfigSpecs returns the extension config spec for the metrics extension.
func (p *Plugin) ConfigSpecs() []config.ExtensionConfigSpec {
	return ConfigSpecs()
}

// ConfigSpecs returns the extension config spec for registration.
func ConfigSpecs() []config.ExtensionConfigSpec {
	return []config.ExtensionConfigSpec{{
		Name: PluginName,
		Scopes: []config.ExtensionScope{
			config.ExtensionScopeGlobal,
		},
		Factory: func() any { return &Config{} },
	}}
}


func (p *Plugin) Init(ctx plugin.PluginContext) error {
	cfg := plugin.Config[Config](ctx)
	if cfg == nil || cfg.SQLitePath == "" {
		if ctx.Logger != nil {
			ctx.Logger.Info("指标持久化已禁用（未配置 sqlite_path）")
		}
		return nil
	}
	absPath, err := filepath.Abs(cfg.SQLitePath)
	if err != nil {
		return fmt.Errorf("解析 sqlite_path 失败: %w", err)
	}
	store, err := metrics.NewStore(absPath)
	if err != nil {
		return fmt.Errorf("初始化指标存储失败: %w", err)
	}
	p.store = store
	p.appCfg = ctx.AppConfig
	if ctx.Logger != nil {
		ctx.Logger.Info("指标持久化已启用", "path", absPath)
	}
	return nil
}

// Shutdown closes the SQLite store.
func (p *Plugin) Shutdown() error {
	if p.store != nil {
		return p.store.Close()
	}
	return nil
}

// EnabledForModel always returns true since metrics recording applies globally.
// Respects extensions.metrics.enabled: false.
func (p *Plugin) EnabledForModel(string) bool {
	if p.store == nil {
		return false
	}
	// Check global enabled flag: default to true if sqlite_path is set.
	if !p.appCfg.ExtensionEnabled(PluginName, "") {
		return false
	}
	return true
}

// Store returns the underlying metrics store. Returns nil when disabled.
func (p *Plugin) Store() *metrics.Store {
	if p == nil {
		return nil
	}
	return p.store
}

// --- RequestCompletionHook ---

// OnRequestCompleted records the request result to SQLite.
func (p *Plugin) OnRequestCompleted(_ *plugin.RequestContext, result plugin.RequestResult) {
	if p.store == nil {
		return
	}
	r := metrics.Record{
		Timestamp:     time.Now(),
		Model:         result.Model,
		ActualModel:   result.ActualModel,
		InputTokens:   int64(result.InputTokens),
		OutputTokens:  int64(result.OutputTokens),
		CacheCreation: int64(result.CacheCreation),
		CacheRead:     int64(result.CacheRead),
		Cost:          result.Cost,
		ResponseTime:  result.Duration,
		Status:        result.Status,
		ErrorMessage:  result.ErrorMessage,
	}
	if err := p.store.Record(r); err != nil {
		// No logger reference available here — silently drop for now.
	}
}

// --- RouteRegistrar ---

// RegisterRoutes mounts the metrics query endpoints on the server mux.
func (p *Plugin) RegisterRoutes(register func(pattern string, handler http.Handler)) {
	if p.store == nil {
		return
	}
	register("GET /v1/admin/metrics", http.HandlerFunc(p.handleQuery))
}

// handleQuery serves GET /v1/admin/metrics — returns recent metrics as JSON.
func (p *Plugin) handleQuery(w http.ResponseWriter, r *http.Request) {
	if p.store == nil {
		http.Error(w, `{"error":"metrics disabled"}`, http.StatusNotFound)
		return
	}

	limit := 100 // default
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 1000 {
			limit = parsed
		}
	}

	opts := metrics.QueryOptions{Limit: limit, OrderAsc: false}
	if model := r.URL.Query().Get("model"); model != "" {
		opts.Model = model
	}
	if status := r.URL.Query().Get("status"); status != "" {
		opts.Status = status
	}
	if since := r.URL.Query().Get("since"); since != "" {
		if t, err := time.Parse(time.RFC3339Nano, since); err == nil {
			opts.Since = t
		}
	}
	if until := r.URL.Query().Get("until"); until != "" {
		if t, err := time.Parse(time.RFC3339Nano, until); err == nil {
			opts.Until = t
		}
	}
	if offset := r.URL.Query().Get("offset"); offset != "" {
		if parsed, err := strconv.Atoi(offset); err == nil && parsed >= 0 {
			opts.Offset = parsed
		}
	}
	if order := r.URL.Query().Get("order"); order == "asc" {
		opts.OrderAsc = true
	}

	records, err := p.store.Query(opts)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"records": records,
		"count":   len(records),
	})
}

// Compile-time interface checks.
var (
	_ plugin.Plugin                = (*Plugin)(nil)
	_ plugin.ConfigSpecProvider    = (*Plugin)(nil)
	_ plugin.RequestCompletionHook = (*Plugin)(nil)
	_ plugin.RouteRegistrar        = (*Plugin)(nil)
)
