// Package orphan implements rules that detect unused ("orphaned") Azure resources.
//
// Extension pattern: create a new file in this package, define a struct
// implementing OrphanRule, and append it to Rules in an init() function.
package orphan

import (
	"github.com/angsak/mbr/internal/azure/collector"
	"github.com/angsak/mbr/internal/azure/graph"
)

// OrphanRule examines the resource graph and marks nodes as orphans.
type OrphanRule interface {
	Name() string
	AppliesTo() collector.ResourceType
	Detect(g *graph.ResourceGraph)
}

// Rules is the package-level registry of all orphan detection rules.
var Rules []OrphanRule

// RunAll applies every registered OrphanRule to the graph in order.
func RunAll(g *graph.ResourceGraph) {
	for _, rule := range Rules {
		rule.Detect(g)
	}
}
