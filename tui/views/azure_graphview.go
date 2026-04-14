package views

// azure_graphview.go renders the dependency graph for a single Azure resource.

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/angsak/mbr/internal/azure/graph"
)

type azureGraphLine struct{ text string }

// AzureGraphViewModel is the BubbleTea model for the Azure graph screen.
type AzureGraphViewModel struct {
	g      *graph.ResourceGraph
	node   *graph.Node
	lines  []azureGraphLine
	offset int
	width  int
	height int
}

func NewAzureGraphViewModel(g *graph.ResourceGraph, nodeID string, width, height int) AzureGraphViewModel {
	m := AzureGraphViewModel{g: g, width: width, height: height}
	if nodeID != "" {
		if n, ok := g.Node(nodeID); ok {
			m.node = n
		}
	}
	m.lines = m.buildLines()
	return m
}

func (m AzureGraphViewModel) Init() tea.Cmd { return nil }

func (m AzureGraphViewModel) Update(msg tea.Msg) (AzureGraphViewModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.lines = m.buildLines()

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

func (m AzureGraphViewModel) View() string {
	if m.width == 0 {
		return ""
	}

	var b strings.Builder

	var titleText string
	if m.node != nil {
		rawID := m.node.Resource.RawID
		if rawID == "" {
			rawID = m.node.Resource.ID
		}
		titleText = fmt.Sprintf("  ⬡  Dependency Graph — %s", truncate(rawID, 30))
	} else {
		titleText = "  ⬡  Dependency Graph"
	}

	title := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#06B6D4")).Bold(true).
		Render(titleText)

	typeStr := ""
	if m.node != nil {
		typeStr = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#6B7280")).
			Render(fmt.Sprintf("(%s)  ", string(m.node.Resource.Type)))
	}

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
		b.WriteString(m.lines[i].text + "\n")
	}
	for i := end - offset; i < h; i++ {
		b.WriteString(strings.Repeat(" ", m.width) + "\n")
	}

	b.WriteString(divider + "\n")
	b.WriteString(m.renderFooter())

	return b.String()
}

func (m AzureGraphViewModel) buildLines() []azureGraphLine {
	var lines []azureGraphLine

	dim := lipgloss.NewStyle().Foreground(lipgloss.Color("#6B7280"))
	accent := lipgloss.NewStyle().Foreground(lipgloss.Color("#06B6D4"))
	bright := lipgloss.NewStyle().Foreground(lipgloss.Color("#F3F4F6")).Bold(true)
	amber := lipgloss.NewStyle().Foreground(lipgloss.Color("#F59E0B"))
	purple := lipgloss.NewStyle().Foreground(lipgloss.Color("#A78BFA")).Bold(true)
	green := lipgloss.NewStyle().Foreground(lipgloss.Color("#10B981")).Bold(true)

	if m.node == nil {
		lines = append(lines, azureGraphLine{text: ""})
		lines = append(lines, azureGraphLine{
			text: "  " + dim.Render("Navigate to a resource on the list and press") +
				" " + bright.Render("g") +
				" " + dim.Render("to view its dependencies."),
		})
		return lines
	}

	res := m.node.Resource
	rawID := res.RawID
	if rawID == "" {
		rawID = res.ID
	}

	lines = append(lines, azureGraphLine{text: ""})
	orphanTag := ""
	if m.node.IsOrphan {
		orphanTag = "  " + amber.Render("⚠ orphan")
	}
	lines = append(lines, azureGraphLine{
		text: "  " + bright.Render(truncate(rawID, 26)) +
			"  " + dim.Render(string(res.Type)) +
			"  " + dim.Render(res.Location) +
			orphanTag,
	})

	displayName := res.DisplayName()
	if displayName != rawID && displayName != "" {
		lines = append(lines, azureGraphLine{
			text: "  " + dim.Render("name: "+displayName),
		})
	}

	outEdges := m.g.EdgesFrom(res.ID)
	if len(outEdges) > 0 {
		lines = append(lines, azureGraphLine{text: ""})
		lines = append(lines, azureGraphLine{
			text: "  " + purple.Render(fmt.Sprintf("DEPENDS ON  (%d)", len(outEdges))),
		})
		for _, e := range outEdges {
			if target, ok := m.g.Node(e.ToID); ok {
				lines = append(lines, azureGraphLine{
					text: m.renderEdgeLine("→", string(e.Relationship), target, accent, dim),
				})
			}
		}
	}

	dependents := m.g.Dependents(res.ID)
	if len(dependents) > 0 {
		lines = append(lines, azureGraphLine{text: ""})
		lines = append(lines, azureGraphLine{
			text: "  " + green.Render(fmt.Sprintf("REFERENCED BY  (%d)", len(dependents))),
		})
		for _, dep := range dependents {
			rel := ""
			for _, e := range m.g.EdgesFrom(dep.Resource.ID) {
				if e.ToID == res.ID {
					rel = string(e.Relationship)
					break
				}
			}
			lines = append(lines, azureGraphLine{
				text: m.renderEdgeLine("←", rel, dep, green, dim),
			})
		}
	}

	if len(outEdges) == 0 && len(dependents) == 0 {
		lines = append(lines, azureGraphLine{text: ""})
		lines = append(lines, azureGraphLine{
			text: "  " + dim.Render("No connections — this resource has no graph edges."),
		})
	}

	return lines
}

func (m AzureGraphViewModel) renderEdgeLine(dir, rel string, node *graph.Node, relStyle, dimStyle lipgloss.Style) string {
	rawID := node.Resource.RawID
	if rawID == "" {
		rawID = node.Resource.ID
	}
	rawID = truncate(rawID, 22)
	idCol := dimStyle.Render(fmt.Sprintf("%-22s", rawID))

	name := node.Resource.DisplayName()
	if name == node.Resource.RawID {
		name = ""
	}
	name = truncate(name, 26)
	nameStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#F3F4F6"))
	nameCol := nameStyle.Render(fmt.Sprintf("%-26s", name))

	return "    " + dir + "  " +
		relStyle.Render(fmt.Sprintf("%-14s", rel)) +
		"  " + idCol +
		"  " + nameCol
}

func (m AzureGraphViewModel) renderFooter() string {
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

func (m AzureGraphViewModel) listRows() int {
	h := m.height - 4
	if h < 3 {
		return 3
	}
	return h
}
