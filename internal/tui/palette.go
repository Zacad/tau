package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/adam/tau/internal/tui/palette"
)

type paletteStep int

const (
	paletteStepList paletteStep = iota
	paletteStepInput
	paletteStepConfirm
	paletteStepTask
	paletteStepMessage
	paletteStepMultiStep
)

type CommandPalette struct {
	active           bool
	commands         []Command
	available        []bool
	list             palette.PaletteList
	input            palette.PaletteInput
	confirm          palette.PaletteConfirm
	task             palette.PaletteTask
	message          palette.PaletteMessage
	step             paletteStep
	width            int
	height           int
	selectionHandler func(item palette.PaletteItem, index int) tea.Cmd
	stepRunner       *palette.StepRunner
}

type commandItem struct {
	cmd   Command
	avail bool
}

func (c commandItem) Title() string {
	return "/" + c.cmd.Name()
}

func (c commandItem) Description() string {
	return c.cmd.Description()
}

func (c commandItem) FilterValue() string {
	return c.cmd.Name()
}

func (p *CommandPalette) Open(commands []Command) {
	p.active = true
	p.commands = commands
	p.available = make([]bool, len(commands))
	for i := range commands {
		p.available[i] = true
	}

	items := make([]palette.PaletteItem, len(commands))
	for i := range commands {
		items[i] = commandItem{cmd: commands[i], avail: true}
	}
	p.list.Init(items, p.available)
}

func (p *CommandPalette) SetAvailability(avail map[string]bool) {
	for i := range p.commands {
		if a, ok := avail[p.commands[i].Name()]; ok {
			p.available[i] = a
		}
	}

	items := make([]palette.PaletteItem, len(p.commands))
	for i := range p.commands {
		items[i] = commandItem{cmd: p.commands[i], avail: p.available[i]}
	}
	p.list.Init(items, p.available)
	p.list.Refilter()
}

func (p *CommandPalette) Close() {
	p.active = false
	p.commands = nil
	p.available = nil
	p.selectionHandler = nil
	p.step = paletteStepList
	p.stepRunner = nil
}

func (p *CommandPalette) IsActive() bool {
	return p.active
}

func (p *CommandPalette) Update(msg tea.Msg) tea.Cmd {
	switch p.step {
	case paletteStepList:
		return p.list.Update(msg)
	case paletteStepInput:
		return p.input.Update(msg)
	case paletteStepConfirm:
		return p.confirm.Update(msg)
	case paletteStepTask:
		return p.task.Update(msg)
	case paletteStepMessage:
		return p.message.Update(msg)
	case paletteStepMultiStep:
		if p.stepRunner == nil {
			return nil
		}
		step := p.stepRunner.CurrentStep()
		if step == nil {
			return nil
		}
		switch step.Kind() {
		case palette.StepKindList:
			return p.list.Update(msg)
		case palette.StepKindInput:
			return p.input.Update(msg)
		case palette.StepKindConfirm:
			return p.confirm.Update(msg)
		case palette.StepKindTask:
			return p.task.Update(msg)
		case palette.StepKindMessage:
			return p.message.Update(msg)
		}
	}
	return nil
}

func (p *CommandPalette) Up() {
	if !p.active || p.step != paletteStepList {
		return
	}
	p.list.Up()
}

func (p *CommandPalette) Down() {
	if !p.active || p.step != paletteStepList {
		return
	}
	p.list.Down()
}

func (p *CommandPalette) Selected() *Command {
	if !p.active {
		return nil
	}
	item := p.list.SelectedItem()
	if item == nil {
		return nil
	}
	if ci, ok := item.(commandItem); ok {
		return &ci.cmd
	}
	return nil
}

func (p *CommandPalette) SearchValue() string {
	return p.list.SearchValue()
}

