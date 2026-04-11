package reid

import (
	"testing"
	"time"
)

func TestAdjacencyGraphBasic(t *testing.T) {
	g := NewAdjacencyGraph()
	g.AddEdge("cam-lobby", "cam-hallway", 30*time.Second)
	g.AddEdge("cam-hallway", "cam-office", 45*time.Second)

	// cam-lobby <-> cam-hallway
	if !g.Adjacent("cam-lobby", "cam-hallway") {
		t.Error("cam-lobby and cam-hallway should be adjacent")
	}
	if !g.Adjacent("cam-hallway", "cam-lobby") {
		t.Error("cam-hallway and cam-lobby should be adjacent (reverse)")
	}

	// cam-lobby is NOT adjacent to cam-office.
	if g.Adjacent("cam-lobby", "cam-office") {
		t.Error("cam-lobby and cam-office should NOT be adjacent")
	}

	// Self-adjacency.
	if !g.Adjacent("cam-lobby", "cam-lobby") {
		t.Error("a camera should be adjacent to itself")
	}
}

func TestAdjacencyGraphTransitTime(t *testing.T) {
	g := NewAdjacencyGraph()
	g.AddEdge("A", "B", 60*time.Second)

	d, ok := g.TransitTime("A", "B")
	if !ok || d != 60*time.Second {
		t.Errorf("transit A->B: got %v (%v), want 60s (true)", d, ok)
	}

	d, ok = g.TransitTime("B", "A")
	if !ok || d != 60*time.Second {
		t.Errorf("transit B->A: got %v (%v), want 60s (true)", d, ok)
	}

	_, ok = g.TransitTime("A", "C")
	if ok {
		t.Error("transit A->C should be false (not connected)")
	}
}

func TestAdjacencyGraphNeighbors(t *testing.T) {
	g := NewAdjacencyGraph()
	g.AddEdge("A", "B", 30*time.Second)
	g.AddEdge("A", "C", 45*time.Second)

	neighbors := g.Neighbors("A")
	if len(neighbors) != 2 {
		t.Fatalf("A neighbors: got %d, want 2", len(neighbors))
	}

	// Should contain both B and C.
	found := map[string]bool{}
	for _, n := range neighbors {
		found[n] = true
	}
	if !found["B"] || !found["C"] {
		t.Errorf("neighbors = %v, want B and C", neighbors)
	}
}

func TestAdjacencyGraphCameraCount(t *testing.T) {
	g := NewAdjacencyGraph()
	g.AddEdge("A", "B", 30*time.Second)
	g.AddEdge("B", "C", 45*time.Second)

	if count := g.CameraCount(); count != 3 {
		t.Errorf("camera count = %d, want 3", count)
	}
}

func TestTransitPlausible(t *testing.T) {
	g := NewAdjacencyGraph()
	g.AddEdge("A", "B", 60*time.Second)

	// Enough time has elapsed.
	if !g.TransitPlausible("A", "B", 90*time.Second) {
		t.Error("90s elapsed for 60s transit should be plausible")
	}

	// Exactly transit time.
	if !g.TransitPlausible("A", "B", 60*time.Second) {
		t.Error("60s elapsed for 60s transit should be plausible")
	}

	// Within 20% tolerance.
	if !g.TransitPlausible("A", "B", 50*time.Second) {
		t.Error("50s elapsed for 60s transit should be plausible (within 20% tolerance)")
	}

	// Too fast.
	if g.TransitPlausible("A", "B", 10*time.Second) {
		t.Error("10s elapsed for 60s transit should NOT be plausible")
	}

	// Non-adjacent cameras are always plausible (lower confidence elsewhere).
	if !g.TransitPlausible("A", "Z", 5*time.Second) {
		t.Error("non-adjacent cameras should always be plausible")
	}

	// Same camera is always plausible.
	if !g.TransitPlausible("A", "A", 0) {
		t.Error("same camera should always be plausible")
	}
}
