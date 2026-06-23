package tui

import (
	"os"

	tea "charm.land/bubbletea/v2"
)

func NewProgram(model tea.Model, opts ...tea.ProgramOption) *tea.Program {
	return tea.NewProgram(model, opts...)
}

func Run(model tea.Model) error {
	p := NewProgram(model)
	_, err := p.Run()
	return err
}

func LogToFile(path string) {
	if _, err := tea.LogToFile(path, "appa-tui"); err != nil {
		os.Stderr.WriteString("failed to write log: " + err.Error() + "\n")
	}
}

type errMsg struct{ Err error }

func (e errMsg) Error() string { return e.Err.Error() }