func (p *CommandPalette) View(screenWidth, screenHeight int) string {
	if !p.active {
		return ""
	}

	p.width = screenWidth
	p.height = screenHeight

	var box string
	switch p.step {
	case paletteStepList:
		p.list.SetSize(screenWidth, screenHeight)
		box = p.list.View()
	case paletteStepInput:
		p.input.SetSize(screenWidth, screenHeight)
		box = p.input.View()
	case paletteStepConfirm:
		p.confirm.SetSize(screenWidth, screenHeight)
		box = p.confirm.View()
	case paletteStepTask:
		p.task.SetSize(screenWidth, screenHeight)
		box = p.task.View()
	case paletteStepMessage:
		p.message.SetSize(screenWidth, screenHeight)
		box = p.message.View()
	case paletteStepMultiStep:
		box = p.RenderBox(screenWidth, screenHeight)
	}

	topPad := max(0, (screenHeight-lipgloss.Height(box))/2)

	centered := lipgloss.PlaceHorizontal(screenWidth, lipgloss.Center, box)
	return paletteOverlayStyle.Width(screenWidth).Height(screenHeight).PaddingTop(topPad).Render(centered)
}

func (p *CommandPalette) RenderBox(screenWidth, screenHeight int) string {
	if !p.active {
		return ""
	}

	p.width = screenWidth
	p.height = screenHeight

	switch p.step {
	case paletteStepList:
		p.list.SetSize(screenWidth, screenHeight)
		return p.list.View()
	case paletteStepInput:
		p.input.SetSize(screenWidth, screenHeight)
		return p.input.View()
	case paletteStepConfirm:
		p.confirm.SetSize(screenWidth, screenHeight)
		return p.confirm.View()
	case paletteStepTask:
		p.task.SetSize(screenWidth, screenHeight)
		return p.task.View()
	case paletteStepMessage:
		p.message.SetSize(screenWidth, screenHeight)
		return p.message.View()
	case paletteStepMultiStep:
		if p.stepRunner != nil {
			return p.renderStep()
		}
	}
	return ""
}

func (p *CommandPalette) ListDone() bool {
	return p.step == paletteStepList && p.list.Done()
}

func (p *CommandPalette) ListCancelled() bool {
	return p.step == paletteStepList && p.list.Cancelled()
}

func (p *CommandPalette) ListResult() (palette.PaletteItem, int) {
	return p.list.Result()
}

func (p *CommandPalette) InputDone() bool {
	return p.step == paletteStepInput && p.input.Done()
}

func (p *CommandPalette) InputCancelled() bool {
	return p.step == paletteStepInput && p.input.Cancelled()
}

func (p *CommandPalette) InputResult() string {
	return p.input.Result()
}

func (p *CommandPalette) ShowInput(label, placeholder string) {
	p.step = paletteStepInput
	p.input.Init(label, placeholder)
}

func (p *CommandPalette) BackToList() {
	p.step = paletteStepList
}

func (p *CommandPalette) IsInputStep() bool {
	return p.step == paletteStepInput
}

func (p *CommandPalette) ConfirmDone() bool {
	return p.step == paletteStepConfirm && p.confirm.Done()
}

func (p *CommandPalette) ConfirmCancelled() bool {
	return p.step == paletteStepConfirm && p.confirm.Cancelled()
}

func (p *CommandPalette) ConfirmResult() bool {
	return p.confirm.Result()
}

func (p *CommandPalette) ShowConfirm(prompt string) {
	p.step = paletteStepConfirm
	p.confirm.Init(prompt)
}

func (p *CommandPalette) IsConfirmStep() bool {
	return p.step == paletteStepConfirm
}

func (p *CommandPalette) TaskDone() bool {
	return p.step == paletteStepTask && p.task.Done()
}

func (p *CommandPalette) TaskCancelled() bool {
	return p.step == paletteStepTask && p.task.Cancelled()
}

func (p *CommandPalette) TaskResult() (success bool, message string, err error) {
	return p.task.Result()
}

