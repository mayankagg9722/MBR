package tui

// msrapp.go is the root BubbleTea model for msr (Make Satya Rich).
// It mirrors app.go but uses Azure-specific packages and views.

import (
	"context"
	"fmt"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/angsak/mbr/internal/azure/collector"
	"github.com/angsak/mbr/internal/azure/cost"
	"github.com/angsak/mbr/internal/azure/graph"
	"github.com/angsak/mbr/internal/azure/orphan"
	"github.com/angsak/mbr/tui/components"
	"github.com/angsak/mbr/tui/views"
)

// AzureScreen identifies which view is currently active.
type AzureScreen int

const (
	AzureScreenLocationSelect AzureScreen = iota
	AzureScreenLoading
	AzureScreenResourceList
	AzureScreenOrphans
	AzureScreenGraph
	AzureScreenDetail
)

// MSRAppModel is the root BubbleTea model for msr.
type MSRAppModel struct {
	screen AzureScreen
	width  int
	height int

	cred           *azidentity.DefaultAzureCredential
	subscriptionID string
	locations      []string

	resourceGraph *graph.ResourceGraph
	allResources  []collector.Resource

	locationView views.AzureLocationModel
	resourceList views.AzureResourceListModel
	orphanList   views.AzureOrphanListModel
	graphView    views.AzureGraphViewModel
	detailView   views.AzureDetailViewModel
	spinner      components.SpinnerModel
	statusBar    components.StatusBarModel

	loadingMsg string
	scanErr    error
}

// NewMSRApp creates the root MSR AppModel.
func NewMSRApp(cred *azidentity.DefaultAzureCredential, subscriptionID string, locations []string) MSRAppModel {
	m := MSRAppModel{
		cred:           cred,
		subscriptionID: subscriptionID,
		locations:      locations,
		spinner:        components.NewSpinner(),
		statusBar:      components.NewStatusBar(),
	}
	m.statusBar.Profile = subscriptionID
	if len(locations) == 1 {
		m.statusBar.Region = locations[0]
	} else if len(locations) > 1 {
		m.statusBar.Region = fmt.Sprintf("%d locations", len(locations))
	}

	if len(locations) > 0 {
		m.screen = AzureScreenLoading
	} else {
		m.screen = AzureScreenLocationSelect
		m.locationView = views.NewAzureLocationModel()
	}
	return m
}

func (m MSRAppModel) Init() tea.Cmd {
	switch m.screen {
	case AzureScreenLocationSelect:
		return m.spinner.Init()
	case AzureScreenLoading:
		return tea.Batch(m.spinner.Init(), azureStartScanCmd(m.cred, m.subscriptionID, m.locations))
	}
	return nil
}

func (m MSRAppModel) contentHeight() int {
	h := m.height - headerHeight - statusBarHeight
	if h < 1 {
		return 1
	}
	return h
}

func (m MSRAppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		var sbCmd tea.Cmd
		m.statusBar, sbCmd = m.statusBar.Update(msg)

		contentMsg := tea.WindowSizeMsg{
			Width:  msg.Width,
			Height: m.contentHeight(),
		}
		var lvCmd tea.Cmd
		m.locationView, lvCmd = m.locationView.Update(contentMsg)
		if m.resourceGraph != nil {
			m.resourceList, _ = m.resourceList.Update(contentMsg)
			if m.screen == AzureScreenOrphans {
				m.orphanList, _ = m.orphanList.Update(contentMsg)
			}
			if m.screen == AzureScreenGraph {
				m.graphView, _ = m.graphView.Update(contentMsg)
			}
			if m.screen == AzureScreenDetail {
				m.detailView, _ = m.detailView.Update(contentMsg)
			}
		}
		return m, tea.Batch(sbCmd, lvCmd)

	case tea.KeyMsg:
		switch {
		case key.Matches(msg, GlobalKeys.Quit):
			return m, tea.Quit
		case key.Matches(msg, GlobalKeys.Back):
			switch m.screen {
			case AzureScreenOrphans, AzureScreenGraph, AzureScreenDetail:
				m.screen = AzureScreenResourceList
				return m, nil
			case AzureScreenResourceList:
				m.screen = AzureScreenLocationSelect
				m.locationView = views.NewAzureLocationModel()
				m.locationView, _ = m.locationView.Update(tea.WindowSizeMsg{
					Width: m.width, Height: m.contentHeight(),
				})
				return m, nil
			}
		}

	case views.AzureLocationSelectedMsg:
		m.locations = msg.Locations
		m.statusBar.Region = fmt.Sprintf("%d location(s)", len(msg.Locations))
		m.screen = AzureScreenLoading
		m.loadingMsg = "Connecting to Azure…"
		return m, tea.Batch(m.spinner.Init(), azureStartScanCmd(m.cred, m.subscriptionID, m.locations))

	case components.ProgressMsg:
		m.loadingMsg = fmt.Sprintf("Scanning %s / %s  (%d/%d)",
			msg.Region, msg.ResourceType, msg.Done, msg.Total)
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case azureScanResultMsg:
		if msg.err != nil {
			m.scanErr = msg.err
		}
		m.allResources = msg.resources
		g := graph.BuildGraph(m.allResources)
		orphan.RunAll(g)
		m.resourceGraph = g
		m.statusBar.ResourceCount = g.Len()
		m.screen = AzureScreenResourceList
		m.resourceList = views.NewAzureResourceListModel(g, m.width, m.contentHeight())
		return m, nil

	case views.AzureShowOrphansMsg:
		m.screen = AzureScreenOrphans
		m.orphanList = views.NewAzureOrphanListModel(m.resourceGraph, m.width, m.contentHeight())
		return m, nil

	case views.AzureShowGraphMsg:
		m.screen = AzureScreenGraph
		m.graphView = views.NewAzureGraphViewModel(m.resourceGraph, msg.NodeID, m.width, m.contentHeight())
		return m, nil

	case views.AzureShowDetailMsg:
		if node, ok := m.resourceGraph.Node(msg.NodeID); ok {
			m.screen = AzureScreenDetail
			m.detailView = views.NewAzureDetailViewModel(node, m.width, m.contentHeight())
			return m, azureFetchCostCmd(m.cred, node)
		}
		return m, nil

	case views.AzureCostResultMsg:
		m.detailView, _ = m.detailView.Update(msg)
		return m, nil

	default:
		if m.screen == AzureScreenLoading || m.screen == AzureScreenLocationSelect {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
	}

	// Delegate to active sub-model.
	switch m.screen {
	case AzureScreenLocationSelect:
		updated, cmd := m.locationView.Update(msg)
		m.locationView = updated
		return m, cmd
	case AzureScreenResourceList:
		updated, cmd := m.resourceList.Update(msg)
		m.resourceList = updated
		return m, cmd
	case AzureScreenOrphans:
		updated, cmd := m.orphanList.Update(msg)
		m.orphanList = updated
		return m, cmd
	case AzureScreenGraph:
		updated, cmd := m.graphView.Update(msg)
		m.graphView = updated
		return m, cmd
	case AzureScreenDetail:
		updated, cmd := m.detailView.Update(msg)
		m.detailView = updated
		return m, cmd
	}

	return m, nil
}

