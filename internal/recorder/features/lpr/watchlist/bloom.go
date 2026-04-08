package watchlist

import (
	"hash/fnv"
	"math"
)

// bloomFilter is a simple Counting-free Bloom filter backed by a bit array.
// It uses two independent hash functions derived from FNV-1a to produce k
// bit positions. The implementation is deliberately self-contained (no
// external dep) and is correct for the 10k-entry, 0.1%-FP use case this
// package targets.
//
// Thread-safety: bloomFilter is NOT goroutine-safe. Callers must hold the
// enclosing Matcher.mu lock when calling Add or Test.
type bloomFilter struct {
	bits []uint64 // bit vector, stored as 64-bit words
	m    uint     // total bit count (rounded up to 64-bit boundary)
	k    uint     // number of hash probes per element
}

// newBloomFilter creates a bloom filter sized for n expected elements at the
// given false-positive probability p (range 0 < p < 1).
//
// Optimal m = -n*ln(p) / (ln(2)^2)
// Optimal k = (m/n)*ln(2)
func newBloomFilter(n int, p float64) *bloomFilter {
	if n <= 0 {
		n = 1
	}
	if p <= 0 || p >= 1 {
		p = 0.001
	}
	m := uint(math.Ceil(-float64(n) * math.Log(p) / (math.Ln2 * math.Ln2)))
	// Round up to the next 64-bit word boundary.
	m = ((m + 63) / 64) * 64
	k := uint(math.Round(float64(m) / float64(n) * math.Ln2))
	if k < 1 {
		k = 1
	}
	return &bloomFilter{
		bits: make([]uint64, m/64),
		m:    m,
		k:    k,
	}
}

// Add inserts s into the filter.
func (bf *bloomFilter) Add(s string) {
	h1, h2 := hashPair(s)
	for i := uint(0); i < bf.k; i++ {
		pos := (h1 + i*h2) % bf.m
		bf.bits[pos/64] |= 1 << (pos % 64)
	}
}

// Test returns true if s was possibly added, false if it was definitely not.
func (bf *bloomFilter) Test(s string) bool {
	h1, h2 := hashPair(s)
	for i := uint(0); i < bf.k; i++ {
		pos := (h1 + i*h2) % bf.m
		if bf.bits[pos/64]&(1<<(pos%64)) == 0 {
			return false
		}
	}
	return true
}

// hashPair returns two independent 64-bit hashes of s using FNV-1a variants.
// The double-hashing scheme (h1 + i*h2) generates k probes from two hashes,
// which achieves near-optimal performance without a separate hash library.
func hashPair(s string) (uint, uint) {
	h1 := fnv.New64a()
	_, _ = h1.Write([]byte(s))
	sum1 := h1.Sum64()

	// Produce h2 by XOR-mixing with a FNV offset constant to get an
	// independent value without a second full hash pass.
	h2 := fnv.New64()
	_, _ = h2.Write([]byte(s))
	sum2 := h2.Sum64()

	return uint(sum1), uint(sum2 | 1) // ensure h2 is odd to cover all bit positions
}

// Len returns the bit-vector size in bits. Useful for telemetry.
func (bf *bloomFilter) Len() uint { return bf.m }

// NumHashFunctions returns k. Useful for telemetry.
func (bf *bloomFilter) NumHashFunctions() uint { return bf.k }
