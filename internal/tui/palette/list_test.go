package palette

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

type testItem struct {
	title string
	desc  string
}

func (t testItem) Title() string       { return t.title }
func (t testItem) Description() string { return t.desc }
func (t testItem) FilterValue() string { return t.title }

type testCategorizedItem struct {
	title    string
	desc     string
	category string
}

func (t testCategorizedItem) Title() string       { return t.title }
func (t testCategorizedItem) Description() string { return t.desc }
func (t testCategorizedItem) FilterValue() string { return t.title + " " + t.category }
func (t testCategorizedItem) Category() string    { return t.category }

func keyUp() tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: tea.KeyUp}
}

func keyDown() tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: tea.KeyDown}
}

func keyEnter() tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: tea.KeyEnter}
}

func keyEsc() tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: tea.KeyEsc}
}

func typeChars(l *PaletteList, s string) {
	for _, ch := range s {
		l.Update(tea.KeyPressMsg{Code: ch, Text: string(ch)})
	}
}

func TestPaletteList_Init(t *testing.T) {
	var l PaletteList
	items := []PaletteItem{
		testItem{title: "/help", desc: "Show help"},
		testItem{title: "/quit", desc: "Exit"},
	}
	avail := []bool{true, true}

	l.Init(items, avail)

	if l.done {
		t.Fatal("should not be done after init")
	}
	if l.cancelled {
		t.Fatal("should not be cancelled after init")
	}
	if len(l.filtered) != 2 {
		t.Fatalf("expected 2 filtered items, got %d", len(l.filtered))
	}
	if !l.search.Focused() {
		t.Fatal("search should be focused after init")
	}
}

func TestPaletteList_Render(t *testing.T) {
	var l PaletteList
	items := []PaletteItem{
		testItem{title: "/help", desc: "Show help"},
		testItem{title: "/quit", desc: "Exit"},
	}
	avail := []bool{true, true}
	l.Init(items, avail)
	l.SetSize(80, 40)

	view := l.View()
	if view == "" {
		t.Fatal("expected non-empty view")
	}
	if !strings.Contains(stripANSI(view), "/help") {
		t.Fatal("view should contain /help")
	}
	if !strings.Contains(stripANSI(view), "/quit") {
		t.Fatal("view should contain /quit")
	}
}

func TestPaletteList_Navigation(t *testing.T) {
	var l PaletteList
	items := []PaletteItem{
		testItem{title: "/a", desc: "A"},
		testItem{title: "/b", desc: "B"},
		testItem{title: "/c", desc: "C"},
	}
	avail := []bool{true, true, true}
	l.Init(items, avail)

	if l.filtered[l.selected].Title() != "/a" {
		t.Fatalf("expected /a, got %s", l.filtered[l.selected].Title())
	}

	l.Down()
	if l.filtered[l.selected].Title() != "/b" {
		t.Fatalf("after Down: expected /b, got %s", l.filtered[l.selected].Title())
	}

	l.Down()
	if l.filtered[l.selected].Title() != "/c" {
		t.Fatalf("after 2nd Down: expected /c, got %s", l.filtered[l.selected].Title())
	}

	l.Down()
	if l.filtered[l.selected].Title() != "/a" {
		t.Fatalf("after wrap Down: expected /a, got %s", l.filtered[l.selected].Title())
	}

	l.Up()
	if l.filtered[l.selected].Title() != "/c" {
		t.Fatalf("after Up: expected /c, got %s", l.filtered[l.selected].Title())
	}
}

func TestPaletteList_Select(t *testing.T) {
	var l PaletteList
	items := []PaletteItem{
		testItem{title: "/help", desc: "Show help"},
		testItem{title: "/quit", desc: "Exit"},
	}
	avail := []bool{true, true}
	l.Init(items, avail)

	l.Select()
	if !l.done {
		t.Fatal("should be done after select")
	}
	result, idx := l.Result()
	if result == nil {
		t.Fatal("result should not be nil")
	}
	if result.Title() != "/help" {
		t.Fatalf("expected /help, got %s", result.Title())
	}
	if idx != 0 {
		t.Fatalf("expected index 0, got %d", idx)
	}
}

