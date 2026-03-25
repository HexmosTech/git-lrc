package appcore

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"github.com/HexmosTech/git-lrc/internal/decisionflow"
)

type decisionPrompt struct {
	Title        string
	Description  string
	Metadata     []string
	InitialText  string
	FocusMessage bool
	AllowCommit  bool
	AllowPush    bool
	AllowAbort   bool
	AllowSkip    bool
	AllowVouch   bool

	RequireMessageForCommit bool
	RequireMessageForSkip   bool
	RequireMessageForVouch  bool
}

type terminalDecision struct {
	Code    int
	Message string
	Push    bool
}

type tuiStatusMsg struct {
	Text string
}

type tuiDraftMsg struct {
	Text    string
	Version int64
}

type decisionAction struct {
	Label           string
	Help            string
	Code            int
	Push            bool
	RequiresMessage bool
}

type decisionTUIModel struct {
	prompt   decisionPrompt
	actions  []decisionAction
	selected int
	focus    int // 0=actions, 1=textbox
	message  []rune
	cursor   int
	status   string
	errorMsg string
	decided  bool
	width    int
	compact  bool
	output   chan<- terminalDecision
	draftVer int64
	onDraft  func(string) int64
	onEditor func() (string, int64, error)
}

func (m decisionTUIModel) Init() tea.Cmd {
	return nil
}

func (m decisionTUIModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.decided {
		return m, tea.Quit
	}

	switch v := msg.(type) {
	case tuiStatusMsg:
		clean := strings.TrimSpace(v.Text)
		clean = strings.TrimPrefix(clean, "Status:")
		m.status = strings.TrimSpace(clean)
		return m, nil
	case tuiDraftMsg:
		if v.Version <= 0 || v.Version < m.draftVer {
			return m, nil
		}
		if string(m.message) != v.Text {
			m.message = []rune(v.Text)
			m.cursor = len(m.message)
		}
		m.draftVer = v.Version
		return m, nil
	case tea.WindowSizeMsg:
		m.width = v.Width
		m.compact = v.Width > 0 && v.Width < 100
		return m, nil
	case tea.KeyPressMsg:
		key := strings.ToLower(v.String())
		if key == "ctrl+c" {
			if m.prompt.AllowAbort {
				if action, ok := m.actionForCode(decisionflow.DecisionAbort, false); ok {
					if done := m.trySubmit(action); done {
						return m, tea.Quit
					}
				}
			}
			return m, nil
		}

		if key == "tab" || key == "shift+tab" {
			m.focus = (m.focus + 1) % 2
			return m, nil
		}

		if m.focus == 1 {
			switch key {
			case "left":
				if m.cursor > 0 {
					m.cursor--
				}
				return m, nil
			case "right":
				if m.cursor < len(m.message) {
					m.cursor++
				}
				return m, nil
			case "home":
				m.cursor = 0
				return m, nil
			case "end":
				m.cursor = len(m.message)
				return m, nil
			case "backspace", "ctrl+h":
				if m.cursor > 0 {
					m.message = append(m.message[:m.cursor-1], m.message[m.cursor:]...)
					m.cursor--
					m.errorMsg = ""
					m.publishDraftChange()
				}
				return m, nil
			case "delete":
				if m.cursor < len(m.message) {
					m.message = append(m.message[:m.cursor], m.message[m.cursor+1:]...)
					m.errorMsg = ""
					m.publishDraftChange()
				}
				return m, nil
			case "ctrl+enter", "ctrl+j":
				action, ok := m.currentAction()
				if ok {
					if done := m.trySubmit(action); done {
						return m, tea.Quit
					}
				}
				return m, nil
			case "enter":
				m.insertRune('\n')
				m.errorMsg = ""
				m.publishDraftChange()
				return m, nil
			}

			if m.insertKeyText(key) {
				m.errorMsg = ""
				m.publishDraftChange()
				return m, nil
			}
			return m, nil
		}

		switch key {
		case "e", "ctrl+e":
			if m.onEditor != nil {
				text, version, err := m.onEditor()
				if err != nil {
					m.errorMsg = "editor failed: " + err.Error()
					return m, nil
				}
				m.message = []rune(text)
				m.cursor = len(m.message)
				if version > 0 {
					m.draftVer = version
				}
				m.errorMsg = ""
				return m, nil
			}
		case "q", "esc":
			if m.prompt.AllowAbort {
				if action, ok := m.actionForCode(decisionflow.DecisionAbort, false); ok {
					if done := m.trySubmit(action); done {
						return m, tea.Quit
					}
				}
			}
		case "up", "k":
			if len(m.actions) > 0 {
				m.selected = (m.selected - 1 + len(m.actions)) % len(m.actions)
			}
			return m, nil
		case "down", "j":
			if len(m.actions) > 0 {
				m.selected = (m.selected + 1) % len(m.actions)
			}
			return m, nil
		case "enter":
			action, ok := m.currentAction()
			if ok {
				if done := m.trySubmit(action); done {
					return m, tea.Quit
				}
			}
			return m, nil
		case "ctrl+s", "s":
			if action, ok := m.actionForCode(decisionflow.DecisionSkip, false); ok {
				if done := m.trySubmit(action); done {
					return m, tea.Quit
				}
			}
			return m, nil
		case "ctrl+v", "ctrl+y", "v", "y":
			if action, ok := m.actionForCode(decisionflow.DecisionVouch, false); ok {
				if done := m.trySubmit(action); done {
					return m, tea.Quit
				}
			}
			return m, nil
		case "p":
			if action, ok := m.actionForCode(decisionflow.DecisionCommit, true); ok {
				if done := m.trySubmit(action); done {
					return m, tea.Quit
				}
			}
			return m, nil
		case "c":
			if action, ok := m.actionForCode(decisionflow.DecisionCommit, false); ok {
				if done := m.trySubmit(action); done {
					return m, tea.Quit
				}
			}
			return m, nil
		}

	}

	return m, nil
}