func (p *CommandPalette) ShowTask(title string, taskFn palette.TaskFunc) tea.Cmd {
	p.step = paletteStepTask
	return p.task.Init(title, taskFn)
}

func (p *CommandPalette) IsTaskStep() bool {
	return p.step == paletteStepTask
}

func (p *CommandPalette) MessageDone() bool {
	return p.step == paletteStepMessage && p.message.Done()
}

func (p *CommandPalette) MessageCancelled() bool {
	return p.step == paletteStepMessage && p.message.Cancelled()
}

func (p *CommandPalette) MessageResult() string {
	return p.message.Result()
}

func (p *CommandPalette) ShowMessage(title, message string) {
	p.step = paletteStepMessage
	p.message.Init(title, message)
}

func (p *CommandPalette) IsMessageStep() bool {
	return p.step == paletteStepMessage
}

func (p *CommandPalette) OpenWithItems(items []palette.PaletteItem, avail []bool, handler func(item palette.PaletteItem, index int) tea.Cmd) {
	p.active = true
	p.commands = nil
	p.available = avail
	p.selectionHandler = handler
	p.list.Init(items, avail)
}

func (p *CommandPalette) OpenWithGroupedItems(items []palette.PaletteItem, avail []bool, handler func(item palette.PaletteItem, index int) tea.Cmd) {
	p.active = true
	p.commands = nil
	p.available = avail
	p.selectionHandler = handler
	p.list.InitGrouped(items, avail)
}

func (p *CommandPalette) SelectionHandler() func(item palette.PaletteItem, index int) tea.Cmd {
	return p.selectionHandler
}

func (p *CommandPalette) ClearSelectionHandler() {
	p.selectionHandler = nil
}

func (p *CommandPalette) ShowSteps(steps []palette.Step, title string) tea.Cmd {
	p.step = paletteStepMultiStep
	p.stepRunner = palette.NewRunner(steps, title)
	p.stepRunner.Init()
	return p.showCurrentStep()
}

func (p *CommandPalette) showCurrentStep() tea.Cmd {
	step := p.stepRunner.CurrentStep()
	if step == nil {
		p.stepRunner = nil
		return nil
	}

	switch step.Kind() {
	case palette.StepKindList:
		items := make([]palette.PaletteItem, len(step.Options()))
		avail := make([]bool, len(step.Options()))
		for i, opt := range step.Options() {
			items[i] = multiStepItem{title: opt.Title, description: opt.Description, value: opt.Value}
			avail[i] = true
		}
		p.list.Init(items, avail)
		return nil

	case palette.StepKindInput:
		p.input.Init(step.Prompt(), step.Placeholder())
		return nil

	case palette.StepKindConfirm:
		p.confirm.Init(step.Prompt())
		return nil

	case palette.StepKindTask:
		taskFn := step.Task()
		results := p.stepRunner.Results()
		wrapped := func() (bool, string, error) {
			return taskFn(results)
		}
		return p.task.Init(step.Title(), wrapped)

	case palette.StepKindMessage:
		p.message.Init(step.Title(), step.Message())
		return nil
	}
	return nil
}

func (p *CommandPalette) renderStep() string {
	step := p.stepRunner.CurrentStep()
	if step == nil {
		return ""
	}

	p.list.SetSize(p.width, p.height)
	p.input.SetSize(p.width, p.height)
	p.confirm.SetSize(p.width, p.height)
	p.task.SetSize(p.width, p.height)
	p.message.SetSize(p.width, p.height)

	switch step.Kind() {
	case palette.StepKindList:
		return p.list.View()
	case palette.StepKindInput:
		return p.input.View()
	case palette.StepKindConfirm:
		return p.confirm.View()
	case palette.StepKindTask:
		return p.task.View()
	case palette.StepKindMessage:
		return p.message.View()
	}
	return ""
}

