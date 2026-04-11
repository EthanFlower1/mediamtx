package reid

import "time"

// AdjacencyGraph models the spatial relationships between cameras. Two cameras
// are "adjacent" if a person could reasonably walk between them within a
// configured transit time. The graph is used by the matching engine to boost
// similarity scores when a candidate track's last camera is adjacent to the
// detection's camera.
//
// The graph is undirected by default — AddEdge(a, b, d) also adds (b, a, d).
// Directed edges can be modeled by calling addDirectedEdge directly.
type AdjacencyGraph struct {
	// edges maps camera_id -> set of (neighbor camera_id -> transit time).
	edges map[string]map[string]time.Duration
}

// NewAdjacencyGraph creates an empty camera adjacency graph.
func NewAdjacencyGraph() *AdjacencyGraph {
	return &AdjacencyGraph{
		edges: make(map[string]map[string]time.Duration),
	}
}

// AddEdge adds an undirected edge between two cameras with the expected
// transit time. If the edge already exists it is updated.
func (g *AdjacencyGraph) AddEdge(cameraA, cameraB string, transitTime time.Duration) {
	g.addDirectedEdge(cameraA, cameraB, transitTime)
	g.addDirectedEdge(cameraB, cameraA, transitTime)
}

func (g *AdjacencyGraph) addDirectedEdge(from, to string, transitTime time.Duration) {
	if g.edges[from] == nil {
		g.edges[from] = make(map[string]time.Duration)
	}
	g.edges[from][to] = transitTime
}

// Adjacent reports whether two cameras are directly connected.
func (g *AdjacencyGraph) Adjacent(cameraA, cameraB string) bool {
	if cameraA == cameraB {
		return true
	}
	neighbors, ok := g.edges[cameraA]
	if !ok {
		return false
	}
	_, found := neighbors[cameraB]
	return found
}

// TransitTime returns the expected transit time between two adjacent cameras.
// Returns 0 and false if the cameras are not adjacent.
func (g *AdjacencyGraph) TransitTime(cameraA, cameraB string) (time.Duration, bool) {
	if cameraA == cameraB {
		return 0, true
	}
	neighbors, ok := g.edges[cameraA]
	if !ok {
		return 0, false
	}
	d, found := neighbors[cameraB]
	return d, found
}

// Neighbors returns the set of cameras adjacent to the given camera.
func (g *AdjacencyGraph) Neighbors(cameraID string) []string {
	neighbors, ok := g.edges[cameraID]
	if !ok {
		return nil
	}
	out := make([]string, 0, len(neighbors))
	for cam := range neighbors {
		out = append(out, cam)
	}
	return out
}

// CameraCount returns the number of distinct cameras in the graph.
func (g *AdjacencyGraph) CameraCount() int {
	seen := make(map[string]struct{})
	for cam, neighbors := range g.edges {
		seen[cam] = struct{}{}
		for n := range neighbors {
			seen[n] = struct{}{}
		}
	}
	return len(seen)
}

// TransitPlausible checks whether a person could have moved from cameraA to
// cameraB in the given elapsed time, using the configured transit time as
// the minimum. Returns true if the cameras are adjacent and the elapsed time
// is at least the transit time (with a small tolerance).
func (g *AdjacencyGraph) TransitPlausible(cameraA, cameraB string, elapsed time.Duration) bool {
	if cameraA == cameraB {
		return true
	}
	transit, ok := g.TransitTime(cameraA, cameraB)
	if !ok {
		// Not adjacent — could still be reachable via multi-hop, but
		// we treat non-adjacent as "plausible with lower confidence".
		return true
	}
	// Allow 20% tolerance below the expected transit time to account
	// for faster-than-normal movement.
	minTransit := time.Duration(float64(transit) * 0.8)
	return elapsed >= minTransit
}
