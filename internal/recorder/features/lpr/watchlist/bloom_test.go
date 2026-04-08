package watchlist

import (
	"context"
	"fmt"
	"testing"
)

func TestBloomFilterBasic(t *testing.T) {
	bf := newBloomFilter(1000, 0.001)

	bf.Add("ABC123")
	bf.Add("XY9999")

	if !bf.Test("ABC123") {
		t.Error("ABC123 should be present")
	}
	if !bf.Test("XY9999") {
		t.Error("XY9999 should be present")
	}
	_ = bf.Test("NOTHERE") // may or may not be a false positive — just must not panic
}

func TestBloomFilterParameters(t *testing.T) {
	bf := newBloomFilter(10_000, 0.001)
	if bf.Len() == 0 {
		t.Error("bloom filter bit length should be > 0")
	}
	if bf.NumHashFunctions() == 0 {
		t.Error("bloom filter k should be > 0")
	}
	t.Logf("10k elements at 0.1%% FP: m=%d bits (%.1f KB), k=%d",
		bf.Len(), float64(bf.Len())/8/1024, bf.NumHashFunctions())
}

// BenchmarkBloomAdd measures insert throughput.
func BenchmarkBloomAdd(b *testing.B) {
	bf := newBloomFilter(b.N+1, 0.001)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bf.Add(fmt.Sprintf("PLATE%07d", i))
	}
}

// BenchmarkBloomTest measures lookup throughput (hot cache, all present).
func BenchmarkBloomTest(b *testing.B) {
	const size = 10_000
	bf := newBloomFilter(size, 0.001)
	for i := 0; i < size; i++ {
		bf.Add(fmt.Sprintf("PLATE%05d", i))
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = bf.Test(fmt.Sprintf("PLATE%05d", i%size))
	}
}

// BenchmarkMatcherMatch measures end-to-end Matcher.Match throughput with a
// 10k-entry watchlist backed by the FakeRepository.
func BenchmarkMatcherMatch(b *testing.B) {
	const n = 10_000
	ctx := context.Background()
	repo := NewFakeRepository()
	wl, err := repo.CreateWatchlist(ctx, Watchlist{
		TenantID: tenantA,
		Name:     "bench",
		Type:     TypeDeny,
	})
	if err != nil {
		b.Fatalf("CreateWatchlist: %v", err)
	}
	for i := 0; i < n; i++ {
		_, _ = repo.AddEntry(ctx, tenantA, PlateEntry{
			WatchlistID: wl.ID,
			PlateText:   fmt.Sprintf("PLATE%05d", i),
		})
	}
	m := NewMatcher(tenantA, repo)
	if err := m.RebuildCache(ctx); err != nil {
		b.Fatalf("RebuildCache: %v", err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = m.Match(ctx, tenantA, fmt.Sprintf("PLATE%05d", i%n))
	}
}