func (m decisionTUIModel) View() tea.View {
	var lines []string
	lines = append(lines, "")
	lines = append(lines, styleHeader("+-------------------------------------------+"))
	lines = append(lines, styleHeader("|            LiveReview Decision            |"))
	lines = append(lines, styleHeader("+-------------------------------------------+"))
	if strings.TrimSpace(m.prompt.Title) != "" {
		lines = append(lines, styleTitle(m.prompt.Title))
	}
	if strings.TrimSpace(m.prompt.Description) != "" {
		lines = append(lines, styleMuted(m.prompt.Description))
	}
	if len(m.prompt.Metadata) > 0 {
		lines = append(lines, "")
		lines = append(lines, m.renderMetadata()...)
	}
	lines = append(lines, "")
	lines = append(lines, m.renderActions()...)
	lines = append(lines, "")
	lines = append(lines, m.renderTextbox()...)
	lines = append(lines, "")
	lines = append(lines, styleMuted("Keys: Tab switch focus, Up/Down select action"))
	lines = append(lines, styleMuted("Actions: Enter confirm | Message: Enter newline, Ctrl-Enter confirm"))
	if !m.compact {
		lines = append(lines, styleMuted("Shortcuts (actions focus): C commit, P commit+push, S skip, V vouch, Q abort, Ctrl-C abort"))
		if m.onEditor != nil {
			lines = append(lines, styleMuted("Optional: E open in editor"))
		}
	}
	if m.errorMsg != "" {
		lines = append(lines, "")
		lines = append(lines, styleError("Error: "+m.errorMsg))
	}
	if m.status != "" {
		lines = append(lines, "")
		lines = append(lines, styleStatus("Status: "+m.status))
	}
	lines = append(lines, "")
	return tea.NewView(strings.Join(lines, "\n"))
}

func (m *decisionTUIModel) submit(d terminalDecision) {
	if m.decided {
		return
	}
	m.decided = true
	switch d.Code {
	case decisionflow.DecisionCommit:
		if d.Push {
			m.status = "commit and push selected"
		} else {
			m.status = "commit selected"
		}
	case decisionflow.DecisionSkip:
		m.status = "skip selected"
	case decisionflow.DecisionVouch:
		m.status = "vouch selected"
	case decisionflow.DecisionAbort:
		m.status = "abort selected"
	}
	select {
	case m.output <- d:
	default:
	}
}

