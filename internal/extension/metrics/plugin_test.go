package metrics_test

import (
	"net/http"
	"testing"

	mbtrics "moonbridge/internal/extension/metrics"
	"moonbridge/internal/extension/plugin"
	"moonbridge/internal/foundation/config"
)

func TestName(t *testing.T) {
	p := mbtrics.NewPlugin()
	if p.Name() != "metrics" {
		t.Fatalf("Name() = %q, want %q", p.Name(), "metrics")
	}
}

func TestEnabledForModel(t *testing.T) {
	p := mbtrics.NewPlugin()
	// Without a store, EnabledForModel returns false
	if p.EnabledForModel("any-model") {
		t.Fatal("EnabledForModel should be false when store is nil")
	}
}

func TestConfigSpecs(t *testing.T) {
	specs := mbtrics.ConfigSpecs()
	if len(specs) != 1 {
		t.Fatalf("ConfigSpecs returned %d specs, want 1", len(specs))
	}
	spec := specs[0]
	if spec.Name != "metrics" {
		t.Fatalf("spec.Name = %q, want %q", spec.Name, "metrics")
	}
	if spec.Factory == nil {
		t.Fatal("spec.Factory should not be nil")
	}
	cfg := spec.Factory()
	if _, ok := cfg.(*mbtrics.Config); !ok {
		t.Fatalf("Factory returned %T, want *Config", cfg)
	}
}

func TestStoreNilWhenNotConfigured(t *testing.T) {
	p := mbtrics.NewPlugin()
	ctx := plugin.PluginContext{
		Config:    nil,
		AppConfig: config.Config{},
	}
	if err := p.Init(ctx); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if p.Store() != nil {
		t.Fatal("Store() should be nil when sqlite_path is empty")
	}
}

func TestInitWithEmptyConfigPath(t *testing.T) {
	p := mbtrics.NewPlugin()
	cfg := &mbtrics.Config{SQLitePath: ""}
	ctx := plugin.PluginContext{
		Config:    cfg,
		AppConfig: config.Config{},
	}
	if err := p.Init(ctx); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if p.Store() != nil {
		t.Fatal("Store() should be nil when sqlite_path is empty")
	}
}

func TestShutdownNoError(t *testing.T) {
	p := mbtrics.NewPlugin()
	if err := p.Shutdown(); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}
}

func TestInterfaceCompliance(t *testing.T) {
	p := mbtrics.NewPlugin()
	var _ plugin.Plugin = p
	var _ plugin.ConfigSpecProvider = p
	var _ plugin.RequestCompletionHook = p
	var _ plugin.RouteRegistrar = p
}

func TestOnRequestCompletedNilStore(t *testing.T) {
	p := mbtrics.NewPlugin()
	// Should not panic when store is nil
	p.OnRequestCompleted(nil, plugin.RequestResult{
		Model:       "test",
		InputTokens: 100,
		Status:      "success",
	})
}

func TestRegisterRoutesNilStore(t *testing.T) {
	p := mbtrics.NewPlugin()
	called := false
	p.RegisterRoutes(func(pattern string, handler http.Handler) {
		called = true
	})
	if called {
		t.Fatal("RegisterRoutes should not register when store is nil")
	}
}
