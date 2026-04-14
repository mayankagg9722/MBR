package collector

// database.go collects Azure SQL Servers + Databases, CosmosDB accounts, and Redis caches.
//
// Metadata keys written by this file:
//
//	SQL Server:   FQDN, State, AdminLogin, Version, ProvisioningState
//	SQL Database: Status, Edition, ServiceObjective, MaxSizeBytes, ServerName
//	CosmosDB:     Kind, ConsistencyPolicy, EnableMultipleWriteLocations,
//	              DocumentEndpoint, ProvisioningState
//	Redis:        SKU, Capacity, HostName, Port, SSLPort, ProvisioningState, Version

import (
	"context"
	"fmt"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/cosmos/armcosmos/v3"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/redis/armredis/v3"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/sql/armsql"
)

func init() {
	DefaultRegistry.Register(TypeSQLServer, func() Collector { return &sqlServerCollector{} })
	DefaultRegistry.Register(TypeCosmosAccount, func() Collector { return &cosmosCollector{} })
	DefaultRegistry.Register(TypeRedisCache, func() Collector { return &redisCollector{} })
}

// ── SQL Servers + Databases ──────────────────────────────────────────────────

type sqlServerCollector struct{}

func (c *sqlServerCollector) Type() ResourceType { return TypeSQLServer }

func (c *sqlServerCollector) Collect(ctx context.Context, cred *azidentity.DefaultAzureCredential, subscriptionID, location string) ([]Resource, error) {
	client, err := armsql.NewServersClient(subscriptionID, cred, nil)
	if err != nil {
		return nil, fmt.Errorf("create SQL server client: %w", err)
	}

	pager := client.NewListPager(nil)
	var resources []Resource

	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("sql ListServers %s: %w", location, err)
		}
		for _, server := range page.Value {
			if server.Location != nil && strings.EqualFold(*server.Location, location) {
				resources = append(resources, normaliseSQLServer(server, subscriptionID, location))

				// Fetch databases for this server.
				dbs, dbErr := collectSQLDatabases(ctx, cred, subscriptionID, location, PtrToString(server.Name), extractResourceGroup(PtrToString(server.ID)))
				if dbErr == nil {
					resources = append(resources, dbs...)
				}
			}
		}
	}
	return resources, nil
}

func collectSQLDatabases(ctx context.Context, cred *azidentity.DefaultAzureCredential, subscriptionID, location, serverName, resourceGroup string) ([]Resource, error) {
	client, err := armsql.NewDatabasesClient(subscriptionID, cred, nil)
	if err != nil {
		return nil, err
	}

	pager := client.NewListByServerPager(resourceGroup, serverName, nil)
	var resources []Resource

	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		for _, db := range page.Value {
			resources = append(resources, normaliseSQLDatabase(db, subscriptionID, location, serverName))
		}
	}
	return resources, nil
}

func normaliseSQLServer(server *armsql.Server, subID, location string) Resource {
	id := PtrToString(server.ID)
	name := PtrToString(server.Name)
	tags := TagsFromAzure(server.Tags)
	rg := extractResourceGroup(id)

	meta := map[string]string{}
	if server.Properties != nil {
		meta["FQDN"] = PtrToString(server.Properties.FullyQualifiedDomainName)
		meta["State"] = PtrToString(server.Properties.State)
		meta["AdminLogin"] = PtrToString(server.Properties.AdministratorLogin)
		meta["Version"] = PtrToString(server.Properties.Version)
	}

	return Resource{
		ID:             id,
		RawID:          name,
		Type:           TypeSQLServer,
		Name:           name,
		Location:       location,
		SubscriptionID: subID,
		ResourceGroup:  rg,
		Tags:           tags,
		Metadata:       meta,
	}
}

func normaliseSQLDatabase(db *armsql.Database, subID, location, serverName string) Resource {
	id := PtrToString(db.ID)
	name := PtrToString(db.Name)
	tags := TagsFromAzure(db.Tags)
	rg := extractResourceGroup(id)

	meta := map[string]string{
		"ServerName": serverName,
	}
	if db.Properties != nil {
		if db.Properties.Status != nil {
			meta["Status"] = string(*db.Properties.Status)
		}
		if db.Properties.MaxSizeBytes != nil {
			meta["MaxSizeBytes"] = fmt.Sprintf("%d", *db.Properties.MaxSizeBytes)
		}
		if db.Properties.CurrentServiceObjectiveName != nil {
			meta["ServiceObjective"] = *db.Properties.CurrentServiceObjectiveName
		}
	}
	if db.SKU != nil {
		meta["Edition"] = PtrToString(db.SKU.Tier)
	}

	return Resource{
		ID:             id,
		RawID:          name,
		Type:           TypeSQLDatabase,
		Name:           name,
		Location:       location,
		SubscriptionID: subID,
		ResourceGroup:  rg,
		Tags:           tags,
		Metadata:       meta,
	}
}

