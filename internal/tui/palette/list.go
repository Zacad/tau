package palette

import (
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/bubbles/v2/textinput"

	"github.com/adam/tau/internal/fuzzy"
)

const MaxVisible = 11

type PaletteItem interface {
	Title() string
	Description() string
	FilterValue() string
}

type PaletteList struct {
	items       []PaletteItem
	avail       []bool
	selected    int
	search      textinput.Model
	filtered    []PaletteItem
	filteredAvail []bool
	positions   [][]int
	done        bool
	cancelled   bool
	result      PaletteItem
	resultIndex int
	width       int
	height      int
}

func (l *PaletteList) Init(items []PaletteItem, avail []bool) {
	l.items = items
	l.avail = avail
	l.selected = 0
	l.done = false
	l.cancelled = false
	l.result = nil
	l.resultIndex = -1

	if l.search.Placeholder == "" {
		l.search = textinput.New()
		l.search.Placeholder = "Search..."
		l.search.CharLimit = 64
	}
	l.search.SetValue("")
	l.search.Focus()

	l.filterItems("")
}

func (l *PaletteList) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "up":
			l.Up()
			return nil
		case "down", "tab":
			l.Down()
			return nil
		case "enter":
			l.Select()
			return nil
		case "esc":
			l.Cancel()
			return nil
		}
	}

	var cmd tea.Cmd
	prevQuery := l.search.Value()
	l.search, cmd = l.search.Update(msg)
	if l.search.Value() != prevQuery {
		l.filterItems(l.search.Value())
	}
	return cmd
}

func (l *PaletteList) View() string {
	boxWidth := min(l.width-8, 90)
	if boxWidth < 20 {
		boxWidth = 20
	}

	l.search.SetWidth(boxWidth - 4)

	var lines []string
	lines = append(lines, SearchStyle.Render(l.search.View()))
	lines = append(lines, "")

	visible := l.getVisibleCommands()
	for _, item := range visible {
		isSelected := item.idx == l.selected
		positions := l.positions[item.idx]
		line := renderItem(item.item, isSelected, item.avail, positions)
		lines = append(lines, line)
	}

	content := strings.Join(lines, "\n")
	return BoxStyle.Width(boxWidth).Render(content)
}

func (l *PaletteList) Done() bool { return l.done }

func (l *PaletteList) Cancelled() bool { return l.cancelled }

func (l *PaletteList) Result() (PaletteItem, int) {
	return l.result, l.resultIndex
}

func (l *PaletteList) SetSize(width, height int) {
	l.width = width
	l.height = height
}

func (l *PaletteList) Refilter() {
	l.filterItems(l.search.Value())
}

func (l *PaletteList) Up() {
	if len(l.filtered) == 0 {
		return
	}
	for i := 1; i <= len(l.filtered); i++ {
		idx := (l.selected - i + len(l.filtered)) % len(l.filtered)
		if l.filteredAvail[idx] {
			l.selected = idx
			return
		}
	}
}

func (l *PaletteList) Down() {
	if len(l.filtered) == 0 {
		return
	}
	for i := 1; i <= len(l.filtered); i++ {
		idx := (l.selected + i) % len(l.filtered)
		if l.filteredAvail[idx] {
			l.selected = idx
			return
		}
	}
}

func (l *PaletteList) Select() {
	if len(l.filtered) == 0 || !l.isSelectable(l.selected) {
		return
	}
	l.result = l.filtered[l.selected]
	l.resultIndex = l.selected
	l.done = true
}

func (l *PaletteList) Cancel() {
	l.cancelled = true
}

func (l *PaletteList) SearchValue() string {
	return l.search.Value()
}

func (l *PaletteList) SelectedItem() PaletteItem {
	if len(l.filtered) == 0 || !l.isSelectable(l.selected) {
		return nil
	}
	return l.filtered[l.selected]
}

