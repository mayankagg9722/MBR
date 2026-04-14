// Package collector defines the universal interface for Azure resource collection
// and the Registry that maps ResourceTypes to their Collector implementations.
//
// Extension pattern: to add a new resource type, create a new file in this
// package, define a struct implementing Collector, and register it via init().
// No changes to any other file are needed.
package collector

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"golang.org/x/sync/errgroup"
)

// ResourceType is a typed string enum identifying an Azure resource category.
// Format is "<service>:<subtype>", e.g. "compute:vm".
type ResourceType string

const (
	TypeVM             ResourceType = "compute:vm"
	TypeDisk           ResourceType = "compute:disk"
	TypeVMSS           ResourceType = "compute:vmss"
	TypeVNet           ResourceType = "network:vnet"
	TypeSubnet         ResourceType = "network:subnet"
	TypeNSG            ResourceType = "network:nsg"
	TypePublicIP       ResourceType = "network:public-ip"
	TypeLoadBalancer   ResourceType = "network:lb"
	TypeSQLServer      ResourceType = "sql:server"
	TypeSQLDatabase    ResourceType = "sql:database"
	TypeCosmosAccount  ResourceType = "cosmos:account"
	TypeRedisCache     ResourceType = "redis:cache"
	TypeStorageAccount ResourceType = "storage:account"
	TypeFunctionApp    ResourceType = "appservice:function"
	TypeAppService     ResourceType = "appservice:webapp"
)

// Resource is the universal normalised representation of any Azure resource.
// All collectors map their SDK-specific output types to this struct so that
// the rest of the codebase (graph, orphan detector, TUI) is decoupled from
// individual Azure service packages.
type Resource struct {
	// ID is the canonical Azure resource ID (ARM ID).
	ID string

	// Type identifies which Azure resource category this is.
	Type ResourceType

	// Name is the human-readable label (resource name).
	Name string

	// Location is the Azure region this resource lives in.
	Location string

	// SubscriptionID is the Azure subscription that owns this resource.
	SubscriptionID string

	// ResourceGroup is the resource group containing this resource.
	ResourceGroup string

	// RawID is the short identifier, e.g. the resource name.
	RawID string

	// Tags are the raw Azure resource tags as key→value pairs.
	Tags map[string]string

	// Metadata holds resource-type-specific fields without requiring type
	// assertions elsewhere. Keys are documented in each collector file.
	Metadata map[string]string
}

// DisplayName returns Name if set, otherwise RawID, otherwise the last
// segment of ID. Safe to call on zero-value Resources.
func (r Resource) DisplayName() string {
	if r.Name != "" {
		return r.Name
	}
	if r.RawID != "" {
		return r.RawID
	}
	return r.ID
}

// Collector is the interface every resource-type collector must satisfy.
// Implementations must be safe to call concurrently from multiple goroutines.
type Collector interface {
	// Type returns the ResourceType this collector handles.
	Type() ResourceType

	// Collect fetches all resources of this type in the given subscription and location.
	Collect(ctx context.Context, cred *azidentity.DefaultAzureCredential, subscriptionID, location string) ([]Resource, error)
}

// Factory constructs a Collector.
// Stored in the Registry so collectors can be instantiated on demand.
type Factory func() Collector

// Registry maps ResourceType → Factory. New resource types register
// themselves in their package's init() function via DefaultRegistry.Register.
type Registry struct {
	mu        sync.RWMutex
	factories map[ResourceType]Factory
}

// NewRegistry returns an empty Registry.
func NewRegistry() *Registry {
	return &Registry{factories: make(map[ResourceType]Factory)}
}

// Register adds a Factory for rt. Panics on duplicate registration so that
// programming errors are caught at startup rather than silently dropped.
func (r *Registry) Register(rt ResourceType, f Factory) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.factories[rt]; exists {
		panic(fmt.Sprintf("collector: duplicate registration for %q", rt))
	}
	r.factories[rt] = f
}

// All returns all registered Factories in a deterministic (sorted) order.
func (r *Registry) All() []Factory {
	r.mu.RLock()
	defer r.mu.RUnlock()

	types := make([]string, 0, len(r.factories))
	for rt := range r.factories {
		types = append(types, string(rt))
	}
	sort.Strings(types)

	out := make([]Factory, 0, len(types))
	for _, t := range types {
		out = append(out, r.factories[ResourceType(t)])
	}
	return out
}

// DefaultRegistry is the package-level singleton populated by init() calls
// in each collector implementation file.
var DefaultRegistry = NewRegistry()

// RunAll executes every collector in reg across every location concurrently,
// merging all results into a single []Resource slice.
//
// maxConcurrency caps the total number of simultaneous Azure API calls.
// A value of 10 is a safe default for most subscriptions.
func RunAll(
	ctx context.Context,
	cred *azidentity.DefaultAzureCredential,
	subscriptionID string,
	locations []string,
	reg *Registry,
	maxConcurrency int,
	progressFn func(location, resourceType string),
) ([]Resource, error) {
	factories := reg.All()
	if len(factories) == 0 || len(locations) == 0 {
		return nil, nil
	}

	sem := make(chan struct{}, maxConcurrency)

	var (
		mu      sync.Mutex
		results []Resource
	)

	eg, ctx := errgroup.WithContext(ctx)

	for _, location := range locations {
		for _, factory := range factories {
			location := location
			factory := factory

			eg.Go(func() error {
				select {
				case sem <- struct{}{}:
					defer func() { <-sem }()
				case <-ctx.Done():
					return ctx.Err()
				}

				c := factory()
				resources, err := c.Collect(ctx, cred, subscriptionID, location)
				if err != nil {
					return fmt.Errorf("[%s/%s] %w", location, c.Type(), err)
				}

				if progressFn != nil {
					progressFn(location, string(c.Type()))
				}

				mu.Lock()
				results = append(results, resources...)
				mu.Unlock()
				return nil
			})
		}
	}

	err := eg.Wait()
	return results, err
}

// tagsFromAzure converts *map[string]*string to map[string]string.
func TagsFromAzure(tags map[string]*string) map[string]string {
	if tags == nil {
		return map[string]string{}
	}
	m := make(map[string]string, len(tags))
	for k, v := range tags {
		if v != nil {
			m[k] = *v
		}
	}
	return m
}

// ptrToString safely dereferences a *string, returning "" if nil.
func PtrToString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
