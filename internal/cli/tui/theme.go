package tui

import (
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
)

func ThemeAppa() *huh.Theme {
	t := huh.ThemeBase()

	var (
		accent = lipgloss.Color("#007fff")
		cream  = lipgloss.AdaptiveColor{Light: "#FFFDF5", Dark: "#FFFDF5"}
		fg     = lipgloss.AdaptiveColor{Light: "235", Dark: "252"}
		dim    = lipgloss.AdaptiveColor{Light: "248", Dark: "238"}
		green  = lipgloss.AdaptiveColor{Light: "#02BA84", Dark: "#02BF87"}
		red    = lipgloss.AdaptiveColor{Light: "#FF4672", Dark: "#ED567A"}
	)

	t.Focused.Base = t.Focused.Base.BorderForeground(lipgloss.Color("238"))
	t.Focused.Title = t.Focused.Title.Foreground(accent).Bold(true)
	t.Focused.NoteTitle = t.Focused.NoteTitle.Foreground(accent).Bold(true).MarginBottom(1)
	t.Focused.Directory = t.Focused.Directory.Foreground(accent)
	t.Focused.Description = t.Focused.Description.Foreground(lipgloss.AdaptiveColor{Light: "", Dark: "243"})
	t.Focused.ErrorIndicator = t.Focused.ErrorIndicator.Foreground(red)
	t.Focused.ErrorMessage = t.Focused.ErrorMessage.Foreground(red)
	t.Focused.SelectSelector = t.Focused.SelectSelector.Foreground(accent)
	t.Focused.NextIndicator = t.Focused.NextIndicator.Foreground(accent)
	t.Focused.PrevIndicator = t.Focused.PrevIndicator.Foreground(accent)
	t.Focused.Option = t.Focused.Option.Foreground(fg)
	t.Focused.MultiSelectSelector = t.Focused.MultiSelectSelector.Foreground(accent)
	t.Focused.SelectedOption = t.Focused.SelectedOption.Foreground(green)
	t.Focused.SelectedPrefix = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#02CF92", Dark: "#02A877"}).SetString("✓ ")
	t.Focused.UnselectedPrefix = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "", Dark: "243"}).SetString("• ")
	t.Focused.UnselectedOption = t.Focused.UnselectedOption.Foreground(fg)
	t.Focused.FocusedButton = t.Focused.FocusedButton.Foreground(cream).Background(accent)
	t.Focused.Next = t.Focused.FocusedButton
	t.Focused.BlurredButton = t.Focused.BlurredButton.Foreground(fg).Background(lipgloss.AdaptiveColor{Light: "252", Dark: "237"})
	t.Focused.TextInput.Cursor = t.Focused.TextInput.Cursor.Foreground(green)
	t.Focused.TextInput.Placeholder = t.Focused.TextInput.Placeholder.Foreground(dim)
	t.Focused.TextInput.Prompt = t.Focused.TextInput.Prompt.Foreground(accent)

	t.Blurred = t.Focused
	t.Blurred.Base = t.Focused.Base.BorderStyle(lipgloss.HiddenBorder())
	t.Blurred.NextIndicator = lipgloss.NewStyle()
	t.Blurred.PrevIndicator = lipgloss.NewStyle()

	t.Group.Title = t.Focused.Title
	t.Group.Description = t.Focused.Description
	return t
}
