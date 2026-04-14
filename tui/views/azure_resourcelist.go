package views

// azure_resourcelist.go renders the main Azure resource browser for MSR.

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/angsak/mbr/internal/azure/collector"
	"github.com/angsak/mbr/internal/azure/graph"
)

// ── Messages ──────────────────────────────────────────────────────────────────

type AzureShowOrphansMsg struct{}
type AzureShowGraphMsg struct{ NodeID string }
type AzureShowDetailMsg struct{ NodeID string }

// ── Group definitions ─────────────────────────────────────────────────────────

type azureCategory struct {
	name  string
	icon  string
	color lipgloss.Color
	types []collector.ResourceType
}

var azureCategories = []azureCategory{
	{
		name:  "Networking",
		icon:  "⬡",
		color: lipgloss.Color("#06B6D4"),
		types: []collector.ResourceType{
			collector.TypeVNet, collector.TypeSubnet,
			collector.TypeNSG, collector.TypePublicIP,
			collector.TypeLoadBalancer,
		},
	},
	{
		name:  "Compute",
		icon:  "⬡",
		color: lipgloss.Color("#3B82F6"),
		types: []collector.ResourceType{
			collector.TypeVM, collector.TypeVMSS,
		},
	},
	{
		name:  "Database",
		icon:  "⬡",
		color: lipgloss.Color("#F97316"),
		types: []collector.ResourceType{
			collector.TypeSQLServer, collector.TypeSQLDatabase,
			collector.TypeCosmosAccount, collector.TypeRedisCache,
		},
	},
	{
		name:  "Serverless",
		icon:  "⬡",
		color: lipgloss.Color("#A78BFA"),
		types: []collector.ResourceType{
			collector.TypeFunctionApp, collector.TypeAppService,
		},
	},
	{
		name:  "Storage",
		icon:  "⬡",
		color: lipgloss.Color("#10B981"),
		types: []collector.ResourceType{
			collector.TypeDisk, collector.TypeStorageAccount,
		},
	},
}

var azureTypeLabel = map[collector.ResourceType]string{
	collector.TypeVM:             "Virtual Machine",
	collector.TypeDisk:           "Managed Disk",
	collector.TypeVMSS:           "VM Scale Set",
	collector.TypeVNet:           "Virtual Network",
	collector.TypeSubnet:         "Subnet",
	collector.TypeNSG:            "Network Security Group",
	collector.TypePublicIP:       "Public IP Address",
	collector.TypeLoadBalancer:   "Load Balancer",
	collector.TypeSQLServer:      "SQL Server",
	collector.TypeSQLDatabase:    "SQL Database",
	collector.TypeCosmosAccount:  "Cosmos DB Account",
	collector.TypeRedisCache:     "Redis Cache",
	collector.TypeStorageAccount: "Storage Account",
	collector.TypeFunctionApp:    "Function App",
	collector.TypeAppService:     "App Service",
}

// ── Model ─────────────────────────────────────────────────────────────────────

type azureRenderedLine struct {
	text   string
	nodeID string
}

// AzureResourceListModel is the BubbleTea model for the Azure resource browser.
type AzureResourceListModel struct {
	g      *graph.ResourceGraph
	width  int
	height int
	lines  []azureRenderedLine
	cursor int
	offset int
}

// NewAzureResourceListModel creates a model sized to the given content area.
func NewAzureResourceListModel(g *graph.ResourceGraph, width, height int) AzureResourceListModel {
	m := AzureResourceListModel{
		g:      g,
		width:  width,
		height: height,
	}
	m.lines = m.buildLines()
	m.cursor = m.firstSelectable()
	return m
}

func (m AzureResourceListModel) Init() tea.Cmd { return nil }

func (m AzureResourceListModel) Update(msg tea.Msg) (AzureResourceListModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.lines = m.buildLines()
		m.cursor = m.firstSelectable()

	case tea.KeyMsg:
		if len(m.lines) == 0 {
			m.lines = m.buildLines()
			m.cursor = m.firstSelectable()
		}
		switch msg.String() {
		case "up", "k":
			m.moveCursor(-1)
		case "down", "j":
			m.moveCursor(1)
		case "pgup", "ctrl+u":
			m.moveCursor(-(m.listRows() / 2))
		case "pgdown", "ctrl+d":
			m.moveCursor(m.listRows() / 2)
		case "enter":
			id := ""
			if m.cursor >= 0 && m.cursor < len(m.lines) {
				id = m.lines[m.cursor].nodeID
			}
			return m, func() tea.Msg { return AzureShowDetailMsg{NodeID: id} }
		case "o":
			return m, func() tea.Msg { return AzureShowOrphansMsg{} }
		case "g":
			id := ""
			if m.cursor >= 0 && m.cursor < len(m.lines) {
				id = m.lines[m.cursor].nodeID
			}
			return m, func() tea.Msg { return AzureShowGraphMsg{NodeID: id} }
		}
	}
	return m, nil
}

