package metrics_test

import (
	"math"
	"testing"
	"time"

	"moonbridge/internal/service/metrics"
)

func TestNewStoreEmptyPath(t *testing.T) {
	s, err := metrics.NewStore("")
	if err != nil {
		t.Fatalf("NewStore('') error = %v, want nil", err)
	}
	if s != nil {
		t.Fatalf("NewStore('') = non-nil, want nil")
	}
}

func TestStoreRecordAndQuery(t *testing.T) {
	dbPath := t.TempDir() + "/test.db"
	s, err := metrics.NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	defer s.Close()

	now := time.Now().Truncate(time.Millisecond)

	records := []metrics.Record{
		{
			Timestamp:    now.Add(-2 * time.Hour),
			Model:        "moonbridge",
			ActualModel:  "deepseek-v4-pro",
			InputTokens:  1000,
			OutputTokens: 500,
			CacheRead:    200,
			Cost:         0.012,
			ResponseTime: 3 * time.Second,
			Status:       "success",
		},
		{
			Timestamp:    now.Add(-1 * time.Hour),
			Model:        "claude",
			ActualModel:  "claude-sonnet-4-20250514",
			InputTokens:  2000,
			OutputTokens: 1000,
			CacheCreation: 500,
			CacheRead:    800,
			Cost:         0.035,
			ResponseTime: 5 * time.Second,
			Status:       "success",
		},
		{
			Timestamp:    now,
			Model:        "moonbridge",
			ActualModel:  "deepseek-v4-pro",
			InputTokens:  500,
			OutputTokens: 200,
			Cost:         0.005,
			ResponseTime: 1 * time.Second,
			Status:       "error",
			ErrorMessage: "rate limit exceeded",
		},
	}

	for _, r := range records {
		if err := s.Record(r); err != nil {
			t.Fatalf("Record() error = %v", err)
		}
	}

	// Query all, newest first (default).
	all, err := s.Query(metrics.QueryOptions{Limit: 10})
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("Query() returned %d records, want 3", len(all))
	}
	// Default order is DESC (newest first).
	if all[0].Model != "moonbridge" || all[0].Status != "error" {
		t.Fatalf("first record = %+v, want moonbridge/error (newest first)", all[0])
	}

	// Query by model.
	modelResults, err := s.Query(metrics.QueryOptions{Model: "claude", Limit: 10})
	if err != nil {
		t.Fatalf("Query(claude) error = %v", err)
	}
	if len(modelResults) != 1 {
		t.Fatalf("Query(claude) = %d records, want 1", len(modelResults))
	}

	// Query by status.
	errResults, err := s.Query(metrics.QueryOptions{Status: "error", Limit: 10})
	if err != nil {
		t.Fatalf("Query(error) error = %v", err)
	}
	if len(errResults) != 1 {
		t.Fatalf("Query(error) = %d records, want 1", len(errResults))
	}

	// Query with time range.
	sinceResults, err := s.Query(metrics.QueryOptions{Since: now.Add(-90 * time.Minute), Limit: 10})
	if err != nil {
		t.Fatalf("Query(since) error = %v", err)
	}
	if len(sinceResults) != 2 {
		t.Fatalf("Query(since 90m ago) = %d records, want 2", len(sinceResults))
	}

	// Query ascending order.
	ascResults, err := s.Query(metrics.QueryOptions{Limit: 10, OrderAsc: true})
	if err != nil {
		t.Fatalf("Query(asc) error = %v", err)
	}
	if len(ascResults) != 3 {
		t.Fatalf("Query(asc) = %d records, want 3", len(ascResults))
	}
	if ascResults[0].Model != "moonbridge" || ascResults[2].Model != "moonbridge" {
		t.Fatalf("asc results unexpected: first=%+v last=%+v", ascResults[0], ascResults[2])
	}
}

func TestStoreRecordOnNilStore(t *testing.T) {
	var s *metrics.Store
	err := s.Record(metrics.Record{})
	if err != nil {
		t.Fatalf("Record on nil store error = %v, want nil", err)
	}
}