func (p *CommandPalette) MultiStepDone() bool {
	if p.step != paletteStepMultiStep || p.stepRunner == nil {
		return false
	}
	return p.stepRunner.IsFinished()
}

func (p *CommandPalette) MultiStepCancelled() bool {
	if p.step != paletteStepMultiStep || p.stepRunner == nil {
		return false
	}
	return p.stepRunner.IsCancelled()
}

func (p *CommandPalette) MultiStepResults() map[string]any {
	if p.stepRunner == nil {
		return nil
	}
	return p.stepRunner.Results()
}

func (p *CommandPalette) IsMultiStep() bool {
	return p.step == paletteStepMultiStep
}

func (p *CommandPalette) CancelMultiStep() {
	if p.stepRunner != nil {
		p.stepRunner.Cancel()
	}
}

// HandleMultiStepDone should be called when a palette component completes
// during multi-step mode. It records the result and advances to the next step.
// Returns a tea.Cmd if the next step needs one.
func (p *CommandPalette) HandleMultiStepDone() tea.Cmd {
	if p.stepRunner == nil {
		return nil
	}

	step := p.stepRunner.CurrentStep()
	if step == nil {
		return nil
	}

	switch step.Kind() {
	case palette.StepKindList:
		item := p.list.SelectedItem()
		if item != nil {
			if mi, ok := item.(multiStepItem); ok {
				value := mi.value
				if value == "" {
					value = mi.title
				}
				_, idx := p.list.Result()
				p.stepRunner.RecordResults(map[string]any{
					step.ResultKey():       value,
					step.ResultKey() + "_index": idx,
				})
			}
		}

	case palette.StepKindInput:
		if p.input.Done() && !p.input.Cancelled() {
			p.stepRunner.RecordResult(step.ResultKey(), p.input.Result())
		}

	case palette.StepKindConfirm:
		if p.confirm.Done() && !p.confirm.Cancelled() {
			p.stepRunner.RecordResult(step.ResultKey(), p.confirm.Result())
		}

	case palette.StepKindTask:
		// Record task result metadata and advance to next step.
		// The task function is responsible for storing its primary data
		// in the results map directly (it receives the map as input).
		success, message, err := p.task.Result()
		p.stepRunner.RecordResults(map[string]any{
			step.ResultKey() + "_ok":  success,
			step.ResultKey() + "_msg": message,
			step.ResultKey() + "_err": err,
		})

	case palette.StepKindMessage:
		// Message steps just advance on Enter.
		p.stepRunner.RecordResult(step.ResultKey(), step.Message())
	}

	if p.stepRunner.IsFinished() {
		return nil
	}

	// Check if next step should be auto-skipped (conditional input)
	p.stepRunner.SkipIfNeeded()

	if p.stepRunner.IsFinished() {
		return nil
	}

	return p.showCurrentStep()
}

func (p *CommandPalette) MultiStepInputDone() bool {
	return p.step == paletteStepMultiStep && p.stepRunner != nil &&
		p.stepRunner.CurrentStep() != nil && p.stepRunner.CurrentStep().Kind() == palette.StepKindInput &&
		p.input.Done()
}

func (p *CommandPalette) MultiStepInputCancelled() bool {
	return p.step == paletteStepMultiStep && p.stepRunner != nil &&
		p.stepRunner.CurrentStep() != nil && p.stepRunner.CurrentStep().Kind() == palette.StepKindInput &&
		p.input.Cancelled()
}

func (p *CommandPalette) MultiStepConfirmDone() bool {
	return p.step == paletteStepMultiStep && p.stepRunner != nil &&
		p.stepRunner.CurrentStep() != nil && p.stepRunner.CurrentStep().Kind() == palette.StepKindConfirm &&
		p.confirm.Done()
}

func (p *CommandPalette) MultiStepConfirmCancelled() bool {
	return p.step == paletteStepMultiStep && p.stepRunner != nil &&
		p.stepRunner.CurrentStep() != nil && p.stepRunner.CurrentStep().Kind() == palette.StepKindConfirm &&
		p.confirm.Cancelled()
}