func (l *PaletteList) filterItems(query string) {
	if query == "" {
		l.filtered = make([]PaletteItem, len(l.items))
		copy(l.filtered, l.items)
		l.filteredAvail = make([]bool, len(l.avail))
		copy(l.filteredAvail, l.avail)
		l.positions = make([][]int, len(l.items))
		for i := range l.items {
			l.positions[i] = nil
		}
	} else {
		type scored struct {
			item      PaletteItem
			avail     bool
			score     int
			positions []int
		}

		var scoredItems []scored
		for i := range l.items {
			it := l.items[i]
			if score, matched, positions := fuzzy.Match(query, it.FilterValue()); matched {
				scoredItems = append(scoredItems, scored{
					item: it, avail: l.avail[i], score: score, positions: positions,
				})
			}
		}

		sort.Slice(scoredItems, func(i, j int) bool {
			return scoredItems[i].score > scoredItems[j].score
		})

		l.filtered = make([]PaletteItem, 0, len(scoredItems))
		l.filteredAvail = make([]bool, 0, len(scoredItems))
		l.positions = make([][]int, 0, len(scoredItems))
		for _, sc := range scoredItems {
			l.filtered = append(l.filtered, sc.item)
			l.filteredAvail = append(l.filteredAvail, sc.avail)
			l.positions = append(l.positions, sc.positions)
		}
	}

	l.ensureSelectableSelection()
}

func (l *PaletteList) ensureSelectableSelection() {
	if len(l.filtered) == 0 {
		l.selected = 0
		return
	}
	if !l.isSelectable(l.selected) {
		l.selectNextAvailable()
	}
}

func (l *PaletteList) isSelectable(idx int) bool {
	if idx < 0 || idx >= len(l.filteredAvail) {
		return false
	}
	return l.filteredAvail[idx]
}

func (l *PaletteList) selectNextAvailable() {
	if len(l.filtered) == 0 {
		return
	}
	for i := 0; i < len(l.filtered); i++ {
		idx := (l.selected + i) % len(l.filtered)
		if l.filteredAvail[idx] {
			l.selected = idx
			return
		}
	}
}

type visibleItem struct {
	item PaletteItem
	avail bool
	idx   int
}

func (l *PaletteList) getVisibleCommands() []visibleItem {
	if len(l.filtered) == 0 {
		return nil
	}

	visibleCount := min(len(l.filtered), MaxVisible)

	start := 0
	if l.selected >= MaxVisible {
		start = l.selected - MaxVisible + 1
	}
	end := start + visibleCount
	if end > len(l.filtered) {
		end = len(l.filtered)
		start = end - visibleCount
		if start < 0 {
			start = 0
		}
	}

	result := make([]visibleItem, 0, end-start)
	for i := start; i < end; i++ {
		result = append(result, visibleItem{item: l.filtered[i], avail: l.filteredAvail[i], idx: i})
	}
	return result
}

func renderItem(item PaletteItem, selected bool, avail bool, positions []int) string {
	prefix := "  "
	if selected {
		prefix = CursorStyle.Render("> ")
	}

	titleStr := item.Title()
	isDisabled := !avail

	posSet := make(map[int]bool)
	for _, p := range positions {
		posSet[p+1] = true
	}

	var titleBuilder strings.Builder
	baseStyle := NameStyle
	if selected {
		baseStyle = SelectedNameStyle
	}
	if isDisabled {
		baseStyle = DisabledNameStyle
	}

	for i, ch := range titleStr {
		chStr := string(ch)
		if posSet[i] {
			titleBuilder.WriteString(baseStyle.Bold(true).Render(chStr))
		} else {
			titleBuilder.WriteString(baseStyle.Render(chStr))
		}
	}
	title := titleBuilder.String()

	if isDisabled {
		title += " " + DisabledTagStyle.Render("[unavailable]")
	}

	descStyle := DescStyle
	if isDisabled {
		descStyle = DisabledDescStyle
	}
	desc := descStyle.Render(item.Description())
	return prefix + title + "  " + desc
}