func startTerminalDecisionBubbleTea(prompt decisionPrompt, onDraftChange func(string) int64, openEditor func() (string, int64, error)) (<-chan terminalDecision, func(string), func(string, int64), func(), <-chan struct{}) {
	decisionCh := make(chan terminalDecision, 1)
	statusCh := make(chan string, 32)
	draftCh := make(chan tuiDraftMsg, 64)
	doneCh := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		defer close(doneCh)
		defer close(decisionCh)

		model := newDecisionTUIModel(prompt, decisionCh, onDraftChange, openEditor)
		program := tea.NewProgram(model, tea.WithContext(ctx))

		forwardDone := make(chan struct{})
		go func() {
			defer close(forwardDone)
			for {
				select {
				case <-ctx.Done():
					return
				case s := <-statusCh:
					program.Send(tuiStatusMsg{Text: s})
				}
			}
		}()

		draftForwardDone := make(chan struct{})
		go func() {
			defer close(draftForwardDone)
			for {
				select {
				case <-ctx.Done():
					return
				case d := <-draftCh:
					program.Send(d)
				}
			}
		}()

		_, _ = program.Run()
		cancel()
		<-forwardDone
		<-draftForwardDone
	}()

	setStatus := func(text string) {
		trimmed := strings.TrimSpace(text)
		if trimmed == "" {
			return
		}
		select {
		case statusCh <- trimmed:
		default:
		}
	}

	stop := func() {
		cancel()
	}

	setDraft := func(text string, version int64) {
		select {
		case draftCh <- tuiDraftMsg{Text: text, Version: version}:
		default:
		}
	}

	return decisionCh, setStatus, setDraft, stop, doneCh
}

func newDecisionTUIModel(prompt decisionPrompt, output chan<- terminalDecision, onDraftChange func(string) int64, openEditor func() (string, int64, error)) decisionTUIModel {
	actions := make([]decisionAction, 0, 5)
	if prompt.AllowCommit {
		actions = append(actions, decisionAction{Label: "Commit", Help: "continue with commit", Code: decisionflow.DecisionCommit, RequiresMessage: prompt.RequireMessageForCommit})
		if prompt.AllowPush {
			actions = append(actions, decisionAction{Label: "Commit & Push", Help: "continue and push", Code: decisionflow.DecisionCommit, Push: true, RequiresMessage: prompt.RequireMessageForCommit})
		}
	}
	if prompt.AllowSkip {
		actions = append(actions, decisionAction{Label: "Skip", Help: "skip review and continue", Code: decisionflow.DecisionSkip, RequiresMessage: prompt.RequireMessageForSkip})
	}
	if prompt.AllowVouch {
		actions = append(actions, decisionAction{Label: "Vouch", Help: "vouch and continue", Code: decisionflow.DecisionVouch, RequiresMessage: prompt.RequireMessageForVouch})
	}
	if prompt.AllowAbort {
		actions = append(actions, decisionAction{Label: "Abort", Help: "abort current flow", Code: decisionflow.DecisionAbort})
	}

	message := []rune(prompt.InitialText)
	return decisionTUIModel{
		prompt:   prompt,
		actions:  actions,
		selected: 0,
		focus:    initialFocus(prompt.FocusMessage),
		message:  message,
		cursor:   len(message),
		output:   output,
		width:    80,
		compact:  true,
		onDraft:  onDraftChange,
		onEditor: openEditor,
	}
}

func initialFocus(focusMessage bool) int {
	if focusMessage {
		return 1
	}
	return 0
}

func (m *decisionTUIModel) currentAction() (decisionAction, bool) {
	if len(m.actions) == 0 {
		return decisionAction{}, false
	}
	if m.selected < 0 || m.selected >= len(m.actions) {
		m.selected = 0
	}
	return m.actions[m.selected], true
}

func (m *decisionTUIModel) actionForCode(code int, push bool) (decisionAction, bool) {
	for _, a := range m.actions {
		if a.Code == code && a.Push == push {
			return a, true
		}
	}
	return decisionAction{}, false
}

func (m *decisionTUIModel) trySubmit(action decisionAction) bool {
	msg := strings.TrimSpace(string(m.message))
	if action.RequiresMessage && msg == "" {
		m.focus = 1
		switch action.Code {
		case decisionflow.DecisionCommit:
			m.errorMsg = "commit message is required"
		case decisionflow.DecisionSkip:
			m.errorMsg = "skip requires a message"
		case decisionflow.DecisionVouch:
			m.errorMsg = "vouch requires a message"
		default:
			m.errorMsg = "message is required"
		}
		return false
	}

	m.errorMsg = ""
	m.submit(terminalDecision{Code: action.Code, Message: msg, Push: action.Push})
	return true
}

