package graph

// builder.go constructs a ResourceGraph from a flat []collector.Resource slice.
//
// Each edge type is implemented as a separate unexported function.
// To add a new relationship type, write a new function and append it to rules.

import (
	"strings"

	"github.com/angsak/mbr/internal/azure/collector"
)

type edgeRule func(byID map[string]*collector.Resource, g *ResourceGraph)

// BuildGraph constructs a ResourceGraph from a flat list of Azure resources.
//
// Edge rules:
//   - Subnet → VNet (contains)
//   - Disk → VM (attached-to, via ManagedBy)
//   - Subnet → NSG (secured-by, via NSGId)
//   - VM → Subnet (contains, via NIC → Subnet mapping)
func BuildGraph(resources []collector.Resource) *ResourceGraph {
	g := New()

	byID := make(map[string]*collector.Resource, len(resources))

	for i := range resources {
		r := &resources[i]
		byID[r.ID] = r
		// Also index by lowercase ID for case-insensitive Azure ARM ID matching.
		byID[strings.ToLower(r.ID)] = r
		g.AddNode(&Node{Resource: *r})
	}

	rules := []edgeRule{
		ruleSubnetToVNet,
		ruleDiskToVM,
		ruleSubnetToNSG,
	}
	for _, rule := range rules {
		rule(byID, g)
	}

	return g
}

// ── Edge rules ───────────────────────────────────────────────────────────────

// ruleSubnetToVNet draws a "contains" edge from VNet → Subnet.
func ruleSubnetToVNet(byID map[string]*collector.Resource, g *ResourceGraph) {
	for _, n := range g.FilterByType(collector.TypeSubnet) {
		vnetID := n.Resource.Metadata["VNetId"]
		if vnetID == "" {
			continue
		}
		vnet, ok := byID[strings.ToLower(vnetID)]
		if !ok {
			continue
		}
		g.AddEdge(Edge{
			FromID:       vnet.ID,
			ToID:         n.Resource.ID,
			Relationship: RelContains,
		})
	}
}

// ruleDiskToVM draws an "attached-to" edge from Disk → VM using ManagedBy.
func ruleDiskToVM(byID map[string]*collector.Resource, g *ResourceGraph) {
	for _, n := range g.FilterByType(collector.TypeDisk) {
		managedBy := n.Resource.Metadata["ManagedBy"]
		if managedBy == "" {
			continue
		}
		vm, ok := byID[strings.ToLower(managedBy)]
		if !ok {
			continue
		}
		g.AddEdge(Edge{
			FromID:       n.Resource.ID,
			ToID:         vm.ID,
			Relationship: RelAttachedTo,
		})
	}
}

// ruleSubnetToNSG draws a "secured-by" edge from Subnet → NSG.
func ruleSubnetToNSG(byID map[string]*collector.Resource, g *ResourceGraph) {
	for _, n := range g.FilterByType(collector.TypeSubnet) {
		nsgID := n.Resource.Metadata["NSGId"]
		if nsgID == "" {
			continue
		}
		nsg, ok := byID[strings.ToLower(nsgID)]
		if !ok {
			continue
		}
		g.AddEdge(Edge{
			FromID:       n.Resource.ID,
			ToID:         nsg.ID,
			Relationship: RelSecuredBy,
		})
	}
}
