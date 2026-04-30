package provider

import "testing"

func TestProviderManagerRoutesProtocolAndUpstreamModel(t *testing.T) {
	manager, err := NewProviderManager(map[string]ProviderConfig{
		"default": {
			BaseURL: "https://anthropic.example.test",
			APIKey:  "anthropic-key",
		},
		"openai": {
			BaseURL:  "https://openai.example.test",
			APIKey:   "openai-key",
			Protocol: "openai-response",
		},
	}, map[string]ModelRoute{
		"image": {Provider: "openai", Name: "gpt-image-1.5"},
	})
	if err != nil {
		t.Fatalf("NewProviderManager() error = %v", err)
	}

	if got := manager.ProtocolForModel("image"); got != "openai-response" {
		t.Fatalf("ProtocolForModel(image) = %q", got)
	}
	if got := manager.UpstreamModelFor("image"); got != "gpt-image-1.5" {
		t.Fatalf("UpstreamModelFor(image) = %q", got)
	}
	if got := manager.ProtocolForModel("unrouted"); got != "anthropic" {
		t.Fatalf("ProtocolForModel(unrouted) = %q", got)
	}
	if got := manager.UpstreamModelFor("unrouted"); got != "unrouted" {
		t.Fatalf("UpstreamModelFor(unrouted) = %q", got)
	}
}

func TestProviderManagerUsesDefaultProtocolForUnroutedModels(t *testing.T) {
	manager, err := NewProviderManager(map[string]ProviderConfig{
		"default": {
			BaseURL:  "https://openai.example.test",
			APIKey:   "openai-key",
			Protocol: "openai-response",
		},
	}, nil)
	if err != nil {
		t.Fatalf("NewProviderManager() error = %v", err)
	}

	if got := manager.ProtocolForModel("gpt-test"); got != "openai-response" {
		t.Fatalf("ProtocolForModel(unrouted default openai) = %q", got)
	}
}

func TestResolveModel_TwoProvidersSameModel(t *testing.T) {
	manager, err := NewProviderManager(map[string]ProviderConfig{
		"alpha": {
			BaseURL:    "https://alpha.test",
			APIKey:     "key-alpha",
			ModelNames: []string{"claude-sonnet-4-5"},
		},
		"beta": {
			BaseURL:    "https://beta.test",
			APIKey:     "key-beta",
			ModelNames: []string{"claude-sonnet-4-5"},
		},
	}, nil)
	if err != nil {
		t.Fatalf("NewProviderManager() error = %v", err)
	}

	route, err := manager.ResolveModel("claude-sonnet-4-5")
	if err != nil {
		t.Fatalf("ResolveModel() error = %v", err)
	}
	if len(route.Candidates) != 2 {
		t.Fatalf("expected 2 candidates, got %d", len(route.Candidates))
	}
	// Should be sorted: alpha < beta
	if route.Candidates[0].ProviderKey != "alpha" || route.Candidates[1].ProviderKey != "beta" {
		t.Errorf("expected alpha, beta; got %s, %s",
			route.Candidates[0].ProviderKey, route.Candidates[1].ProviderKey)
	}
	// Both should have non-nil clients
	for i, c := range route.Candidates {
		if c.Client == nil {
			t.Errorf("candidate[%d] has nil client", i)
		}
	}
}

func TestResolveModel_RouteAliasPriority(t *testing.T) {
	manager, err := NewProviderManager(map[string]ProviderConfig{
		"p1": {
			BaseURL:    "https://p1.test",
			APIKey:     "key-p1",
			ModelNames: []string{"claude-sonnet-4-5"},
		},
		"p2": {
			BaseURL: "https://p2.test",
			APIKey:  "key-p2",
		},
	}, map[string]ModelRoute{
		"claude-sonnet-4-5": {Provider: "p2", Name: "claude-sonnet-4-5-v2"},
	})
	if err != nil {
		t.Fatalf("NewProviderManager() error = %v", err)
	}

	route, err := manager.ResolveModel("claude-sonnet-4-5")
	if err != nil {
		t.Fatalf("ResolveModel() error = %v", err)
	}
	if len(route.Candidates) != 1 {
		t.Fatalf("expected 1 candidate (route alias), got %d", len(route.Candidates))
	}
	// Route alias should win over same-named model in provider catalog
	if route.Candidates[0].ProviderKey != "p2" {
		t.Errorf("expected provider p2 (route alias), got %s", route.Candidates[0].ProviderKey)
	}
	if route.Candidates[0].UpstreamModel != "claude-sonnet-4-5-v2" {
		t.Errorf("expected upstream model claude-sonnet-4-5-v2, got %s", route.Candidates[0].UpstreamModel)
	}
}

