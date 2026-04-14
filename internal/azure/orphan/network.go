package orphan

// network.go defines orphan-detection rules for Azure network resources.

import (
	"github.com/angsak/mbr/internal/azure/collector"
	"github.com/angsak/mbr/internal/azure/graph"
)

func init() {
	Rules = append(Rules,
		unassociatedPublicIPRule{},
		emptyNSGRule{},
		emptyLBRule{},
	)
}

// ── Unassociated Public IP ───────────────────────────────────────────────────

type unassociatedPublicIPRule struct{}

func (r unassociatedPublicIPRule) Name() string                        { return "unassociated-public-ip" }
func (r unassociatedPublicIPRule) AppliesTo() collector.ResourceType   { return collector.TypePublicIP }
func (r unassociatedPublicIPRule) Detect(g *graph.ResourceGraph) {
	for _, n := range g.FilterByType(collector.TypePublicIP) {
		assocID := n.Resource.Metadata["AssociatedResourceId"]
		if assocID == "" {
			n.IsOrphan = true
			n.OrphanReasons = append(n.OrphanReasons,
				"Public IP address is not associated with any resource")
		}
	}
}

// ── Empty NSG (no subnets or NICs attached) ──────────────────────────────────

type emptyNSGRule struct{}

func (r emptyNSGRule) Name() string                        { return "empty-nsg" }
func (r emptyNSGRule) AppliesTo() collector.ResourceType   { return collector.TypeNSG }
func (r emptyNSGRule) Detect(g *graph.ResourceGraph) {
	for _, n := range g.FilterByType(collector.TypeNSG) {
		subnetCount := n.Resource.Metadata["AttachedSubnetCount"]
		nicCount := n.Resource.Metadata["AttachedNICCount"]
		if subnetCount == "0" && nicCount == "0" {
			n.IsOrphan = true
			n.OrphanReasons = append(n.OrphanReasons,
				"Network Security Group has no attached subnets or NICs")
		}
	}
}

// ── Empty Load Balancer ──────────────────────────────────────────────────────

type emptyLBRule struct{}

func (r emptyLBRule) Name() string                        { return "empty-lb" }
func (r emptyLBRule) AppliesTo() collector.ResourceType   { return collector.TypeLoadBalancer }
func (r emptyLBRule) Detect(g *graph.ResourceGraph) {
	for _, n := range g.FilterByType(collector.TypeLoadBalancer) {
		beCount := n.Resource.Metadata["BackendPoolCount"]
		if beCount == "0" {
			n.IsOrphan = true
			n.OrphanReasons = append(n.OrphanReasons,
				"Load Balancer has no backend pools configured")
		}
	}
}
