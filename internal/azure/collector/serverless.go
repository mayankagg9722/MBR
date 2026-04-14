package collector

// serverless.go collects Azure Function Apps and App Service Web Apps.
//
// Metadata keys written by this file:
//
//	Function App: Kind, State, DefaultHostName, Runtime, ProvisioningState
//	App Service:  Kind, State, DefaultHostName, ProvisioningState

import (
	"context"
	"fmt"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/appservice/armappservice/v4"
)

func init() {
	DefaultRegistry.Register(TypeFunctionApp, func() Collector { return &functionAppCollector{} })
	DefaultRegistry.Register(TypeAppService, func() Collector { return &appServiceCollector{} })
}

// ── Function Apps ────────────────────────────────────────────────────────────

type functionAppCollector struct{}

func (c *functionAppCollector) Type() ResourceType { return TypeFunctionApp }

func (c *functionAppCollector) Collect(ctx context.Context, cred *azidentity.DefaultAzureCredential, subscriptionID, location string) ([]Resource, error) {
	client, err := armappservice.NewWebAppsClient(subscriptionID, cred, nil)
	if err != nil {
		return nil, fmt.Errorf("create WebApps client: %w", err)
	}

	pager := client.NewListPager(nil)
	var resources []Resource

	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("appservice ListWebApps %s: %w", location, err)
		}
		for _, app := range page.Value {
			if app.Location != nil && strings.EqualFold(*app.Location, location) {
				kind := PtrToString(app.Kind)
				if strings.Contains(strings.ToLower(kind), "functionapp") {
					resources = append(resources, normaliseApp(app, subscriptionID, location, TypeFunctionApp))
				}
			}
		}
	}
	return resources, nil
}

// ── App Service (Web Apps) ───────────────────────────────────────────────────

type appServiceCollector struct{}

func (c *appServiceCollector) Type() ResourceType { return TypeAppService }

func (c *appServiceCollector) Collect(ctx context.Context, cred *azidentity.DefaultAzureCredential, subscriptionID, location string) ([]Resource, error) {
	client, err := armappservice.NewWebAppsClient(subscriptionID, cred, nil)
	if err != nil {
		return nil, fmt.Errorf("create WebApps client: %w", err)
	}

	pager := client.NewListPager(nil)
	var resources []Resource

	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("appservice ListWebApps %s: %w", location, err)
		}
		for _, app := range page.Value {
			if app.Location != nil && strings.EqualFold(*app.Location, location) {
				kind := PtrToString(app.Kind)
				if !strings.Contains(strings.ToLower(kind), "functionapp") {
					resources = append(resources, normaliseApp(app, subscriptionID, location, TypeAppService))
				}
			}
		}
	}
	return resources, nil
}

// normaliseApp converts an Azure Site (Web App or Function App) to the canonical Resource.
func normaliseApp(app *armappservice.Site, subID, location string, rt ResourceType) Resource {
	id := PtrToString(app.ID)
	name := PtrToString(app.Name)
	tags := TagsFromAzure(app.Tags)
	rg := extractResourceGroup(id)

	meta := map[string]string{
		"Kind": PtrToString(app.Kind),
	}

	if app.Properties != nil {
		meta["State"] = PtrToString(app.Properties.State)
		meta["DefaultHostName"] = PtrToString(app.Properties.DefaultHostName)
	}

	return Resource{
		ID:             id,
		RawID:          name,
		Type:           rt,
		Name:           name,
		Location:       location,
		SubscriptionID: subID,
		ResourceGroup:  rg,
		Tags:           tags,
		Metadata:       meta,
	}
}