func TestResolveModel_DirectRef(t *testing.T) {
	manager, err := NewProviderManager(map[string]ProviderConfig{
		"my-provider": {
			BaseURL: "https://my.test",
			APIKey:  "key-my",
		},
		"other": {
			BaseURL: "https://other.test",
			APIKey:  "key-other",
		},
	}, nil)
	if err != nil {
		t.Fatalf("NewProviderManager() error = %v", err)
	}

	// Test provider/model format
	route, err := manager.ResolveModel("my-provider/gpt-4")
	if err != nil {
		t.Fatalf("ResolveModel(provider/model) error = %v", err)
	}
	if len(route.Candidates) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(route.Candidates))
	}
	if route.Candidates[0].ProviderKey != "my-provider" {
		t.Errorf("expected provider my-provider, got %s", route.Candidates[0].ProviderKey)
	}
	if route.Candidates[0].UpstreamModel != "gpt-4" {
		t.Errorf("expected upstream model gpt-4, got %s", route.Candidates[0].UpstreamModel)
	}

	// Test model(provider) format
	route, err = manager.ResolveModel("gpt-4(other)")
	if err != nil {
		t.Fatalf("ResolveModel(model(provider)) error = %v", err)
	}
	if len(route.Candidates) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(route.Candidates))
	}
	if route.Candidates[0].ProviderKey != "other" {
		t.Errorf("expected provider other, got %s", route.Candidates[0].ProviderKey)
	}
	if route.Candidates[0].UpstreamModel != "gpt-4" {
		t.Errorf("expected upstream model gpt-4, got %s", route.Candidates[0].UpstreamModel)
	}
}

func TestResolveModel_ModelNotFound(t *testing.T) {
	manager, err := NewProviderManager(map[string]ProviderConfig{
		"default": {
			BaseURL: "https://default.test",
			APIKey:  "key-default",
		},
	}, nil)
	if err != nil {
		t.Fatalf("NewProviderManager() error = %v", err)
	}

	_, err = manager.ResolveModel("nonexistent-model")
	if err == nil {
		t.Fatal("ResolveModel() expected error for unknown model")
	}
}

func TestResolveModel_CandidatesSortedByProviderKey(t *testing.T) {
	manager, err := NewProviderManager(map[string]ProviderConfig{
		"zulu": {
			BaseURL:    "https://z.test",
			APIKey:     "key-z",
			ModelNames: []string{"shared-model"},
		},
		"alpha": {
			BaseURL:    "https://a.test",
			APIKey:     "key-a",
			ModelNames: []string{"shared-model"},
		},
		"mike": {
			BaseURL:    "https://m.test",
			APIKey:     "key-m",
			ModelNames: []string{"shared-model"},
		},
	}, nil)
	if err != nil {
		t.Fatalf("NewProviderManager() error = %v", err)
	}

	route, err := manager.ResolveModel("shared-model")
	if err != nil {
		t.Fatalf("ResolveModel() error = %v", err)
	}
	if len(route.Candidates) != 3 {
		t.Fatalf("expected 3 candidates, got %d", len(route.Candidates))
	}
	expected := []string{"alpha", "mike", "zulu"}
	for i, exp := range expected {
		if route.Candidates[i].ProviderKey != exp {
			t.Errorf("candidate[%d].ProviderKey = %s, want %s", i, route.Candidates[i].ProviderKey, exp)
		}
	}
}

func TestResolveModel_Preferred(t *testing.T) {
	manager, err := NewProviderManager(map[string]ProviderConfig{
		"p1": {
			BaseURL:    "https://p1.test",
			APIKey:     "key-p1",
			ModelNames: []string{"model-x"},
		},
	}, nil)
	if err != nil {
		t.Fatalf("NewProviderManager() error = %v", err)
	}

	route, err := manager.ResolveModel("model-x")
	if err != nil {
		t.Fatalf("ResolveModel() error = %v", err)
	}

	candidate, ok := route.Preferred()
	if !ok {
		t.Fatal("Preferred() returned false, expected true")
	}
	if candidate.ProviderKey != "p1" {
		t.Errorf("Preferred().ProviderKey = %s, want p1", candidate.ProviderKey)
	}
}

func TestResolveModel_ProviderPriority(t *testing.T) {
	manager, err := NewProviderManager(map[string]ProviderConfig{
		"cheap": {
			BaseURL:    "https://cheap.test",
			APIKey:     "key-cheap",
			Priority:   10,
			ModelNames: []string{"shared-model"},
		},
		"fast": {
			BaseURL:    "https://fast.test",
			APIKey:     "key-fast",
			Priority:   5,
			ModelNames: []string{"shared-model"},
		},
		"zulu": {
			BaseURL:    "https://z.test",
			APIKey:     "key-z",
			ModelNames: []string{"shared-model"},
		},
	}, nil)
	if err != nil {
		t.Fatalf("NewProviderManager() error = %v", err)
	}

	route, err := manager.ResolveModel("shared-model")
	if err != nil {
		t.Fatalf("ResolveModel() error = %v", err)
	}
	if len(route.Candidates) != 3 {
		t.Fatalf("expected 3 candidates, got %d", len(route.Candidates))
	}
	// zulu (priority=0, default) should come first, then fast (priority=5), then cheap (priority=10)
	expected := []string{"zulu", "fast", "cheap"}
	for i, exp := range expected {
		if route.Candidates[i].ProviderKey != exp {
			t.Errorf("candidate[%d].ProviderKey = %s, want %s", i, route.Candidates[i].ProviderKey, exp)
		}
	}
}