func TestStoreQueryOnNilStore(t *testing.T) {
	var s *metrics.Store
	records, err := s.Query(metrics.QueryOptions{Limit: 10})
	if err != nil {
		t.Fatalf("Query on nil store error = %v, want nil", err)
	}
	if records != nil {
		t.Fatalf("expected nil records, got %v", records)
	}
}

func TestStoreCloseOnNilStore(t *testing.T) {
	var s *metrics.Store
	err := s.Close()
	if err != nil {
		t.Fatalf("Close on nil store error = %v, want nil", err)
	}
}

func TestRecordTimestampPrecision(t *testing.T) {
	dbPath := t.TempDir() + "/precision.db"
	s, err := metrics.NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	defer s.Close()

	// Record with nanosecond precision.
	precise := time.Date(2025, 6, 15, 10, 30, 0, 123456789, time.UTC)
	err = s.Record(metrics.Record{
		Timestamp:    precise,
		Model:        "test",
		InputTokens:  100,
		OutputTokens: 50,
		ResponseTime: 1 * time.Second,
		Status:       "success",
	})
	if err != nil {
		t.Fatalf("Record() error = %v", err)
	}

	results, err := s.Query(metrics.QueryOptions{Limit: 10})
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("got %d records, want 1", len(results))
	}
	if !results[0].Timestamp.Equal(precise) {
		t.Fatalf("timestamp = %v, want %v", results[0].Timestamp, precise)
	}
}

func TestLargeValues(t *testing.T) {
	dbPath := t.TempDir() + "/large.db"
	s, err := metrics.NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	defer s.Close()

	r := metrics.Record{
		Timestamp:     time.Now(),
		Model:         "bigmodel",
		ActualModel:   "ultra-large-v99",
		InputTokens:   1_000_000_000,
		OutputTokens:  500_000_000,
		CacheCreation: 200_000_000,
		CacheRead:     800_000_000,
		Cost:          12345.67,
		ResponseTime:  3600 * time.Second, // 1 hour
		Status:        "success",
	}
	if err := s.Record(r); err != nil {
		t.Fatalf("Record(large values) error = %v", err)
	}

	results, err := s.Query(metrics.QueryOptions{Limit: 10})
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("got %d records, want 1", len(results))
	}
	got := results[0]
	if got.InputTokens != r.InputTokens {
		t.Errorf("InputTokens = %d, want %d", got.InputTokens, r.InputTokens)
	}
	if got.Cost != r.Cost {
		t.Errorf("Cost = %f, want %f", got.Cost, r.Cost)
	}
	if got.ResponseTime != r.ResponseTime {
		t.Errorf("ResponseTime = %v, want %v", got.ResponseTime, r.ResponseTime)
	}
}

func TestDefaultLimit(t *testing.T) {
	dbPath := t.TempDir() + "/limit.db"
	s, err := metrics.NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	defer s.Close()

	for i := 0; i < 150; i++ {
		s.Record(metrics.Record{
			Timestamp:    time.Now(),
			Model:        "test",
			InputTokens:  1,
			OutputTokens: 1,
			ResponseTime: time.Millisecond,
			Status:       "success",
		})
	}

	// Default limit is 100.
	results, err := s.Query(metrics.QueryOptions{})
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}
	if len(results) != 100 {
		t.Fatalf("len = %d, want default limit 100", len(results))
	}

	// Explicit limit.
	results, err = s.Query(metrics.QueryOptions{Limit: 200})
	if err != nil {
		t.Fatalf("Query(limit=200) error = %v", err)
	}
	if len(results) != 150 {
		t.Fatalf("len = %d, want 150", len(results))
	}
}

func TestZeroDuration(t *testing.T) {
	dbPath := t.TempDir() + "/zerodur.db"
	s, err := metrics.NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	defer s.Close()

	err = s.Record(metrics.Record{
		Timestamp:    time.Now(),
		Model:        "test",
		InputTokens:  0,
		OutputTokens: 0,
		Cost:         math.Inf(1), // Should still be storable
		ResponseTime: 0,
		Status:       "success",
	})
	if err != nil {
		t.Fatalf("Record() error = %v", err)
	}
}
