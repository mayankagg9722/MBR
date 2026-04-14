package collector

// compute.go collects Azure VMs, Managed Disks, and VM Scale Sets.
//
// Metadata keys written by this file:
//
//	VM:   PowerState, VMSize, OsType, PrivateIP, PublicIP, VNetId, SubnetId, NSGId,
//	      AdminUsername, ImagePublisher, ImageOffer, ImageSKU
//	Disk: State, DiskSizeGB, SKU, OsType, ManagedBy (attached VM ID)
//	VMSS: SKU, Capacity, ProvisioningState

import (
	"context"
	"fmt"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute/v6"
)

// ── Virtual Machines ─────────────────────────────────────────────────────────

type vmCollector struct{}

func init() {
	DefaultRegistry.Register(TypeVM, func() Collector { return &vmCollector{} })
	DefaultRegistry.Register(TypeDisk, func() Collector { return &diskCollector{} })
	DefaultRegistry.Register(TypeVMSS, func() Collector { return &vmssCollector{} })
}

func (c *vmCollector) Type() ResourceType { return TypeVM }

func (c *vmCollector) Collect(ctx context.Context, cred *azidentity.DefaultAzureCredential, subscriptionID, location string) ([]Resource, error) {
	client, err := armcompute.NewVirtualMachinesClient(subscriptionID, cred, nil)
	if err != nil {
		return nil, fmt.Errorf("create VM client: %w", err)
	}

	pager := client.NewListAllPager(nil)
	var resources []Resource

	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("compute ListVMs %s: %w", location, err)
		}
		for _, vm := range page.Value {
			if vm.Location != nil && strings.EqualFold(*vm.Location, location) {
				resources = append(resources, normaliseVM(vm, subscriptionID, location))
			}
		}
	}
	return resources, nil
}

func normaliseVM(vm *armcompute.VirtualMachine, subID, location string) Resource {
	id := PtrToString(vm.ID)
	name := PtrToString(vm.Name)
	tags := TagsFromAzure(vm.Tags)
	rg := extractResourceGroup(id)

	meta := map[string]string{}

	if vm.Properties != nil {
		if vm.Properties.HardwareProfile != nil && vm.Properties.HardwareProfile.VMSize != nil {
			meta["VMSize"] = string(*vm.Properties.HardwareProfile.VMSize)
		}
		if vm.Properties.StorageProfile != nil && vm.Properties.StorageProfile.OSDisk != nil &&
			vm.Properties.StorageProfile.OSDisk.OSType != nil {
			meta["OsType"] = string(*vm.Properties.StorageProfile.OSDisk.OSType)
		}
		if vm.Properties.StorageProfile != nil && vm.Properties.StorageProfile.ImageReference != nil {
			ir := vm.Properties.StorageProfile.ImageReference
			meta["ImagePublisher"] = PtrToString(ir.Publisher)
			meta["ImageOffer"] = PtrToString(ir.Offer)
			meta["ImageSKU"] = PtrToString(ir.SKU)
		}
		if vm.Properties.OSProfile != nil {
			meta["AdminUsername"] = PtrToString(vm.Properties.OSProfile.AdminUsername)
		}
		if vm.Properties.ProvisioningState != nil {
			meta["ProvisioningState"] = *vm.Properties.ProvisioningState
		}

		// Extract network interface IDs.
		if vm.Properties.NetworkProfile != nil {
			var nicIDs []string
			for _, nic := range vm.Properties.NetworkProfile.NetworkInterfaces {
				if nic.ID != nil {
					nicIDs = append(nicIDs, *nic.ID)
				}
			}
			if len(nicIDs) > 0 {
				meta["NetworkInterfaceIds"] = strings.Join(nicIDs, ",")
			}
		}

		// Power state from instance view statuses.
		if vm.Properties.InstanceView != nil {
			for _, s := range vm.Properties.InstanceView.Statuses {
				if s.Code != nil && strings.HasPrefix(*s.Code, "PowerState/") {
					meta["PowerState"] = strings.TrimPrefix(*s.Code, "PowerState/")
				}
			}
		}
	}

	return Resource{
		ID:             id,
		RawID:          name,
		Type:           TypeVM,
		Name:           name,
		Location:       location,
		SubscriptionID: subID,
		ResourceGroup:  rg,
		Tags:           tags,
		Metadata:       meta,
	}
}

