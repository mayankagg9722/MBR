package views

// azure_orphanlist.go renders the orphaned-resource review screen for Azure.

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/angsak/mbr/internal/azure/graph"
)

// AzureOrphanListModel is the BubbleTea model for the Azure orphan review screen.
type AzureOrphanListModel struct {
	nodes  []*graph.Node
	cursor int
	offset int
	width  int
	height int
}

func NewAzureOrphanListModel(g *graph.ResourceGraph, width, height int) AzureOrphanListModel {
	return AzureOrphanListModel{
		nodes:  g.Orphans(),
		width:  width,
		height: height,
	}
}

func (m AzureOrphanListModel) Init() tea.Cmd { return nil }

func (m AzureOrphanListModel) Update(msg tea.Msg) (AzureOrphanListModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case tea.KeyMsg:
		h := m.listRows()
		switch msg.String() {
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
				if m.cursor < m.offset {
					m.offset = m.cursor
				}
			}
		case "down", "j":
			if m.cursor < len(m.nodes)-1 {
				m.cursor++
				if m.cursor >= m.offset+h {
					m.offset = m.cursor - h + 1
				}
			}
		}
	}
	return m, nil
}

func (m AzureOrphanListModel) View() string {
	if m.width == 0 {
		return ""
	}

	var b strings.Builder

	title := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#F59E0B")).Bold(true).
		Render("  ⚠  Orphaned Resources")
	countStr := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#6B7280")).
		Render(fmt.Sprintf("%d found  ", len(m.nodes)))
	gap := m.width - lipgloss.Width(title) - lipgloss.Width(countStr)
	if gap < 1 {
		gap = 1
	}
	b.WriteString(title + strings.Repeat(" ", gap) + countStr + "\n")

	divider := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#374151")).
		Render(strings.Repeat("─", m.width))
	b.WriteString(divider + "\n")

	h := m.listRows()

	if len(m.nodes) == 0 {
		empty := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#6B7280")).
			Render("  No orphaned resources detected.")
		b.WriteString(empty + "\n")
		for i := 1; i < h; i++ {
			b.WriteString(strings.Repeat(" ", m.width) + "\n")
		}
	} else {
		offset := m.offset
		if offset > len(m.nodes)-h {
			offset = len(m.nodes) - h
		}
		if offset < 0 {
			offset = 0
		}
		end := offset + h
		if end > len(m.nodes) {
			end = len(m.nodes)
		}
		for i := offset; i < end; i++ {
			b.WriteString(m.renderRow(i) + "\n")
		}
		for i := end - offset; i < h; i++ {
			b.WriteString(strings.Repeat(" ", m.width) + "\n")
		}
	}

	b.WriteString(divider + "\n")
	b.WriteString(m.renderFooter())

	return b.String()
}

func (m AzureOrphanListModel) renderRow(i int) string {
	node := m.nodes[i]
	isCursor := i == m.cursor

	cursor := "  "
	if isCursor {
		cursor = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#F59E0B")).Bold(true).
			Render("▶ ")
	}

	rawID := node.Resource.RawID
	if rawID == "" {
		rawID = node.Resource.ID
	}
	rawID = truncate(rawID, 22)
	idCol := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#9CA3AF")).
		Render(fmt.Sprintf("%-22s", rawID))

	name := node.Resource.DisplayName()
	if name == node.Resource.RawID {
		name = ""
	}
	name = truncate(name, 26)
	nameCol := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#F3F4F6")).
		Render(fmt.Sprintf("%-26s", name))

	reason := ""
	if len(node.OrphanReasons) > 0 {
		reason = node.OrphanReasons[0]
	}
	reason = truncate(reason, m.width)
	reasonCol := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#F59E0B")).
		Render(reason)

	row := cursor + idCol + "  " + nameCol + "  " + reasonCol

	w := lipgloss.Width(row)
	if w < m.width {
		row += strings.Repeat(" ", m.width-w)
	}

	if isCursor {
		return lipgloss.NewStyle().Background(lipgloss.Color("#374151")).Render(row)
	}
	return row
}

func (m AzureOrphanListModel) renderFooter() string {
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
		key("esc", "back"),
	}
	total := lipgloss.NewStyle().Foreground(lipgloss.Color("#4B5563")).
		Render(fmt.Sprintf("%d orphan(s)  ", len(m.nodes)))

	left := "  " + strings.Join(hints, "   ")
	gap := m.width - lipgloss.Width(left) - lipgloss.Width(total)
	if gap < 1 {
		gap = 1
	}
	return left + strings.Repeat(" ", gap) + total
}

func (m AzureOrphanListModel) listRows() int {
	h := m.height - 4
	if h < 3 {
		return 3
	}
	return h
}