func (m *AzureResourceListModel) moveCursor(delta int) {
	if len(m.lines) == 0 {
		return
	}
	sel := m.selectableIndices()
	if len(sel) == 0 {
		return
	}

	pos := 0
	for i, idx := range sel {
		if idx == m.cursor {
			pos = i
			break
		}
	}

	pos += delta
	if pos < 0 {
		pos = 0
	}
	if pos >= len(sel) {
		pos = len(sel) - 1
	}
	m.cursor = sel[pos]
	m.scrollToCursor()
}

func (m *AzureResourceListModel) scrollToCursor() {
	h := m.listRows()
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+h {
		m.offset = m.cursor - h + 1
	}
}

func (m *AzureResourceListModel) selectableIndices() []int {
	var out []int
	for i, l := range m.lines {
		if l.nodeID != "" {
			out = append(out, i)
		}
	}
	return out
}

func (m AzureResourceListModel) firstSelectable() int {
	for i, l := range m.lines {
		if l.nodeID != "" {
			return i
		}
	}
	return 0
}

func (m AzureResourceListModel) View() string {
	if m.width == 0 || m.g == nil {
		return ""
	}

	lines := m.lines
	if len(lines) == 0 {
		lines = m.buildLines()
	}

	h := m.listRows()
	offset := m.offset
	if offset > len(lines)-h {
		offset = len(lines) - h
	}
	if offset < 0 {
		offset = 0
	}

	end := offset + h
	if end > len(lines) {
		end = len(lines)
	}

	var b strings.Builder
	for i := offset; i < end; i++ {
		line := lines[i].text
		if i == m.cursor && lines[i].nodeID != "" {
			rest := ""
			if len(line) > 2 {
				rest = line[2:]
			}
			arrow := lipgloss.NewStyle().
				Foreground(lipgloss.Color("#06B6D4")).Bold(true).
				Render("▶")
			line = lipgloss.NewStyle().
				Background(lipgloss.Color("#374151")).
				Render(arrow + " " + rest)
		}
		b.WriteString(line + "\n")
	}

	for i := end - offset; i < h; i++ {
		b.WriteString(strings.Repeat(" ", m.width) + "\n")
	}

	b.WriteString(m.renderFooter())
	return b.String()
}

type azureTypeGroup struct {
	rt    collector.ResourceType
	nodes []*graph.Node
}

func (m AzureResourceListModel) buildLines() []azureRenderedLine {
	var lines []azureRenderedLine

	for _, cat := range azureCategories {
		var groups []azureTypeGroup
		for _, rt := range cat.types {
			nodes := m.g.FilterByType(rt)
			if len(nodes) > 0 {
				groups = append(groups, azureTypeGroup{rt: rt, nodes: nodes})
			}
		}
		if len(groups) == 0 {
			continue
		}

		totalCount := 0
		for _, grp := range groups {
			totalCount += len(grp.nodes)
		}

		if len(lines) > 0 {
			lines = append(lines, azureRenderedLine{text: ""})
		}

		lines = append(lines, m.renderCategoryHeader(cat, totalCount))

		rule := lipgloss.NewStyle().Foreground(cat.color).
			Render(strings.Repeat("─", m.width))
		lines = append(lines, azureRenderedLine{text: rule})

		useSubHeaders := len(groups) > 1
		for i, grp := range groups {
			if useSubHeaders {
				if i > 0 {
					lines = append(lines, azureRenderedLine{text: ""})
				}
				lines = append(lines, m.renderSubGroupHeader(grp.rt, len(grp.nodes), cat.color))
			}
			for _, node := range grp.nodes {
				lines = append(lines, azureRenderedLine{
					text:   m.renderResourceRow(node),
					nodeID: node.Resource.ID,
				})
			}
		}
	}

	orphans := m.g.Orphans()
	if len(orphans) > 0 {
		lines = append(lines, azureRenderedLine{text: ""})
		lines = append(lines, azureRenderedLine{
			text: lipgloss.NewStyle().
				Foreground(lipgloss.Color("#F59E0B")).Bold(true).
				Render(fmt.Sprintf("  ⚠  %d orphaned resource(s) detected — press o to review", len(orphans))),
		})
	}

	return lines
}

func (m AzureResourceListModel) renderCategoryHeader(cat azureCategory, count int) azureRenderedLine {
	iconStyle := lipgloss.NewStyle().Foreground(cat.color)
	nameStyle := lipgloss.NewStyle().Foreground(cat.color).Bold(true)
	countStyle := lipgloss.NewStyle().Foreground(cat.color)

	left := "  " + iconStyle.Render(cat.icon) + "  " + nameStyle.Render(strings.ToUpper(cat.name))
	right := countStyle.Render(fmt.Sprintf("%d resource(s)  ", count))

	gap := m.width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}
	return azureRenderedLine{text: left + strings.Repeat(" ", gap) + right}
}