func TestPaletteList_Cancel(t *testing.T) {
	var l PaletteList
	items := []PaletteItem{
		testItem{title: "/help", desc: "Show help"},
	}
	avail := []bool{true}
	l.Init(items, avail)

	l.Cancel()
	if !l.cancelled {
		t.Fatal("should be cancelled after cancel")
	}
}

func TestPaletteList_SearchFiltering(t *testing.T) {
	var l PaletteList
	items := []PaletteItem{
		testItem{title: "/help", desc: "Show help"},
		testItem{title: "/quit", desc: "Exit"},
		testItem{title: "/model", desc: "Change model"},
		testItem{title: "/connect", desc: "Connect"},
		testItem{title: "/disconnect", desc: "Disconnect"},
	}
	avail := []bool{true, true, true, true, true}
	l.Init(items, avail)

	if len(l.filtered) != 5 {
		t.Fatalf("empty query should show all 5 items, got %d", len(l.filtered))
	}

	typeChars(&l, "mod")
	if len(l.filtered) != 1 {
		t.Fatalf("search 'mod' should match 1 item, got %d", len(l.filtered))
	}
	if l.filtered[0].Title() != "/model" {
		t.Fatalf("search 'mod' should match '/model', got %q", l.filtered[0].Title())
	}

	l2 := PaletteList{}
	l2.Init(items, avail)
	typeChars(&l2, "con")
	if len(l2.filtered) != 2 {
		t.Fatalf("search 'con' should match 2 items, got %d", len(l2.filtered))
	}
	names := []string{l2.filtered[0].Title(), l2.filtered[1].Title()}
	foundConnect := false
	foundDisconnect := false
	for _, n := range names {
		if n == "/connect" {
			foundConnect = true
		}
		if n == "/disconnect" {
			foundDisconnect = true
		}
	}
	if !foundConnect || !foundDisconnect {
		t.Fatalf("search 'con' should match /connect and /disconnect, got %v", names)
	}
}

func TestPaletteList_NavigationWithFilter(t *testing.T) {
	var l PaletteList
	items := []PaletteItem{
		testItem{title: "/help", desc: "Show help"},
		testItem{title: "/quit", desc: "Exit"},
		testItem{title: "/model", desc: "Change model"},
		testItem{title: "/connect", desc: "Connect"},
		testItem{title: "/disconnect", desc: "Disconnect"},
	}
	avail := []bool{true, true, true, true, true}
	l.Init(items, avail)

	typeChars(&l, "con")

	if len(l.filtered) != 2 {
		t.Fatalf("expected 2 filtered items, got %d", len(l.filtered))
	}

	l.Down()
	first := l.filtered[l.selected].Title()
	l.Up()
	second := l.filtered[l.selected].Title()
	if first == second {
		t.Fatal("Up/Down should change selection in filtered list")
	}
}

func TestPaletteList_Scrolling(t *testing.T) {
	var l PaletteList
	var items []PaletteItem
	for i := 0; i < 20; i++ {
		items = append(items, testItem{title: fmt.Sprintf("/cmd%d", i), desc: fmt.Sprintf("Command %d", i)})
	}
	avail := make([]bool, 20)
	for i := range avail {
		avail[i] = true
	}
	l.Init(items, avail)
	l.SetSize(100, 50)

	typeChars(&l, "cmd")

	if len(l.filtered) != 20 {
		t.Fatalf("expected 20 filtered items, got %d", len(l.filtered))
	}

	for i := 0; i < MaxVisible+2; i++ {
		l.Down()
	}

	if l.selected < MaxVisible {
		t.Fatalf("expected selection beyond visible range, got %d", l.selected)
	}
}