func (m *decisionTUIModel) insertKeyText(key string) bool {
	if key == "space" {
		m.insertRune(' ')
		return true
	}

	if utf8.RuneCountInString(key) == 1 {
		r, _ := utf8.DecodeRuneInString(key)
		if r >= 32 && r != 127 {
			m.insertRune(r)
			return true
		}
	}

	return false
}

func (m *decisionTUIModel) insertRune(r rune) {
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor > len(m.message) {
		m.cursor = len(m.message)
	}

	m.message = append(m.message[:m.cursor], append([]rune{r}, m.message[m.cursor:]...)...)
	m.cursor++
}

func (m *decisionTUIModel) publishDraftChange() {
	if m.onDraft == nil {
		return
	}
	if version := m.onDraft(string(m.message)); version > 0 {
		m.draftVer = version
	}
}

func (m decisionTUIModel) renderActions() []string {
	lines := []string{styleSection("+ Actions +")}
	for i, action := range m.actions {
		prefix := "  "
		if i == m.selected {
			prefix = styleMuted("* ")
		}
		if m.focus == 0 && i == m.selected {
			prefix = styleFocus("▶ ")
		}
		if m.compact {
			lines = append(lines, prefix+styleAction(action.Label, i == m.selected))
		} else {
			lines = append(lines, prefix+styleAction(action.Label, i == m.selected)+" "+styleMuted("- "+action.Help))
		}
	}
	return lines
}

func (m decisionTUIModel) renderMetadata() []string {
	lines := []string{styleSection("+ Metadata +")}
	for _, item := range m.prompt.Metadata {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			continue
		}
		lines = append(lines, styleMuted(trimmed))
	}
	return lines
}

func (m decisionTUIModel) renderTextbox() []string {
	focusMark := styleMuted("  ")
	if m.focus == 1 {
		focusMark = styleFocus("▶ ")
	}
	lines := []string{styleSection("+ Message +")}
	rendered := m.renderMessageLines()
	for i, line := range rendered {
		prefix := styleMuted("  ")
		if i == 0 {
			prefix = focusMark
		}
		if line == "" {
			line = " "
		}
		lines = append(lines, prefix+line)
	}
	return lines
}

func (m decisionTUIModel) renderMessageLines() []string {
	if m.focus != 1 && len(m.message) == 0 {
		return []string{styleMuted("<empty>")}
	}

	cursor := m.cursor
	if cursor < 0 {
		cursor = 0
	}
	if cursor > len(m.message) {
		cursor = len(m.message)
	}

	withCursor := make([]rune, 0, len(m.message)+1)
	for i := 0; i <= len(m.message); i++ {
		if m.focus == 1 && i == cursor {
			withCursor = append(withCursor, '|')
		}
		if i < len(m.message) {
			withCursor = append(withCursor, m.message[i])
		}
	}

	text := string(withCursor)
	if text == "" {
		text = "|"
	}

	return strings.Split(text, "\n")
}

func styleHeader(s string) string { return "\x1b[1;38;5;51m" + s + "\x1b[0m" }
func styleTitle(s string) string  { return "\x1b[1;38;5;255m" + s + "\x1b[0m" }
func styleSection(s string) string {
	return "\x1b[1;38;5;117m" + strings.ToUpper(s) + "\x1b[0m"
}
func styleMuted(s string) string  { return "\x1b[38;5;245m" + s + "\x1b[0m" }
func styleError(s string) string  { return "\x1b[1;38;5;203m" + s + "\x1b[0m" }
func styleStatus(s string) string { return "\x1b[1;38;5;84m" + s + "\x1b[0m" }
func styleFocus(s string) string  { return "\x1b[1;38;5;214m" + s + "\x1b[0m" }

func styleAction(s string, selected bool) string {
	if selected {
		return fmt.Sprintf("\x1b[1;38;5;16;48;5;45m %s \x1b[0m", s)
	}
	return "\x1b[38;5;252m" + s + "\x1b[0m"
}
