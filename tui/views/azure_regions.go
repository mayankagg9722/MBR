package views

// azure_regions.go is the Azure location picker for MSR.

import (
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// AllAzureLocations is the complete list of generally-available Azure locations.
var AllAzureLocations = func() []string {
	locations := []string{
		"australiacentral",
		"australiaeast",
		"australiasoutheast",
		"brazilsouth",
		"canadacentral",
		"canadaeast",
		"centralindia",
		"centralus",
		"eastasia",
		"eastus",
		"eastus2",
		"francecentral",
		"germanywestcentral",
		"israelcentral",
		"italynorth",
		"japaneast",
		"japanwest",
		"koreacentral",
		"koreasouth",
		"mexicocentral",
		"newzealandnorth",
		"northcentralus",
		"northeurope",
		"norwayeast",
		"polandcentral",
		"qatarcentral",
		"southafricanorth",
		"southcentralus",
		"southeastasia",
		"southindia",
		"spaincentral",
		"swedencentral",
		"switzerlandnorth",
		"uaenorth",
		"uksouth",
		"ukwest",
		"westcentralus",
		"westeurope",
		"westindia",
		"westus",
		"westus2",
		"westus3",
	}
	sort.Strings(locations)
	return locations
}()

// AzureLocationSelectedMsg is sent to the root AppModel when the user confirms
// their location selection.
type AzureLocationSelectedMsg struct {
	Locations []string
}

// AzureLocationModel is a custom-rendered Azure location picker.
type AzureLocationModel struct {
	all        []string
	visible    []string
	selected   map[string]bool
	cursor     int
	offset     int
	filterMode bool
	filterText string
	allToggle  bool
	width      int
	height     int
}

// NewAzureLocationModel returns an AzureLocationModel pre-populated with Azure locations.
func NewAzureLocationModel() AzureLocationModel {
	return AzureLocationModel{
		all:      AllAzureLocations,
		visible:  AllAzureLocations,
		selected: make(map[string]bool),
	}
}

func (m AzureLocationModel) Init() tea.Cmd { return nil }

func (m AzureLocationModel) Update(msg tea.Msg) (AzureLocationModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case tea.KeyMsg:
		if m.filterMode {
			return m.updateFilterMode(msg)
		}
		return m.updateNormalMode(msg)
	}
	return m, nil
}

func (m AzureLocationModel) updateNormalMode(msg tea.KeyMsg) (AzureLocationModel, tea.Cmd) {
	listH := m.listRows()

	switch msg.String() {
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
			if m.cursor < m.offset {
				m.offset = m.cursor
			}
		}
	case "down", "j":
		if m.cursor < len(m.visible)-1 {
			m.cursor++
			if m.cursor >= m.offset+listH {
				m.offset = m.cursor - listH + 1
			}
		}
	case " ":
		if m.cursor < len(m.visible) {
			name := m.visible[m.cursor]
			m.selected[name] = !m.selected[name]
			if m.cursor < len(m.visible)-1 {
				m.cursor++
				if m.cursor >= m.offset+listH {
					m.offset = m.cursor - listH + 1
				}
			}
		}
	case "a":
		m.allToggle = !m.allToggle
		if m.allToggle {
			for _, r := range m.all {
				m.selected[r] = true
			}
		} else {
			m.selected = make(map[string]bool)
		}
	case "/":
		m.filterMode = true
		m.filterText = ""
	case "enter":
		chosen := m.chosenLocations()
		if len(chosen) > 0 {
			return m, func() tea.Msg { return AzureLocationSelectedMsg{Locations: chosen} }
		}
	}
	return m, nil
}

func (m AzureLocationModel) updateFilterMode(msg tea.KeyMsg) (AzureLocationModel, tea.Cmd) {
	switch msg.String() {
	case "enter":
		m.filterMode = false
	case "esc":
		m.filterMode = false
		m.filterText = ""
		m.applyFilter()
	case "backspace", "ctrl+h":
		if len(m.filterText) > 0 {
			runes := []rune(m.filterText)
			m.filterText = string(runes[:len(runes)-1])
		}
		m.applyFilter()
	default:
		if len(msg.String()) == 1 {
			m.filterText += msg.String()
			m.applyFilter()
		}
	}
	return m, nil
}

func (m *AzureLocationModel) applyFilter() {
	q := strings.ToLower(m.filterText)
	if q == "" {
		m.visible = m.all
	} else {
		filtered := make([]string, 0, len(m.all))
		for _, r := range m.all {
			if strings.Contains(r, q) {
				filtered = append(filtered, r)
			}
		}
		m.visible = filtered
	}
	m.cursor = 0
	m.offset = 0
}