func TestPaletteList_DisabledItems(t *testing.T) {
	var l PaletteList
	items := []PaletteItem{
		testItem{title: "/a", desc: "A"},
		testItem{title: "/b", desc: "B"},
		testItem{title: "/c", desc: "C"},
	}
	avail := []bool{true, false, true}
	l.Init(items, avail)

	if len(l.filtered) != 3 {
		t.Fatalf("expected 3 items, got %d", len(l.filtered))
	}

	if l.selected != 0 {
		t.Fatalf("expected selection 0, got %d", l.selected)
	}

	l.Down()
	if l.filtered[l.selected].Title() != "/c" {
		t.Fatalf("Down should skip disabled item, got %s", l.filtered[l.selected].Title())
	}

	l.Down()
	if l.filtered[l.selected].Title() != "/a" {
		t.Fatalf("Down should wrap to /a, got %s", l.filtered[l.selected].Title())
	}
}

func TestPaletteList_EmptyItems(t *testing.T) {
	var l PaletteList
	l.Init(nil, nil)

	if len(l.filtered) != 0 {
		t.Fatalf("expected 0 filtered items, got %d", len(l.filtered))
	}

	l.Select()
	if l.done {
		t.Fatal("selecting from empty list should not set done")
	}
}

func TestPaletteList_GroupedMode(t *testing.T) {
	var l PaletteList
	items := []PaletteItem{
		testCategorizedItem{title: "gpt-4o", desc: "128K ctx", category: "OpenAI"},
		testCategorizedItem{title: "gpt-4o-mini", desc: "128K ctx", category: "OpenAI"},
		testCategorizedItem{title: "claude-sonnet-4", desc: "200K ctx", category: "Anthropic"},
		testCategorizedItem{title: "gemini-2.5-pro", desc: "128K ctx", category: "Google"},
	}
	avail := []bool{true, true, true, true}

	l.InitGrouped(items, avail)

	if !l.grouped {
		t.Fatal("grouped mode should be true")
	}

	if len(l.filtered) != 7 {
		t.Fatalf("expected 7 items (3 headers + 4 models), got %d", len(l.filtered))
	}

	if l.filtered[0] != nil {
		t.Fatal("first item should be header (nil)")
	}

	l.Select()
	if !l.done {
		t.Fatal("should be done after select")
	}
	result, idx := l.Result()
	if result == nil {
		t.Fatal("result should not be nil")
	}
	if result.Title() != "claude-sonnet-4" {
		t.Fatalf("expected claude-sonnet-4 (first selectable after Anthropic header), got %s", result.Title())
	}
	if idx != 1 {
		t.Fatalf("expected index 1, got %d", idx)
	}
}

func TestPaletteList_GroupedNavigation(t *testing.T) {
	var l PaletteList
	items := []PaletteItem{
		testCategorizedItem{title: "gpt-4o", desc: "", category: "OpenAI"},
		testCategorizedItem{title: "claude-sonnet-4", desc: "", category: "Anthropic"},
	}
	avail := []bool{true, true}

	l.InitGrouped(items, avail)

	if l.filtered[0] != nil {
		t.Fatalf("first item should be header (nil), got %v", l.filtered[0])
	}

	if l.filtered[l.selected].Title() != "claude-sonnet-4" {
		t.Fatalf("first selectable should be claude-sonnet-4, got %s", l.filtered[l.selected].Title())
	}

	l.Down()
	if l.filtered[l.selected].Title() != "gpt-4o" {
		t.Fatalf("after Down: expected gpt-4o, got %s", l.filtered[l.selected].Title())
	}

	l.Down()
	if l.filtered[l.selected].Title() != "claude-sonnet-4" {
		t.Fatalf("after 2nd Down (wrap): expected claude-sonnet-4, got %s", l.filtered[l.selected].Title())
	}
}

func TestPaletteList_GroupedSearchFlattens(t *testing.T) {
	var l PaletteList
	items := []PaletteItem{
		testCategorizedItem{title: "gpt-4o", desc: "", category: "OpenAI"},
		testCategorizedItem{title: "claude-sonnet-4", desc: "", category: "Anthropic"},
		testCategorizedItem{title: "gemini-2.5-pro", desc: "", category: "Google"},
	}
	avail := []bool{true, true, true}

	l.InitGrouped(items, avail)

	if len(l.filtered) != 6 {
		t.Fatalf("expected 6 items (3 headers + 3 models), got %d", len(l.filtered))
	}

	typeChars(&l, "gpt")

	if len(l.filtered) != 1 {
		t.Fatalf("search 'gpt' should match 1 item, got %d", len(l.filtered))
	}

	if l.filtered[0].Title() != "gpt-4o" {
		t.Fatalf("search 'gpt' should match gpt-4o, got %s", l.filtered[0].Title())
	}
}

