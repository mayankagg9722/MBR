package azure

import (
	"context"
	"fmt"
	"sort"

	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armsubscriptions"
)

// ListLocations returns all available Azure locations for the given subscription.
// Results are sorted alphabetically for stable display.
func ListLocations(ctx context.Context, cred *azidentity.DefaultAzureCredential, subscriptionID string) ([]string, error) {
	client, err := armsubscriptions.NewClient(cred, nil)
	if err != nil {
		return nil, fmt.Errorf("create subscriptions client: %w", err)
	}

	pager := client.NewListLocationsPager(subscriptionID, nil)
	var locations []string

	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list locations: %w", err)
		}
		for _, loc := range page.Value {
			if loc.Name != nil && loc.Metadata != nil && loc.Metadata.RegionType != nil {
				// Only include physical regions (not logical/edge zones).
				if *loc.Metadata.RegionType == "Physical" {
					locations = append(locations, *loc.Name)
				}
			}
		}
	}

	sort.Strings(locations)
	return locations, nil
}

// ListSubscriptions returns all accessible subscription IDs and display names.
func ListSubscriptions(ctx context.Context, cred *azidentity.DefaultAzureCredential) ([]Subscription, error) {
	client, err := armsubscriptions.NewClient(cred, nil)
	if err != nil {
		return nil, fmt.Errorf("create subscriptions client: %w", err)
	}

	pager := client.NewListPager(nil)
	var subs []Subscription

	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list subscriptions: %w", err)
		}
		for _, s := range page.Value {
			if s.SubscriptionID != nil && s.DisplayName != nil {
				subs = append(subs, Subscription{
					ID:          *s.SubscriptionID,
					DisplayName: *s.DisplayName,
				})
			}
		}
	}

	return subs, nil
}

// Subscription holds a simplified Azure subscription reference.
type Subscription struct {
	ID          string
	DisplayName string
}