func (m AzureLocationModel) View() string {
	if m.width == 0 {
		return ""
	}

	var b strings.Builder
	listH := m.listRows()

	// Title row
	title := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#06B6D4")).
		Bold(true).
		Render("  Select Azure locations to scan")

	selCount := m.chosenLocations()
	countBadge := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#10B981")).
		Bold(true).
		Render(fmt.Sprintf("%d selected", len(selCount)))

	totalBadge := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#6B7280")).
		Render(fmt.Sprintf("/ %d total", len(m.visible)))

	titleGap := m.width - lipgloss.Width(title) - lipgloss.Width(countBadge) - lipgloss.Width(totalBadge) - 4
	if titleGap < 1 {
		titleGap = 1
	}
	b.WriteString(title + strings.Repeat(" ", titleGap) + countBadge + "  " + totalBadge + "\n")

	// Filter bar
	b.WriteString(m.renderFilterBar() + "\n")

	divider := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#374151")).
		Render(strings.Repeat("─", m.width))
	b.WriteString(divider + "\n")

	// Location rows
	end := m.offset + listH
	if end > len(m.visible) {
		end = len(m.visible)
	}

	for i := m.offset; i < end; i++ {
		b.WriteString(m.renderRow(i) + "\n")
	}
	for i := end - m.offset; i < listH; i++ {
		b.WriteString(strings.Repeat(" ", m.width) + "\n")
	}

	b.WriteString(divider + "\n")
	b.WriteString(m.renderFooter())

	return b.String()
}

func (m AzureLocationModel) renderRow(i int) string {
	name := m.visible[i]
	isCursor := i == m.cursor
	isSelected := m.selected[name]

	cursor := "  "
	if isCursor {
		cursor = lipgloss.NewStyle().Foreground(lipgloss.Color("#06B6D4")).Bold(true).Render("▶ ")
	}

	checkbox := lipgloss.NewStyle().Foreground(lipgloss.Color("#4B5563")).Render("○ ")
	if isSelected {
		checkbox = lipgloss.NewStyle().Foreground(lipgloss.Color("#10B981")).Bold(true).Render("● ")
	}

	nameStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#D1D5DB"))
	if isCursor && isSelected {
		nameStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#10B981")).Bold(true)
	} else if isCursor {
		nameStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#F3F4F6")).Bold(true)
	} else if isSelected {
		nameStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#10B981"))
	}

	row := cursor + checkbox + nameStyle.Render(name)
	rowWidth := lipgloss.Width(row)
	if rowWidth < m.width {
		row += strings.Repeat(" ", m.width-rowWidth)
	}

	if isCursor {
		return lipgloss.NewStyle().Background(lipgloss.Color("#1F2937")).Render(row)
	}
	return row
}

func (m AzureLocationModel) renderFilterBar() string {
	prefix := lipgloss.NewStyle().Foreground(lipgloss.Color("#6B7280")).Render("  / filter: ")

	if m.filterMode {
		text := lipgloss.NewStyle().Foreground(lipgloss.Color("#F3F4F6")).Render(m.filterText)
		cursor := lipgloss.NewStyle().Foreground(lipgloss.Color("#06B6D4")).Bold(true).Render("█")
		bar := prefix + text + cursor
		padW := m.width - lipgloss.Width(bar)
		if padW > 0 {
			bar += strings.Repeat(" ", padW)
		}
		return lipgloss.NewStyle().Background(lipgloss.Color("#111827")).Render(bar)
	}

	if m.filterText != "" {
		text := lipgloss.NewStyle().Foreground(lipgloss.Color("#06B6D4")).Render(m.filterText)
		hint := lipgloss.NewStyle().Foreground(lipgloss.Color("#4B5563")).Render("  (/ to edit, esc to clear)")
		return prefix + text + hint
	}

	hint := lipgloss.NewStyle().Foreground(lipgloss.Color("#4B5563")).Render("type / to filter locations")
	return prefix + hint
}

func (m AzureLocationModel) renderFooter() string {
	key := func(k, desc string) string {
		kStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#F3F4F6")).
			Background(lipgloss.Color("#374151")).
			PaddingLeft(1).PaddingRight(1).
			Bold(true)
		dStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#6B7280"))
		return kStyle.Render(k) + " " + dStyle.Render(desc)
	}

	hints := []string{
		key("↑↓", "move"),
		key("space", "toggle"),
		key("a", "all/none"),
		key("/", "filter"),
	}

	n := len(m.chosenLocations())
	var enterHint string
	if n > 0 {
		enterHint = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#F3F4F6")).
			Background(lipgloss.Color("#06B6D4")).
			Bold(true).
			PaddingLeft(2).PaddingRight(2).
			Render(fmt.Sprintf("enter  scan %d location(s) →", n))
	} else {
		enterHint = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#6B7280")).
			Render("select locations then press enter")
	}

	left := "  " + strings.Join(hints, "   ")
	gap := m.width - lipgloss.Width(left) - lipgloss.Width(enterHint) - 2
	if gap < 1 {
		gap = 1
	}
	return left + strings.Repeat(" ", gap) + enterHint
}

func (m AzureLocationModel) listRows() int {
	h := m.height - 5
	if h < 3 {
		return 3
	}
	return h
}

func (m AzureLocationModel) chosenLocations() []string {
	var out []string
	for r, sel := range m.selected {
		if sel {
			out = append(out, r)
		}
	}
	sort.Strings(out)
	return out
}
