package orphan

// database.go defines orphan-detection rules for Azure database resources.

import (
	"github.com/angsak/mbr/internal/azure/collector"
	"github.com/angsak/mbr/internal/azure/graph"
)

func init() {
	Rules = append(Rules,
		sqlDBPausedRule{},
	)
}

// ── SQL Database: paused ─────────────────────────────────────────────────────

type sqlDBPausedRule struct{}

func (r sqlDBPausedRule) Name() string                        { return "sql-db-paused" }
func (r sqlDBPausedRule) AppliesTo() collector.ResourceType   { return collector.TypeSQLDatabase }
func (r sqlDBPausedRule) Detect(g *graph.ResourceGraph) {
	for _, n := range g.FilterByType(collector.TypeSQLDatabase) {
		status := n.Resource.Metadata["Status"]
		if status == "Paused" || status == "Offline" {
			n.IsOrphan = true
			n.OrphanReasons = append(n.OrphanReasons,
				"SQL Database is "+status+" (may still incur costs)")
		}
	}
}
