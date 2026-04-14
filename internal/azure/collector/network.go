package collector

// network.go collects Azure VNets, Subnets, NSGs, Public IPs, and Load Balancers.
//
// Metadata keys written by this file:
//
//	VNet:     AddressSpace, ProvisioningState, SubnetCount
//	Subnet:   AddressPrefix, VNetId, NSGId, ProvisioningState
//	NSG:      InboundRuleCount, OutboundRuleCount, ProvisioningState
//	PublicIP: IPAddress, AllocationMethod, SKU, AssociatedResourceId, ProvisioningState
//	LB:      SKU, FrontendIPCount, BackendPoolCount, ProvisioningState

import (
	"context"
	"fmt"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork/v6"
)

func init() {
	DefaultRegistry.Register(TypeVNet, func() Collector { return &vnetCollector{} })
	DefaultRegistry.Register(TypeNSG, func() Collector { return &nsgCollector{} })
	DefaultRegistry.Register(TypePublicIP, func() Collector { return &publicIPCollector{} })
	DefaultRegistry.Register(TypeLoadBalancer, func() Collector { return &lbCollector{} })
}

// ── Virtual Networks ─────────────────────────────────────────────────────────

type vnetCollector struct{}

func (c *vnetCollector) Type() ResourceType { return TypeVNet }

func (c *vnetCollector) Collect(ctx context.Context, cred *azidentity.DefaultAzureCredential, subscriptionID, location string) ([]Resource, error) {
	client, err := armnetwork.NewVirtualNetworksClient(subscriptionID, cred, nil)
	if err != nil {
		return nil, fmt.Errorf("create VNet client: %w", err)
	}

	pager := client.NewListAllPager(nil)
	var resources []Resource

	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("network ListVNets %s: %w", location, err)
		}
		for _, vnet := range page.Value {
			if vnet.Location != nil && strings.EqualFold(*vnet.Location, location) {
				vnetRes, subnetRes := normaliseVNet(vnet, subscriptionID, location)
				resources = append(resources, vnetRes)
				resources = append(resources, subnetRes...)
			}
		}
	}
	return resources, nil
}

func normaliseVNet(vnet *armnetwork.VirtualNetwork, subID, location string) (Resource, []Resource) {
	id := PtrToString(vnet.ID)
	name := PtrToString(vnet.Name)
	tags := TagsFromAzure(vnet.Tags)
	rg := extractResourceGroup(id)

	meta := map[string]string{}
	var subnets []Resource

	if vnet.Properties != nil {
		if vnet.Properties.AddressSpace != nil && len(vnet.Properties.AddressSpace.AddressPrefixes) > 0 {
			var prefixes []string
			for _, p := range vnet.Properties.AddressSpace.AddressPrefixes {
				if p != nil {
					prefixes = append(prefixes, *p)
				}
			}
			meta["AddressSpace"] = strings.Join(prefixes, ", ")
		}
		if vnet.Properties.ProvisioningState != nil {
			meta["ProvisioningState"] = string(*vnet.Properties.ProvisioningState)
		}

		subnetCount := 0
		if vnet.Properties.Subnets != nil {
			subnetCount = len(vnet.Properties.Subnets)
			for _, sn := range vnet.Properties.Subnets {
				subnets = append(subnets, normaliseSubnet(sn, id, subID, location, rg))
			}
		}
		meta["SubnetCount"] = fmt.Sprintf("%d", subnetCount)
	}

	vnetRes := Resource{
		ID:             id,
		RawID:          name,
		Type:           TypeVNet,
		Name:           name,
		Location:       location,
		SubscriptionID: subID,
		ResourceGroup:  rg,
		Tags:           tags,
		Metadata:       meta,
	}
	return vnetRes, subnets
}

func normaliseSubnet(sn *armnetwork.Subnet, vnetID, subID, location, rg string) Resource {
	id := PtrToString(sn.ID)
	name := PtrToString(sn.Name)

	meta := map[string]string{
		"VNetId": vnetID,
	}

	if sn.Properties != nil {
		meta["AddressPrefix"] = PtrToString(sn.Properties.AddressPrefix)
		if sn.Properties.ProvisioningState != nil {
			meta["ProvisioningState"] = string(*sn.Properties.ProvisioningState)
		}
		if sn.Properties.NetworkSecurityGroup != nil && sn.Properties.NetworkSecurityGroup.ID != nil {
			meta["NSGId"] = *sn.Properties.NetworkSecurityGroup.ID
		}
	}

	return Resource{
		ID:             id,
		RawID:          name,
		Type:           TypeSubnet,
		Name:           name,
		Location:       location,
		SubscriptionID: subID,
		ResourceGroup:  rg,
		Metadata:       meta,
	}
}

// ── Network Security Groups ──────────────────────────────────────────────────

type nsgCollector struct{}

func (c *nsgCollector) Type() ResourceType { return TypeNSG }

func (c *nsgCollector) Collect(ctx context.Context, cred *azidentity.DefaultAzureCredential, subscriptionID, location string) ([]Resource, error) {
	client, err := armnetwork.NewSecurityGroupsClient(subscriptionID, cred, nil)
	if err != nil {
		return nil, fmt.Errorf("create NSG client: %w", err)
	}

	pager := client.NewListAllPager(nil)
	var resources []Resource

	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("network ListNSGs %s: %w", location, err)
		}
		for _, nsg := range page.Value {
			if nsg.Location != nil && strings.EqualFold(*nsg.Location, location) {
				resources = append(resources, normaliseNSG(nsg, subscriptionID, location))
			}
		}
	}
	return resources, nil
}

