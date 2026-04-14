// Package graph provides the in-memory resource dependency graph that is
// built after all Azure collectors have run. It is the central data structure
// used by the orphan detector, TUI views, and cost attribution.
package graph

import (
	"sort"
	"sync"

	"github.com/angsak/mbr/internal/azure/collector"
)

// Relationship describes the semantic of a directed edge between two nodes.
type Relationship string

const (
	RelContains   Relationship = "contains"
	RelAttachedTo Relationship = "attached-to"
	RelSecuredBy  Relationship = "secured-by"
	RelRoutesVia  Relationship = "routes-via"
	RelBalances   Relationship = "balances"
	RelHostedOn   Relationship = "hosted-on"
	RelBackedBy   Relationship = "backed-by"
)

// Node wraps a collector.Resource with graph-computed metadata.
type Node struct {
	Resource      collector.Resource
	InDegree      int
	OutDegree     int
	CostUSD       float64
	IsOrphan      bool
	OrphanReasons []string
	DangerScore   int
}

// Edge is a typed directed relationship from one node to another.
type Edge struct {
	FromID       string
	ToID         string
	Relationship Relationship
}

// ResourceGraph is the in-memory directed graph.
type ResourceGraph struct {
	mu         sync.RWMutex
	nodes      map[string]*Node
	adjacency  map[string][]Edge
	reverseAdj map[string][]Edge
}

// New returns an empty ResourceGraph ready for population.
func New() *ResourceGraph {
	return &ResourceGraph{
		nodes:      make(map[string]*Node),
		adjacency:  make(map[string][]Edge),
		reverseAdj: make(map[string][]Edge),
	}
}

// AddNode inserts n into the graph, keyed by n.Resource.ID.
func (g *ResourceGraph) AddNode(n *Node) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.nodes[n.Resource.ID] = n
}

// AddEdge inserts a directed edge and updates degree counters.
func (g *ResourceGraph) AddEdge(e Edge) {
	g.mu.Lock()
	defer g.mu.Unlock()

	from, fromOK := g.nodes[e.FromID]
	to, toOK := g.nodes[e.ToID]
	if !fromOK || !toOK {
		return
	}

	g.adjacency[e.FromID] = append(g.adjacency[e.FromID], e)
	g.reverseAdj[e.ToID] = append(g.reverseAdj[e.ToID], e)

	from.OutDegree++
	to.InDegree++
}

// Node returns the *Node for the given resource ID.
func (g *ResourceGraph) Node(id string) (*Node, bool) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	n, ok := g.nodes[id]
	return n, ok
}

// Neighbours returns the nodes directly reachable from id (forward edges).
func (g *ResourceGraph) Neighbours(id string) []*Node {
	g.mu.RLock()
	defer g.mu.RUnlock()

	edges := g.adjacency[id]
	out := make([]*Node, 0, len(edges))
	for _, e := range edges {
		if n, ok := g.nodes[e.ToID]; ok {
			out = append(out, n)
		}
	}
	sortNodes(out)
	return out
}

// Dependents returns the nodes that point to id (reverse edges).
func (g *ResourceGraph) Dependents(id string) []*Node {
	g.mu.RLock()
	defer g.mu.RUnlock()

	edges := g.reverseAdj[id]
	out := make([]*Node, 0, len(edges))
	for _, e := range edges {
		if n, ok := g.nodes[e.FromID]; ok {
			out = append(out, n)
		}
	}
	sortNodes(out)
	return out
}

// AllNodes returns every node in the graph in a stable order.
func (g *ResourceGraph) AllNodes() []*Node {
	g.mu.RLock()
	defer g.mu.RUnlock()

	out := make([]*Node, 0, len(g.nodes))
	for _, n := range g.nodes {
		out = append(out, n)
	}
	sortNodes(out)
	return out
}

// FilterByType returns all nodes of the given ResourceType.
func (g *ResourceGraph) FilterByType(rt collector.ResourceType) []*Node {
	g.mu.RLock()
	defer g.mu.RUnlock()

	var out []*Node
	for _, n := range g.nodes {
		if n.Resource.Type == rt {
			out = append(out, n)
		}
	}
	sortNodes(out)
	return out
}

// Orphans returns all nodes where IsOrphan == true.
func (g *ResourceGraph) Orphans() []*Node {
	g.mu.RLock()
	defer g.mu.RUnlock()

	var out []*Node
	for _, n := range g.nodes {
		if n.IsOrphan {
			out = append(out, n)
		}
	}
	sortNodes(out)
	return out
}

// Len returns the total number of nodes in the graph.
func (g *ResourceGraph) Len() int {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return len(g.nodes)
}

// EdgesFrom returns all outbound edges from the given node ID.
func (g *ResourceGraph) EdgesFrom(id string) []Edge {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return append([]Edge(nil), g.adjacency[id]...)
}

// ComputeDangerScores assigns a DangerScore (0–100) to every node.
func (g *ResourceGraph) ComputeDangerScores() {
	g.mu.Lock()
	defer g.mu.Unlock()

	maxCost := 0.0
	for _, n := range g.nodes {
		if n.CostUSD > maxCost {
			maxCost = n.CostUSD
		}
	}

	for _, n := range g.nodes {
		score := 0
		if n.IsOrphan {
			score += 40
		}
		if maxCost > 0 {
			score += int((n.CostUSD / maxCost) * 60)
		}
		n.DangerScore = score
	}
}

func sortNodes(nodes []*Node) {
	sort.Slice(nodes, func(i, j int) bool {
		return nodes[i].Resource.ID < nodes[j].Resource.ID
	})
}
