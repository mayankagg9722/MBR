package views

// azure_detailview.go shows full resource metadata and 30-day cost for an
// Azure resource selected in the MSR TUI.

import (
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/angsak/mbr/internal/azure/cost"
	"github.com/angsak/mbr/internal/azure/graph"
)

// AzureCostResultMsg carries the async cost fetch result.
type AzureCostResultMsg struct {
	NodeID string
	Result cost.Result
}

// AzureDetailViewModel is the BubbleTea model for the Azure resource detail screen.
type AzureDetailViewModel struct {
	node        *graph.Node
	costLoading bool
	costResult  *cost.Result
	lines       []string
	offset      int
	width       int
	height      int
}

func NewAzureDetailViewModel(node *graph.Node, width, height int) AzureDetailViewModel {
	m := AzureDetailViewModel{
		node:        node,
		costLoading: true,
		width:       width,
		height:      height,
	}
	m.lines = m.buildLines()
	return m
}

func (m AzureDetailViewModel) Init() tea.Cmd { return nil }

func (m AzureDetailViewModel) Update(msg tea.Msg) (AzureDetailViewModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.lines = m.buildLines()

	case AzureCostResultMsg:
		if m.node != nil && msg.NodeID == m.node.Resource.ID {
			r := msg.Result
			m.costResult = &r
			m.costLoading = false
			m.node.CostUSD = r.USD
			m.lines = m.buildLines()
		}

	case tea.KeyMsg:
		h := m.listRows()
		switch msg.String() {
		case "up", "k":
			if m.offset > 0 {
				m.offset--
			}
		case "down", "j":
			if m.offset+h < len(m.lines) {
				m.offset++
			}
		case "pgup", "ctrl+u":
			m.offset -= h / 2
			if m.offset < 0 {
				m.offset = 0
			}
		case "pgdown", "ctrl+d":
			m.offset += h / 2
			max := len(m.lines) - h
			if m.offset > max {
				m.offset = max
			}
			if m.offset < 0 {
				m.offset = 0
			}
		}
	}
	return m, nil
}

func (m AzureDetailViewModel) View() string {
	if m.width == 0 || m.node == nil {
		return ""
	}

	var b strings.Builder

	res := m.node.Resource
	rawID := res.RawID
	if rawID == "" {
		rawID = res.ID
	}
	title := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#06B6D4")).Bold(true).
		Render(fmt.Sprintf("  ◈  %s", truncate(rawID, 36)))
	typeStr := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#6B7280")).
		Render(fmt.Sprintf("(%s)  ", string(res.Type)))

	gap := m.width - lipgloss.Width(title) - lipgloss.Width(typeStr)
	if gap < 1 {
		gap = 1
	}
	b.WriteString(title + strings.Repeat(" ", gap) + typeStr + "\n")

	divider := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#374151")).
		Render(strings.Repeat("─", m.width))
	b.WriteString(divider + "\n")

	h := m.listRows()
	offset := m.offset
	if offset > len(m.lines)-h {
		offset = len(m.lines) - h
	}
	if offset < 0 {
		offset = 0
	}
	end := offset + h
	if end > len(m.lines) {
		end = len(m.lines)
	}
	for i := offset; i < end; i++ {
		b.WriteString(m.lines[i] + "\n")
	}
	for i := end - offset; i < h; i++ {
		b.WriteString(strings.Repeat(" ", m.width) + "\n")
	}

	b.WriteString(divider + "\n")
	b.WriteString(m.renderFooter())
	return b.String()
}

func (m AzureDetailViewModel) buildLines() []string {
	if m.node == nil {
		return nil
	}
	res := m.node.Resource

	dim := lipgloss.NewStyle().Foreground(lipgloss.Color("#6B7280"))
	bright := lipgloss.NewStyle().Foreground(lipgloss.Color("#F3F4F6"))
	section := lipgloss.NewStyle().Foreground(lipgloss.Color("#9CA3AF")).Bold(true)
	amber := lipgloss.NewStyle().Foreground(lipgloss.Color("#F59E0B"))
	green := lipgloss.NewStyle().Foreground(lipgloss.Color("#10B981")).Bold(true)
	blue := lipgloss.NewStyle().Foreground(lipgloss.Color("#3B82F6"))

	kw := 20

	kv := func(key, val string) string {
		return "  " + dim.Render(fmt.Sprintf("%-*s", kw, key)) + "  " + bright.Render(val)
	}

	var lines []string
	add := func(s string) { lines = append(lines, s) }

	add("")
	add(kv("Resource ID", truncate(res.ID, m.width-kw-6)))
	add(kv("Name", res.Name))
	add(kv("Type", string(res.Type)))
	add(kv("Location", res.Location))
	add(kv("Resource Group", res.ResourceGroup))
	if res.SubscriptionID != "" {
		add(kv("Subscription", res.SubscriptionID))
	}
	if m.node.IsOrphan {
		orphanStr := amber.Render("⚠  orphan")
		if len(m.node.OrphanReasons) > 0 {
			orphanStr += dim.Render("  — " + m.node.OrphanReasons[0])
		}
		add("  " + orphanStr)
	}

	// Metadata
	if len(res.Metadata) > 0 {
		add("")
		add("  " + section.Render("METADATA"))

		keys := make([]string, 0, len(res.Metadata))
		for k := range res.Metadata {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			add(kv(k, res.Metadata[k]))
		}
	}

	// Tags
	if len(res.Tags) > 0 {
		add("")
		add("  " + section.Render("TAGS"))

		keys := make([]string, 0, len(res.Tags))
		for k := range res.Tags {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			add(kv(k, res.Tags[k]))
		}
	}

	// Cost
	add("")
	add("  " + section.Render("COST") + "  " + dim.Render("(30-day, USD)"))

	switch {
	case m.costLoading:
		add("  " + dim.Render("⟳  Loading…"))
	case m.costResult == nil || m.costResult.Granularity == "none":
		add("  " + dim.Render("—  not available"))
	case m.costResult.Err != nil:
		add("  " + amber.Render("⚠  "+m.costResult.Err.Error()))
	case m.costResult.Granularity == "resource":
		add("  " + green.Render(fmt.Sprintf("$%.4f", m.costResult.USD)) +
			"  " + dim.Render("(per-resource)"))
	case m.costResult.Granularity == "service":
		svc := ""
		if sn, ok := cost.ServiceNameFor(res.Type); ok {
			svc = sn
		}
		add("  " + blue.Render(fmt.Sprintf("$%.2f", m.costResult.USD)) +
			"  " + dim.Render(fmt.Sprintf("(service total: %s)", svc)))
	}

	add("")
	return lines
}

func (m AzureDetailViewModel) renderFooter() string {
	key := func(k, desc string) string {
		kStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#F3F4F6")).
			Background(lipgloss.Color("#374151")).
			PaddingLeft(1).PaddingRight(1).Bold(true)
		dStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#6B7280"))
		return kStyle.Render(k) + " " + dStyle.Render(desc)
	}
	hints := []string{
		key("↑↓", "scroll"),
		key("esc", "back"),
	}
	return "  " + strings.Join(hints, "   ")
}

func (m AzureDetailViewModel) listRows() int {
	h := m.height - 4
	if h < 3 {
		return 3
	}
	return h
}