func (m MSRAppModel) View() string {
	if m.width == 0 {
		return ""
	}

	header := m.renderHeader()
	status := m.statusBar.View()
	body := m.renderBody()

	return header + body + status
}

func (m MSRAppModel) renderHeader() string {
	logo := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#06B6D4")).
		Bold(true).
		Render("  msr")

	tagline := Styles.Dim.Render("  Azure Resource Browser")

	right := ""
	if m.subscriptionID != "" {
		right = Styles.Dim.Render(m.subscriptionID + "  ")
	}

	leftPart := logo + tagline
	rightPart := right
	gap := m.width - lipgloss.Width(leftPart) - lipgloss.Width(rightPart)
	if gap < 0 {
		gap = 0
	}
	inner := leftPart + strings.Repeat(" ", gap) + rightPart

	headerStyle := lipgloss.NewStyle().
		Background(lipgloss.Color("#1F2937")).
		Width(m.width)

	divider := lipgloss.NewStyle().
		Foreground(Palette.Border).
		Width(m.width).
		Render(strings.Repeat("─", m.width))

	return headerStyle.Render(inner) + "\n" + divider + "\n"
}

func (m MSRAppModel) renderBody() string {
	w := m.width
	h := m.contentHeight()

	var content string

	switch m.screen {
	case AzureScreenLocationSelect:
		content = m.locationView.View()

	case AzureScreenLoading:
		inner := Styles.Title.Render("msr — Azure Resource Browser") + "\n\n" +
			m.spinner.View() + "  " + Styles.Dim.Render(m.loadingMsg)
		content = lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, inner)

	case AzureScreenResourceList:
		content = m.resourceList.View()

	case AzureScreenOrphans:
		content = m.orphanList.View()

	case AzureScreenGraph:
		content = m.graphView.View()

	case AzureScreenDetail:
		content = m.detailView.View()
	}

	if m.scanErr != nil {
		content += "\n" + Styles.Orphan.Render("⚠  "+m.scanErr.Error())
	}

	return lipgloss.NewStyle().Height(h).Render(content)
}

// ── Commands ─────────────────────────────────────────────────────────────────

type azureScanResultMsg struct {
	resources []collector.Resource
	err       error
}

func azureStartScanCmd(cred *azidentity.DefaultAzureCredential, subscriptionID string, locations []string) tea.Cmd {
	return func() tea.Msg {
		resources, err := collector.RunAll(
			context.Background(),
			cred,
			subscriptionID,
			locations,
			collector.DefaultRegistry,
			10,
			nil,
		)
		return azureScanResultMsg{resources: resources, err: err}
	}
}

func azureFetchCostCmd(cred *azidentity.DefaultAzureCredential, node *graph.Node) tea.Cmd {
	return func() tea.Msg {
		result := cost.FetchResource(context.Background(), cred, node.Resource)
		return views.AzureCostResultMsg{NodeID: node.Resource.ID, Result: result}
	}
}