// ── CosmosDB ─────────────────────────────────────────────────────────────────

type cosmosCollector struct{}

func (c *cosmosCollector) Type() ResourceType { return TypeCosmosAccount }

func (c *cosmosCollector) Collect(ctx context.Context, cred *azidentity.DefaultAzureCredential, subscriptionID, location string) ([]Resource, error) {
	client, err := armcosmos.NewDatabaseAccountsClient(subscriptionID, cred, nil)
	if err != nil {
		return nil, fmt.Errorf("create CosmosDB client: %w", err)
	}

	pager := client.NewListPager(nil)
	var resources []Resource

	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("cosmos ListAccounts %s: %w", location, err)
		}
		for _, acct := range page.Value {
			if acct.Location != nil && strings.EqualFold(*acct.Location, location) {
				resources = append(resources, normaliseCosmosAccount(acct, subscriptionID, location))
			}
		}
	}
	return resources, nil
}

func normaliseCosmosAccount(acct *armcosmos.DatabaseAccountGetResults, subID, location string) Resource {
	id := PtrToString(acct.ID)
	name := PtrToString(acct.Name)
	tags := TagsFromAzure(acct.Tags)
	rg := extractResourceGroup(id)

	meta := map[string]string{}
	if acct.Kind != nil {
		meta["Kind"] = string(*acct.Kind)
	}
	if acct.Properties != nil {
		if acct.Properties.ConsistencyPolicy != nil && acct.Properties.ConsistencyPolicy.DefaultConsistencyLevel != nil {
			meta["ConsistencyPolicy"] = string(*acct.Properties.ConsistencyPolicy.DefaultConsistencyLevel)
		}
		if acct.Properties.EnableMultipleWriteLocations != nil {
			meta["EnableMultipleWriteLocations"] = fmt.Sprintf("%v", *acct.Properties.EnableMultipleWriteLocations)
		}
		meta["DocumentEndpoint"] = PtrToString(acct.Properties.DocumentEndpoint)
		if acct.Properties.ProvisioningState != nil {
			meta["ProvisioningState"] = *acct.Properties.ProvisioningState
		}
	}

	return Resource{
		ID:             id,
		RawID:          name,
		Type:           TypeCosmosAccount,
		Name:           name,
		Location:       location,
		SubscriptionID: subID,
		ResourceGroup:  rg,
		Tags:           tags,
		Metadata:       meta,
	}
}

// ── Redis Cache ──────────────────────────────────────────────────────────────

type redisCollector struct{}

func (c *redisCollector) Type() ResourceType { return TypeRedisCache }

func (c *redisCollector) Collect(ctx context.Context, cred *azidentity.DefaultAzureCredential, subscriptionID, location string) ([]Resource, error) {
	client, err := armredis.NewClient(subscriptionID, cred, nil)
	if err != nil {
		return nil, fmt.Errorf("create Redis client: %w", err)
	}

	pager := client.NewListBySubscriptionPager(nil)
	var resources []Resource

	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("redis ListCaches %s: %w", location, err)
		}
		for _, cache := range page.Value {
			if cache.Location != nil && strings.EqualFold(*cache.Location, location) {
				resources = append(resources, normaliseRedis(cache, subscriptionID, location))
			}
		}
	}
	return resources, nil
}

func normaliseRedis(cache *armredis.ResourceInfo, subID, location string) Resource {
	id := PtrToString(cache.ID)
	name := PtrToString(cache.Name)
	tags := TagsFromAzure(cache.Tags)
	rg := extractResourceGroup(id)

	meta := map[string]string{}

	if cache.Properties != nil {
		meta["HostName"] = PtrToString(cache.Properties.HostName)
		if cache.Properties.Port != nil {
			meta["Port"] = fmt.Sprintf("%d", *cache.Properties.Port)
		}
		if cache.Properties.SSLPort != nil {
			meta["SSLPort"] = fmt.Sprintf("%d", *cache.Properties.SSLPort)
		}
		if cache.Properties.ProvisioningState != nil {
			meta["ProvisioningState"] = string(*cache.Properties.ProvisioningState)
		}
		meta["Version"] = PtrToString(cache.Properties.RedisVersion)
	}

	if cache.Properties != nil && cache.Properties.SKU != nil {
		sku := cache.Properties.SKU
		if sku.Name != nil {
			meta["SKU"] = string(*sku.Name)
		}
		if sku.Capacity != nil {
			meta["Capacity"] = fmt.Sprintf("%d", *sku.Capacity)
		}
	}

	return Resource{
		ID:             id,
		RawID:          name,
		Type:           TypeRedisCache,
		Name:           name,
		Location:       location,
		SubscriptionID: subID,
		ResourceGroup:  rg,
		Tags:           tags,
		Metadata:       meta,
	}
}