func TestPaletteList_MouseWheelScrolling(t *testing.T) {
	var l PaletteList
	items := []PaletteItem{
		testItem{title: "/a", desc: "A"},
		testItem{title: "/b", desc: "B"},
		testItem{title: "/c", desc: "C"},
	}
	avail := []bool{true, true, true}
	l.Init(items, avail)

	if l.filtered[l.selected].Title() != "/a" {
		t.Fatalf("expected /a, got %s", l.filtered[l.selected].Title())
	}

	l.Update(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	if l.filtered[l.selected].Title() != "/b" {
		t.Fatalf("after wheel down: expected /b, got %s", l.filtered[l.selected].Title())
	}

	l.Update(tea.MouseWheelMsg{Button: tea.MouseWheelUp})
	if l.filtered[l.selected].Title() != "/a" {
		t.Fatalf("after wheel up: expected /a, got %s", l.filtered[l.selected].Title())
	}

	l.Update(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	l.Update(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	if l.filtered[l.selected].Title() != "/c" {
		t.Fatalf("after wheel down 2: expected /c, got %s", l.filtered[l.selected].Title())
	}
}

func TestPaletteList_GroupedIndentation(t *testing.T) {
	var l PaletteList
	items := []PaletteItem{
		testCategorizedItem{title: "gpt-4o", desc: "128K ctx", category: "OpenAI"},
		testCategorizedItem{title: "claude-sonnet-4", desc: "200K ctx", category: "Anthropic"},
		testCategorizedItem{title: "gpt-4o-mini", desc: "128K ctx", category: "OpenAI"},
	}
	avail := []bool{true, true, true}

	l.InitGrouped(items, avail)
	l.SetSize(100, 50)

	l.Down()

	view := l.View()
	stripped := stripANSI(view)

	lines := strings.Split(stripped, "\n")
	var headerLine, modelLine string
	for _, line := range lines {
		if (strings.Contains(line, "OpenAI") || strings.Contains(line, "Anthropic")) &&
			!strings.Contains(line, "gpt-4o") && !strings.Contains(line, "claude-sonnet-4") {
			headerLine = line
		}
		if strings.Contains(line, "gpt-4o-mini") {
			modelLine = line
		}
	}

	if headerLine == "" {
		t.Fatalf("expected a header line in view, got lines:\n%s", strings.Join(lines, "\n"))
	}
	if modelLine == "" {
		t.Fatalf("expected a model line in view, got lines:\n%s", strings.Join(lines, "\n"))
	}

	headerContent := strings.TrimPrefix(strings.TrimSuffix(headerLine, "│"), "│")
	modelContent := strings.TrimPrefix(strings.TrimSuffix(modelLine, "│"), "│")

	headerIndent := len(headerContent) - len(strings.TrimLeft(headerContent, " "))
	modelIndent := len(modelContent) - len(strings.TrimLeft(modelContent, " "))

	if modelIndent <= headerIndent {
		t.Fatalf("model line indent (%d) should be greater than header indent (%d)", modelIndent, headerIndent)
	}
	if modelIndent-headerIndent != 4 {
		t.Fatalf("model should have 4 extra spaces of indentation (2 from indentation + 2 from box padding), got %d extra", modelIndent-headerIndent)
	}
}

func stripANSI(s string) string {
	var result strings.Builder
	inEscape := false
	for _, ch := range s {
		if ch == '\x1b' {
			inEscape = true
			continue
		}
		if inEscape {
			if ch == 'm' {
				inEscape = false
			}
			continue
		}
		result.WriteRune(ch)
	}
	return result.String()
}
