package orphan

// compute.go defines orphan-detection rules for Azure compute resources.

import (
	"github.com/angsak/mbr/internal/azure/collector"
	"github.com/angsak/mbr/internal/azure/graph"
)

func init() {
	Rules = append(Rules,
		vmDeallocatedRule{},
		unattachedDiskRule{},
		vmssZeroCapacityRule{},
	)
}

// ── VM: deallocated ──────────────────────────────────────────────────────────

type vmDeallocatedRule struct{}

func (r vmDeallocatedRule) Name() string                        { return "vm-deallocated" }
func (r vmDeallocatedRule) AppliesTo() collector.ResourceType   { return collector.TypeVM }
func (r vmDeallocatedRule) Detect(g *graph.ResourceGraph) {
	for _, n := range g.FilterByType(collector.TypeVM) {
		state := n.Resource.Metadata["PowerState"]
		if state == "deallocated" || state == "stopped" {
			n.IsOrphan = true
			n.OrphanReasons = append(n.OrphanReasons,
				"Virtual Machine is "+state+" (still incurs storage costs)")
		}
	}
}

// ── Unattached Managed Disk ──────────────────────────────────────────────────

type unattachedDiskRule struct{}

func (r unattachedDiskRule) Name() string                        { return "unattached-disk" }
func (r unattachedDiskRule) AppliesTo() collector.ResourceType   { return collector.TypeDisk }
func (r unattachedDiskRule) Detect(g *graph.ResourceGraph) {
	for _, n := range g.FilterByType(collector.TypeDisk) {
		state := n.Resource.Metadata["State"]
		managedBy := n.Resource.Metadata["ManagedBy"]

		// Azure disk state "Unattached" and no VM reference means orphaned.
		if state == "Unattached" && managedBy == "" {
			n.IsOrphan = true
			n.OrphanReasons = append(n.OrphanReasons,
				"Managed Disk is unattached (not associated with any VM)")
		}
	}
}

// ── VMSS: zero capacity ──────────────────────────────────────────────────────

type vmssZeroCapacityRule struct{}

func (r vmssZeroCapacityRule) Name() string                        { return "vmss-zero-capacity" }
func (r vmssZeroCapacityRule) AppliesTo() collector.ResourceType   { return collector.TypeVMSS }
func (r vmssZeroCapacityRule) Detect(g *graph.ResourceGraph) {
	for _, n := range g.FilterByType(collector.TypeVMSS) {
		capacity := n.Resource.Metadata["Capacity"]
		if capacity == "0" {
			n.IsOrphan = true
			n.OrphanReasons = append(n.OrphanReasons,
				"VM Scale Set has 0 capacity (no instances running)")
		}
	}
}
