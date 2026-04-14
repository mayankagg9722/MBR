// Package cost fetches cost data from Azure Cost Management for resources.
// It requires Billing Reader or Cost Management Reader role on the subscription.
package cost

import (
	"context"
	"fmt"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/costmanagement/armcostmanagement/v2"

	"github.com/angsak/mbr/internal/azure/collector"
)

// Result holds the cost lookup outcome for one resource.
type Result struct {
	// USD is the 30-day cost in US dollars.
	USD float64

	// Granularity is "resource" when a per-resource cost was available,
	// "service" when only a service-level aggregate was found, or "none"
	// when Cost Management returned no data.
	Granularity string

	// Err is set when the API call failed.
	Err error
}

// serviceFor maps Azure resource types to Cost Management service names.
var serviceFor = map[collector.ResourceType]string{
	collector.TypeVM:             "Microsoft.Compute",
	collector.TypeDisk:           "Microsoft.Compute",
	collector.TypeVMSS:           "Microsoft.Compute",
	collector.TypeVNet:           "Microsoft.Network",
	collector.TypeSubnet:         "Microsoft.Network",
	collector.TypeNSG:            "Microsoft.Network",
	collector.TypePublicIP:       "Microsoft.Network",
	collector.TypeLoadBalancer:   "Microsoft.Network",
	collector.TypeSQLServer:      "Microsoft.Sql",
	collector.TypeSQLDatabase:    "Microsoft.Sql",
	collector.TypeCosmosAccount:  "Microsoft.DocumentDB",
	collector.TypeRedisCache:     "Microsoft.Cache",
	collector.TypeStorageAccount: "Microsoft.Storage",
	collector.TypeFunctionApp:    "Microsoft.Web",
	collector.TypeAppService:     "Microsoft.Web",
}

// FetchResource tries to get the 30-day cost for a specific Azure resource.
func FetchResource(ctx context.Context, cred *azidentity.DefaultAzureCredential, res collector.Resource) Result {
	scope := fmt.Sprintf("/subscriptions/%s", res.SubscriptionID)

	client, err := armcostmanagement.NewQueryClient(cred, nil)
	if err != nil {
		return Result{Err: fmt.Errorf("create cost client: %w", err)}
	}

	end := time.Now()
	start := end.AddDate(0, -1, 0)

	granularity := armcostmanagement.GranularityTypeDaily
	queryType := armcostmanagement.ExportTypeActualCost
	funcSum := armcostmanagement.FunctionTypeSum

	resID := res.ID
	result, err := client.Usage(ctx, scope, armcostmanagement.QueryDefinition{
		Type:      &queryType,
		Timeframe: toPtr(armcostmanagement.TimeframeTypeCustom),
		TimePeriod: &armcostmanagement.QueryTimePeriod{
			From: &start,
			To:   &end,
		},
		Dataset: &armcostmanagement.QueryDataset{
			Granularity: &granularity,
			Aggregation: map[string]*armcostmanagement.QueryAggregation{
				"totalCost": {
					Name:     toPtr("Cost"),
					Function: &funcSum,
				},
			},
			Filter: &armcostmanagement.QueryFilter{
				Dimensions: &armcostmanagement.QueryComparisonExpression{
					Name:     toPtr("ResourceId"),
					Operator: toPtr(armcostmanagement.QueryOperatorTypeIn),
					Values:   []*string{&resID},
				},
			},
		},
	}, nil)

	if err == nil && result.Properties != nil && result.Properties.Rows != nil {
		total := sumCostRows(result.Properties.Rows)
		if total > 0 {
			return Result{USD: total, Granularity: "resource"}
		}
	}

	// Fall back to service-level cost.
	return FetchService(ctx, cred, res)
}

// FetchService returns the 30-day cost for the Azure service that owns res.
func FetchService(ctx context.Context, cred *azidentity.DefaultAzureCredential, res collector.Resource) Result {
	svc, ok := serviceFor[res.Type]
	if !ok {
		return Result{Granularity: "none"}
	}

	scope := fmt.Sprintf("/subscriptions/%s", res.SubscriptionID)

	client, err := armcostmanagement.NewQueryClient(cred, nil)
	if err != nil {
		return Result{Err: fmt.Errorf("create cost client: %w", err)}
	}

	start := time.Now().AddDate(0, -1, 0)
	end := time.Now()

	granularity2 := armcostmanagement.GranularityTypeDaily
	queryType2 := armcostmanagement.ExportTypeActualCost
	funcSum2 := armcostmanagement.FunctionTypeSum

	result, err := client.Usage(ctx, scope, armcostmanagement.QueryDefinition{
		Type:      &queryType2,
		Timeframe: toPtr(armcostmanagement.TimeframeTypeCustom),
		TimePeriod: &armcostmanagement.QueryTimePeriod{
			From: &start,
			To:   &end,
		},
		Dataset: &armcostmanagement.QueryDataset{
			Granularity: &granularity2,
			Aggregation: map[string]*armcostmanagement.QueryAggregation{
				"totalCost": {
					Name:     toPtr("Cost"),
					Function: &funcSum2,
				},
			},
			Filter: &armcostmanagement.QueryFilter{
				Dimensions: &armcostmanagement.QueryComparisonExpression{
					Name:     toPtr("ServiceName"),
					Operator: toPtr(armcostmanagement.QueryOperatorTypeIn),
					Values:   []*string{&svc},
				},
			},
		},
	}, nil)

	if err != nil {
		return Result{Err: fmt.Errorf("cost management: %w", err)}
	}

	if result.Properties != nil && result.Properties.Rows != nil {
		total := sumCostRows(result.Properties.Rows)
		return Result{USD: total, Granularity: "service"}
	}

	return Result{Granularity: "none"}
}

// ServiceNameFor returns the Azure Cost Management service name for a resource type.
func ServiceNameFor(rt collector.ResourceType) (string, bool) {
	s, ok := serviceFor[rt]
	return s, ok
}

func sumCostRows(rows [][]any) float64 {
	var total float64
	for _, row := range rows {
		if len(row) > 0 {
			if v, ok := row[0].(float64); ok {
				total += v
			}
		}
	}
	return total
}

func toPtr[T any](v T) *T {
	return &v
}