func (p *CommandPalette) MultiStepTaskDone() bool {
	return p.step == paletteStepMultiStep && p.stepRunner != nil &&
		p.stepRunner.CurrentStep() != nil && p.stepRunner.CurrentStep().Kind() == palette.StepKindTask &&
		p.task.Done()
}

func (p *CommandPalette) MultiStepTaskCancelled() bool {
	return p.step == paletteStepMultiStep && p.stepRunner != nil &&
		p.stepRunner.CurrentStep() != nil && p.stepRunner.CurrentStep().Kind() == palette.StepKindTask &&
		p.task.Cancelled()
}

func (p *CommandPalette) MultiStepMessageDone() bool {
	return p.step == paletteStepMultiStep && p.stepRunner != nil &&
		p.stepRunner.CurrentStep() != nil && p.stepRunner.CurrentStep().Kind() == palette.StepKindMessage &&
		p.message.Done()
}

func (p *CommandPalette) MultiStepMessageCancelled() bool {
	return p.step == paletteStepMultiStep && p.stepRunner != nil &&
		p.stepRunner.CurrentStep() != nil && p.stepRunner.CurrentStep().Kind() == palette.StepKindMessage &&
		p.message.Cancelled()
}

func (p *CommandPalette) MultiStepListDone() bool {
	return p.step == paletteStepMultiStep && p.stepRunner != nil &&
		p.stepRunner.CurrentStep() != nil && p.stepRunner.CurrentStep().Kind() == palette.StepKindList &&
		p.list.Done()
}

func (p *CommandPalette) MultiStepListCancelled() bool {
	return p.step == paletteStepMultiStep && p.stepRunner != nil &&
		p.stepRunner.CurrentStep() != nil && p.stepRunner.CurrentStep().Kind() == palette.StepKindList &&
		p.list.Cancelled()
}

func (p *CommandPalette) MultiStepInputResult() string {
	return p.input.Result()
}

func (p *CommandPalette) MultiStepConfirmResult() bool {
	return p.confirm.Result()
}

func (p *CommandPalette) MultiStepTaskResult() (success bool, message string, err error) {
	return p.task.Result()
}

type multiStepItem struct {
	title       string
	description string
	value       string
}

func (m multiStepItem) Title() string       { return m.title }
func (m multiStepItem) Description() string { return m.description }
func (m multiStepItem) FilterValue() string { return m.title }

func renderPaletteItem(cmd Command, selected bool, avail bool, positions []int) string {
	prefix := "  "
	if selected {
		prefix = paletteCursorStyle.Render("> ")
	}

	nameStr := "/" + cmd.Name()
	isMultiStep := cmd.MultiStep() != nil
	isDisabled := !avail

	posSet := make(map[int]bool)
	for _, p := range positions {
		posSet[p+1] = true
	}

	var nameBuilder strings.Builder
	baseStyle := paletteNameStyle
	if selected {
		baseStyle = paletteSelectedNameStyle
	}
	if isDisabled {
		baseStyle = paletteDisabledNameStyle
	}

	for i, ch := range nameStr {
		chStr := string(ch)
		if posSet[i] {
			nameBuilder.WriteString(baseStyle.Bold(true).Render(chStr))
		} else {
			nameBuilder.WriteString(baseStyle.Render(chStr))
		}
	}
	name := nameBuilder.String()

	if isMultiStep {
		name += " " + multiStepIndicatorStyle.Render("→")
	}
	if isDisabled {
		name += " " + paletteDisabledTagStyle.Render("[unavailable]")
	}

	descStyle := paletteDescStyle
	if isDisabled {
		descStyle = paletteDisabledDescStyle
	}
	desc := descStyle.Render(cmd.Description())
	return prefix + name + "  " + desc
}
