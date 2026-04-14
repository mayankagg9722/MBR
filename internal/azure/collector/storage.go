package collector

// storage.go collects Azure Storage Accounts.
//
// Metadata keys written by this file:
//
//	Storage Account: Kind, SKU, AccessTier, ProvisioningState,
//	                 PrimaryLocation, EnableHTTPSTrafficOnly,
//	                 BlobEndpoint, FileEndpoint, TableEndpoint, QueueEndpoint

import (
	"context"
	"fmt"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/storage/armstorage"
)

func init() {
	DefaultRegistry.Register(TypeStorageAccount, func() Collector { return &storageCollector{} })
}

type storageCollector struct{}

func (c *storageCollector) Type() ResourceType { return TypeStorageAccount }

func (c *storageCollector) Collect(ctx context.Context, cred *azidentity.DefaultAzureCredential, subscriptionID, location string) ([]Resource, error) {
	client, err := armstorage.NewAccountsClient(subscriptionID, cred, nil)
	if err != nil {
		return nil, fmt.Errorf("create Storage client: %w", err)
	}

	pager := client.NewListPager(nil)
	var resources []Resource

	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("storage ListAccounts %s: %w", location, err)
		}
		for _, acct := range page.Value {
			if acct.Location != nil && strings.EqualFold(*acct.Location, location) {
				resources = append(resources, normaliseStorageAccount(acct, subscriptionID, location))
			}
		}
	}
	return resources, nil
}

func normaliseStorageAccount(acct *armstorage.Account, subID, location string) Resource {
	id := PtrToString(acct.ID)
	name := PtrToString(acct.Name)
	tags := TagsFromAzure(acct.Tags)
	rg := extractResourceGroup(id)

	meta := map[string]string{}

	if acct.Kind != nil {
		meta["Kind"] = string(*acct.Kind)
	}

	if acct.SKU != nil && acct.SKU.Name != nil {
		meta["SKU"] = string(*acct.SKU.Name)
	}

	if acct.Properties != nil {
		if acct.Properties.AccessTier != nil {
			meta["AccessTier"] = string(*acct.Properties.AccessTier)
		}
		if acct.Properties.ProvisioningState != nil {
			meta["ProvisioningState"] = string(*acct.Properties.ProvisioningState)
		}
		meta["PrimaryLocation"] = PtrToString(acct.Properties.PrimaryLocation)
		if acct.Properties.EnableHTTPSTrafficOnly != nil {
			meta["EnableHTTPSTrafficOnly"] = fmt.Sprintf("%v", *acct.Properties.EnableHTTPSTrafficOnly)
		}
		if acct.Properties.PrimaryEndpoints != nil {
			ep := acct.Properties.PrimaryEndpoints
			meta["BlobEndpoint"] = PtrToString(ep.Blob)
			meta["FileEndpoint"] = PtrToString(ep.File)
			meta["TableEndpoint"] = PtrToString(ep.Table)
			meta["QueueEndpoint"] = PtrToString(ep.Queue)
		}
	}

	return Resource{
		ID:             id,
		RawID:          name,
		Type:           TypeStorageAccount,
		Name:           name,
		Location:       location,
		SubscriptionID: subID,
		ResourceGroup:  rg,
		Tags:           tags,
		Metadata:       meta,
	}
}