// ── Managed Disks ────────────────────────────────────────────────────────────

type diskCollector struct{}

func (c *diskCollector) Type() ResourceType { return TypeDisk }

func (c *diskCollector) Collect(ctx context.Context, cred *azidentity.DefaultAzureCredential, subscriptionID, location string) ([]Resource, error) {
	client, err := armcompute.NewDisksClient(subscriptionID, cred, nil)
	if err != nil {
		return nil, fmt.Errorf("create disk client: %w", err)
	}

	pager := client.NewListPager(nil)
	var resources []Resource

	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("compute ListDisks %s: %w", location, err)
		}
		for _, disk := range page.Value {
			if disk.Location != nil && strings.EqualFold(*disk.Location, location) {
				resources = append(resources, normaliseDisk(disk, subscriptionID, location))
			}
		}
	}
	return resources, nil
}

func normaliseDisk(disk *armcompute.Disk, subID, location string) Resource {
	id := PtrToString(disk.ID)
	name := PtrToString(disk.Name)
	tags := TagsFromAzure(disk.Tags)
	rg := extractResourceGroup(id)

	meta := map[string]string{}

	if disk.Properties != nil {
		if disk.Properties.DiskState != nil {
			meta["State"] = string(*disk.Properties.DiskState)
		}
		if disk.Properties.DiskSizeGB != nil {
			meta["DiskSizeGB"] = fmt.Sprintf("%d", *disk.Properties.DiskSizeGB)
		}
		if disk.Properties.OSType != nil {
			meta["OsType"] = string(*disk.Properties.OSType)
		}
		meta["ManagedBy"] = PtrToString(disk.ManagedBy)
	}

	if disk.SKU != nil && disk.SKU.Name != nil {
		meta["SKU"] = string(*disk.SKU.Name)
	}

	return Resource{
		ID:             id,
		RawID:          name,
		Type:           TypeDisk,
		Name:           name,
		Location:       location,
		SubscriptionID: subID,
		ResourceGroup:  rg,
		Tags:           tags,
		Metadata:       meta,
	}
}

// ── VM Scale Sets ────────────────────────────────────────────────────────────

type vmssCollector struct{}

func (c *vmssCollector) Type() ResourceType { return TypeVMSS }

func (c *vmssCollector) Collect(ctx context.Context, cred *azidentity.DefaultAzureCredential, subscriptionID, location string) ([]Resource, error) {
	client, err := armcompute.NewVirtualMachineScaleSetsClient(subscriptionID, cred, nil)
	if err != nil {
		return nil, fmt.Errorf("create VMSS client: %w", err)
	}

	pager := client.NewListAllPager(nil)
	var resources []Resource

	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("compute ListVMSS %s: %w", location, err)
		}
		for _, vmss := range page.Value {
			if vmss.Location != nil && strings.EqualFold(*vmss.Location, location) {
				resources = append(resources, normaliseVMSS(vmss, subscriptionID, location))
			}
		}
	}
	return resources, nil
}

func normaliseVMSS(vmss *armcompute.VirtualMachineScaleSet, subID, location string) Resource {
	id := PtrToString(vmss.ID)
	name := PtrToString(vmss.Name)
	tags := TagsFromAzure(vmss.Tags)
	rg := extractResourceGroup(id)

	meta := map[string]string{}

	if vmss.SKU != nil {
		meta["SKU"] = PtrToString(vmss.SKU.Name)
		if vmss.SKU.Capacity != nil {
			meta["Capacity"] = fmt.Sprintf("%d", *vmss.SKU.Capacity)
		}
	}

	if vmss.Properties != nil && vmss.Properties.ProvisioningState != nil {
		meta["ProvisioningState"] = *vmss.Properties.ProvisioningState
	}

	return Resource{
		ID:             id,
		RawID:          name,
		Type:           TypeVMSS,
		Name:           name,
		Location:       location,
		SubscriptionID: subID,
		ResourceGroup:  rg,
		Tags:           tags,
		Metadata:       meta,
	}
}

// ── Helpers ──────────────────────────────────────────────────────────────────

// extractResourceGroup extracts the resource group name from an Azure ARM ID.
func extractResourceGroup(armID string) string {
	parts := strings.Split(armID, "/")
	for i, p := range parts {
		if strings.EqualFold(p, "resourceGroups") && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	return ""
}