func (m AzureResourceListModel) renderSubGroupHeader(rt collector.ResourceType, count int, catColor lipgloss.Color) azureRenderedLine {
	label := azureTypeLabel[rt]
	if label == "" {
		label = string(rt)
	}
	labelStyle := lipgloss.NewStyle().Foreground(catColor).Bold(true)
	countStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#6B7280"))
	text := "  " + labelStyle.Render(label) +
		"  " + countStyle.Render(fmt.Sprintf("(%d)", count))
	return azureRenderedLine{text: text}
}

func (m AzureResourceListModel) renderResourceRow(node *graph.Node) string {
	res := node.Resource

	indicator := "  "
	if node.IsOrphan {
		indicator = lipgloss.NewStyle().Foreground(lipgloss.Color("#F59E0B")).Bold(true).Render("⚠ ")
	}

	rawID := res.RawID
	if rawID == "" {
		rawID = res.ID
	}
	rawID = truncate(rawID, 22)
	idStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#9CA3AF"))
	idCol := idStyle.Render(fmt.Sprintf("%-22s", rawID))

	name := res.DisplayName()
	if name == res.RawID {
		name = ""
	}
	name = truncate(name, 26)
	nameStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#F3F4F6"))
	nameCol := nameStyle.Render(fmt.Sprintf("%-26s", name))

	meta := azureResourceMeta(res)
	meta = truncate(meta, 24)
	metaStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#6B7280"))
	metaCol := metaStyle.Render(fmt.Sprintf("%-24s", meta))

	locationStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#4B5563"))
	locationCol := locationStyle.Render(res.Location)

	row := "  " + indicator + idCol + "  " + nameCol + "  " + metaCol + "  " + locationCol

	w := lipgloss.Width(row)
	if w < m.width {
		row += strings.Repeat(" ", m.width-w)
	}
	return row
}

func azureResourceMeta(res collector.Resource) string {
	switch res.Type {
	case collector.TypeVM:
		size := res.Metadata["VMSize"]
		state := res.Metadata["PowerState"]
		if size != "" && state != "" {
			return size + "  " + state
		}
		return size + state
	case collector.TypeDisk:
		size := res.Metadata["DiskSizeGB"]
		state := res.Metadata["State"]
		if size != "" {
			return size + " GB  " + state
		}
		return state
	case collector.TypeVNet:
		return res.Metadata["AddressSpace"]
	case collector.TypeSubnet:
		return res.Metadata["AddressPrefix"]
	case collector.TypeNSG:
		return res.Metadata["SecurityRuleCount"] + " rules"
	case collector.TypePublicIP:
		ip := res.Metadata["IPAddress"]
		if ip != "" {
			return ip
		}
		return res.Metadata["AllocationMethod"]
	case collector.TypeLoadBalancer:
		return res.Metadata["SKU"] + "  " + res.Metadata["BackendPoolCount"] + " pools"
	case collector.TypeSQLServer:
		return res.Metadata["State"]
	case collector.TypeSQLDatabase:
		return res.Metadata["Edition"] + "  " + res.Metadata["Status"]
	case collector.TypeCosmosAccount:
		return res.Metadata["Kind"]
	case collector.TypeRedisCache:
		return res.Metadata["SKU"] + "  C" + res.Metadata["Capacity"]
	case collector.TypeStorageAccount:
		return res.Metadata["Kind"] + "  " + res.Metadata["SKU"]
	case collector.TypeFunctionApp, collector.TypeAppService:
		return res.Metadata["State"]
	case collector.TypeVMSS:
		return res.Metadata["SKU"] + "  " + res.Metadata["Capacity"] + " instances"
	}
	return ""
}

func (m AzureResourceListModel) renderFooter() string {
	key := func(k, desc string) string {
		kStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#F3F4F6")).
			Background(lipgloss.Color("#374151")).
			PaddingLeft(1).PaddingRight(1).Bold(true)
		dStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#6B7280"))
		return kStyle.Render(k) + " " + dStyle.Render(desc)
	}

	hints := []string{
		key("↑↓", "navigate"),
		key("enter", "detail"),
		key("o", "orphans"),
		key("g", "graph"),
		key("q", "quit"),
	}

	total := m.g.Len()
	right := lipgloss.NewStyle().Foreground(lipgloss.Color("#4B5563")).
		Render(fmt.Sprintf("%d resources  ", total))

	left := "  " + strings.Join(hints, "   ")
	gap := m.width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}
	return left + strings.Repeat(" ", gap) + right
}

func (m AzureResourceListModel) listRows() int {
	h := m.height - 1
	if h < 5 {
		return 5
	}
	return h
}