func normaliseNSG(nsg *armnetwork.SecurityGroup, subID, location string) Resource {
	id := PtrToString(nsg.ID)
	name := PtrToString(nsg.Name)
	tags := TagsFromAzure(nsg.Tags)
	rg := extractResourceGroup(id)

	meta := map[string]string{}

	if nsg.Properties != nil {
		if nsg.Properties.ProvisioningState != nil {
			meta["ProvisioningState"] = string(*nsg.Properties.ProvisioningState)
		}
		inCount := 0
		if nsg.Properties.SecurityRules != nil {
			inCount = len(nsg.Properties.SecurityRules)
		}
		meta["SecurityRuleCount"] = fmt.Sprintf("%d", inCount)

		// Count attached subnets and NICs.
		subnetCount := 0
		if nsg.Properties.Subnets != nil {
			subnetCount = len(nsg.Properties.Subnets)
		}
		nicCount := 0
		if nsg.Properties.NetworkInterfaces != nil {
			nicCount = len(nsg.Properties.NetworkInterfaces)
		}
		meta["AttachedSubnetCount"] = fmt.Sprintf("%d", subnetCount)
		meta["AttachedNICCount"] = fmt.Sprintf("%d", nicCount)
	}

	return Resource{
		ID:             id,
		RawID:          name,
		Type:           TypeNSG,
		Name:           name,
		Location:       location,
		SubscriptionID: subID,
		ResourceGroup:  rg,
		Tags:           tags,
		Metadata:       meta,
	}
}

// ── Public IP Addresses ──────────────────────────────────────────────────────

type publicIPCollector struct{}

func (c *publicIPCollector) Type() ResourceType { return TypePublicIP }

func (c *publicIPCollector) Collect(ctx context.Context, cred *azidentity.DefaultAzureCredential, subscriptionID, location string) ([]Resource, error) {
	client, err := armnetwork.NewPublicIPAddressesClient(subscriptionID, cred, nil)
	if err != nil {
		return nil, fmt.Errorf("create PublicIP client: %w", err)
	}

	pager := client.NewListAllPager(nil)
	var resources []Resource

	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("network ListPublicIPs %s: %w", location, err)
		}
		for _, pip := range page.Value {
			if pip.Location != nil && strings.EqualFold(*pip.Location, location) {
				resources = append(resources, normalisePublicIP(pip, subscriptionID, location))
			}
		}
	}
	return resources, nil
}

func normalisePublicIP(pip *armnetwork.PublicIPAddress, subID, location string) Resource {
	id := PtrToString(pip.ID)
	name := PtrToString(pip.Name)
	tags := TagsFromAzure(pip.Tags)
	rg := extractResourceGroup(id)

	meta := map[string]string{}

	if pip.Properties != nil {
		meta["IPAddress"] = PtrToString(pip.Properties.IPAddress)
		if pip.Properties.PublicIPAllocationMethod != nil {
			meta["AllocationMethod"] = string(*pip.Properties.PublicIPAllocationMethod)
		}
		if pip.Properties.ProvisioningState != nil {
			meta["ProvisioningState"] = string(*pip.Properties.ProvisioningState)
		}
		if pip.Properties.IPConfiguration != nil && pip.Properties.IPConfiguration.ID != nil {
			meta["AssociatedResourceId"] = *pip.Properties.IPConfiguration.ID
		}
	}

	if pip.SKU != nil && pip.SKU.Name != nil {
		meta["SKU"] = string(*pip.SKU.Name)
	}

	return Resource{
		ID:             id,
		RawID:          name,
		Type:           TypePublicIP,
		Name:           name,
		Location:       location,
		SubscriptionID: subID,
		ResourceGroup:  rg,
		Tags:           tags,
		Metadata:       meta,
	}
}

// ── Load Balancers ───────────────────────────────────────────────────────────

type lbCollector struct{}

func (c *lbCollector) Type() ResourceType { return TypeLoadBalancer }

func (c *lbCollector) Collect(ctx context.Context, cred *azidentity.DefaultAzureCredential, subscriptionID, location string) ([]Resource, error) {
	client, err := armnetwork.NewLoadBalancersClient(subscriptionID, cred, nil)
	if err != nil {
		return nil, fmt.Errorf("create LB client: %w", err)
	}

	pager := client.NewListAllPager(nil)
	var resources []Resource

	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("network ListLBs %s: %w", location, err)
		}
		for _, lb := range page.Value {
			if lb.Location != nil && strings.EqualFold(*lb.Location, location) {
				resources = append(resources, normaliseLB(lb, subscriptionID, location))
			}
		}
	}
	return resources, nil
}

func normaliseLB(lb *armnetwork.LoadBalancer, subID, location string) Resource {
	id := PtrToString(lb.ID)
	name := PtrToString(lb.Name)
	tags := TagsFromAzure(lb.Tags)
	rg := extractResourceGroup(id)

	meta := map[string]string{}

	if lb.SKU != nil && lb.SKU.Name != nil {
		meta["SKU"] = string(*lb.SKU.Name)
	}

	if lb.Properties != nil {
		if lb.Properties.ProvisioningState != nil {
			meta["ProvisioningState"] = string(*lb.Properties.ProvisioningState)
		}
		feCount := 0
		if lb.Properties.FrontendIPConfigurations != nil {
			feCount = len(lb.Properties.FrontendIPConfigurations)
		}
		beCount := 0
		if lb.Properties.BackendAddressPools != nil {
			beCount = len(lb.Properties.BackendAddressPools)
		}
		meta["FrontendIPCount"] = fmt.Sprintf("%d", feCount)
		meta["BackendPoolCount"] = fmt.Sprintf("%d", beCount)
	}

	return Resource{
		ID:             id,
		RawID:          name,
		Type:           TypeLoadBalancer,
		Name:           name,
		Location:       location,
		SubscriptionID: subID,
		ResourceGroup:  rg,
		Tags:           tags,
		Metadata:       meta,
	}
}
