// msr is a CLI tool for browsing Azure resources across all locations,
// visualising resource relationships, detecting orphans, and analysing costs.
// msr stands for "Make Satya Rich" — the Azure counterpart to mbr ("Make Bezos Rich").
//
// Usage:
//
//	msr                                # Launch interactive TUI (default)
//	msr scan                           # Scan and print resources to stdout
//	msr scan --location eastus         # Scan a single location
//	msr orphans                        # List orphaned resources (text/JSON)
//	msr version                        # Print version info
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	azurepkg "github.com/angsak/mbr/internal/azure"
	"github.com/angsak/mbr/internal/azure/collector"
	"github.com/angsak/mbr/internal/azure/graph"
	"github.com/angsak/mbr/internal/azure/orphan"
	"github.com/angsak/mbr/internal/version"
	"github.com/angsak/mbr/tui"

	// Blank imports register all collectors and orphan rules via init().
	_ "github.com/angsak/mbr/internal/azure/collector"
	_ "github.com/angsak/mbr/internal/azure/orphan"
)

// Global flags shared across all commands.
var (
	flagSubscription string
	flagLocation     string
	flagOutput       string // "text" or "json"
)

func main() {
	root := buildRootCmd()
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func buildRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "msr",
		Short: "Azure Resource Browser — visualise and manage Azure resources",
		Long: `msr (Make Satya Rich) scans your Azure subscription across all locations,
maps resource relationships, detects orphaned resources, and shows cost
breakdowns — all in an interactive terminal UI.

Run without a subcommand to launch the TUI.`,
		RunE: runTUI,
	}

	root.PersistentFlags().StringVar(&flagSubscription, "subscription", "", "Azure subscription ID (default: AZURE_SUBSCRIPTION_ID env var)")
	root.PersistentFlags().StringVar(&flagLocation, "location", "", "Limit scan to one Azure location (default: all locations)")
	root.PersistentFlags().StringVar(&flagOutput, "output", "text", "Output format: text or json")

	root.AddCommand(buildScanCmd())
	root.AddCommand(buildOrphansCmd())
	root.AddCommand(buildVersionCmd())

	return root
}

// ── TUI (default command) ─────────────────────────────────────────────────────

func runTUI(cmd *cobra.Command, args []string) error {
	cred, err := azurepkg.LoadCredential()
	if err != nil {
		return fmt.Errorf("Azure credential: %w", err)
	}

	subID := resolveSubscriptionID()
	locations := []string{}
	if flagLocation != "" {
		locations = []string{flagLocation}
	}

	app := tui.NewMSRApp(cred, subID, locations)
	p := tea.NewProgram(app, tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("TUI: %w", err)
	}
	return nil
}

// ── scan subcommand ───────────────────────────────────────────────────────────

func buildScanCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "scan",
		Short: "Scan Azure resources and print results",
		RunE:  runScan,
	}
}

func runScan(cmd *cobra.Command, args []string) error {
	cred, locations, err := loadCredAndLocations()
	if err != nil {
		return err
	}

	subID := resolveSubscriptionID()
	fmt.Fprintf(os.Stderr, "Scanning %d location(s)…\n", len(locations))

	resources, err := collector.RunAll(
		context.Background(),
		cred,
		subID,
		locations,
		collector.DefaultRegistry,
		10,
		func(location, rt string) {
			fmt.Fprintf(os.Stderr, "  ✓ %s / %s\n", location, rt)
		},
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: partial errors during scan: %v\n", err)
	}

	return printResources(resources)
}

// ── orphans subcommand ────────────────────────────────────────────────────────

func buildOrphansCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "orphans",
		Short: "List orphaned (unused) Azure resources",
		RunE:  runOrphans,
	}
}

func runOrphans(cmd *cobra.Command, args []string) error {
	cred, locations, err := loadCredAndLocations()
	if err != nil {
		return err
	}

	subID := resolveSubscriptionID()
	fmt.Fprintf(os.Stderr, "Scanning %d location(s)…\n", len(locations))

	resources, err := collector.RunAll(
		context.Background(),
		cred,
		subID,
		locations,
		collector.DefaultRegistry,
		10, nil,
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
	}

	g := graph.BuildGraph(resources)
	orphan.RunAll(g)

	orphans := g.Orphans()
	if len(orphans) == 0 {
		fmt.Println("No orphaned resources found.")
		return nil
	}

	fmt.Printf("Found %d orphaned resource(s):\n\n", len(orphans))
	return printNodes(orphans)
}

// ── version subcommand ────────────────────────────────────────────────────────

func buildVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("msr %s (commit %s, built %s)\n",
				version.Version, version.Commit, version.Date)
		},
	}
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func resolveSubscriptionID() string {
	if flagSubscription != "" {
		return flagSubscription
	}
	if id := os.Getenv("AZURE_SUBSCRIPTION_ID"); id != "" {
		return id
	}
	return ""
}

func loadCredAndLocations() (*azurepkg.Cred, []string, error) {
	cred, err := azurepkg.LoadCredential()
	if err != nil {
		return nil, nil, fmt.Errorf("Azure credential: %w", err)
	}

	if flagLocation != "" {
		return cred, []string{flagLocation}, nil
	}

	subID := resolveSubscriptionID()
	if subID == "" {
		// Fall back to common Azure locations if no subscription for dynamic lookup.
		return cred, defaultLocations(), nil
	}

	locations, err := azurepkg.ListLocations(context.Background(), cred, subID)
	if err != nil {
		return nil, nil, fmt.Errorf("list locations: %w", err)
	}
	return cred, locations, nil
}

// defaultLocations returns a sensible default set of Azure locations for scanning.
func defaultLocations() []string {
	return []string{
		"eastus", "eastus2", "westus", "westus2", "westus3",
		"centralus", "northcentralus", "southcentralus", "westcentralus",
		"canadacentral", "canadaeast",
		"northeurope", "westeurope", "uksouth", "ukwest",
		"francecentral", "germanywestcentral", "swedencentral", "norwayeast",
		"southeastasia", "eastasia", "japaneast", "japanwest",
		"australiaeast", "australiasoutheast",
		"brazilsouth",
		"centralindia", "southindia",
		"koreacentral",
	}
}

func printResources(resources []collector.Resource) error {
	if flagOutput == "json" {
		return printJSON(resources)
	}
	for _, r := range resources {
		fmt.Printf("%-20s  %-20s  %-15s  %s\n",
			r.Type, r.Location, r.RawID, r.DisplayName())
	}
	return nil
}

func printNodes(nodes []*graph.Node) error {
	if flagOutput == "json" {
		resources := make([]collector.Resource, len(nodes))
		for i, n := range nodes {
			resources[i] = n.Resource
		}
		return printJSON(resources)
	}
	for _, n := range nodes {
		r := n.Resource
		reasons := ""
		if len(n.OrphanReasons) > 0 {
			reasons = " — " + n.OrphanReasons[0]
		}
		fmt.Printf("%-20s  %-20s  %-15s  %s%s\n",
			r.Type, r.Location, r.RawID, r.DisplayName(), reasons)
	}
	return nil
}

func printJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
