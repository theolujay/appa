package output

import (
	"fmt"
	"image/color"
	"os"
	"strings"

	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/table"
)

var (
	Purple    = lipgloss.Color("#7D56F4")
	Green     = lipgloss.Color("#43BF6D")
	Red       = lipgloss.Color("#FF5555")
	Yellow    = lipgloss.Color("#F1FA8C")
	Cyan      = lipgloss.Color("#8BE9FD")
	Gray      = lipgloss.Color("#6272A4")
	White     = lipgloss.Color("#FAFAFA")
	DarkBg    = lipgloss.Color("#1A1A2E")
	DarkCard  = lipgloss.Color("#16213E")
	DraculaBg = lipgloss.Color("#282A36")
	DraculaFg = lipgloss.Color("#F8F8F2")
)

var (
	SectionStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(Purple)

	SuccessStyle = lipgloss.NewStyle().
			Foreground(Green)

	ErrorLabelStyle = lipgloss.NewStyle().
			Background(Red).
			Foreground(White).
			Bold(true).
			Padding(0, 1)

	WarnStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(Yellow)

	SubtleStyle = lipgloss.NewStyle().
			Foreground(Gray)

	BadgeStyle = lipgloss.NewStyle().
			Foreground(White).
			Bold(true).
			Padding(0, 1)
)

func StatusBadge(text string, c color.Color) string {
	return BadgeStyle.Background(c).Render(text)
}

func Check(label string, ok bool, args ...any) {
	s := fmt.Sprintf(label, args...)
	if ok {
		lipgloss.Printf("%s %s\n", BoldGreen("✓"), s)
	} else {
		lipgloss.Printf("%s %s\n", BoldRed("✗"), s)
	}
}

func Warn(format string, args ...any) {
	lipgloss.Printf("%s\n", WarnStyle.Render("! "+fmt.Sprintf(format, args...)))
}

func Section(format string, args ...any) {
	title := fmt.Sprintf(format, args...)
	lipgloss.Println()
	lipgloss.Println(SectionStyle.Render(title))
	lipgloss.Println(SubtleStyle.Render(strings.Repeat("─", len(title))))
}

func Error(format string, args ...any) {
	lipgloss.Fprintf(os.Stderr, "%s %s\n",
		ErrorLabelStyle.Render("Error"),
		fmt.Sprintf(format, args...))
}

func Success(format string, args ...any) {
	lipgloss.Println(SuccessStyle.Render(fmt.Sprintf(format, args...)))
}

func PrintTable(header []string, rows [][]string, dimmed []bool) {
	t := table.New().
		Border(lipgloss.NormalBorder()).
		BorderStyle(lipgloss.NewStyle().Foreground(Gray)).
		StyleFunc(func(row, col int) lipgloss.Style {
			switch {
			case row == table.HeaderRow:
				return lipgloss.NewStyle().
					Bold(true).
					Foreground(Purple).
					Align(lipgloss.Center).
					Padding(0, 1)
			case dimmed != nil && dimmed[row]:
				return lipgloss.NewStyle().
					Foreground(Gray).
					Padding(0, 1)
			default:
				return lipgloss.NewStyle().Padding(0, 1)
			}

		}).
		Headers(header...).
		Rows(rows...)
	lipgloss.Println(t)
}

func Header(title string) {
	Section("%s", title)
}

func Panel(header, body string) {
	top := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder(), true, false).
		BorderForeground(Purple).
		Foreground(Purple).
		Bold(true).
		Padding(0, 1).
		Width(80).
		Render(header)
	content := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder(), false, true, true, true).
		BorderForeground(Purple).
		Padding(0, 2).
		Width(78).
		Render(body)
	lipgloss.Printf("%s\n%s\n\n", top, content)
}

func KV(key, value string) {
	k := lipgloss.NewStyle().Foreground(Purple).Bold(true).Render(key)
	sep := SubtleStyle.Render(" • ")
	lipgloss.Printf("  %s %s %s\n", k, sep, value)
}

func Step(n int, label string) {
	num := lipgloss.NewStyle().
		Foreground(White).
		Background(Purple).
		Padding(0, 1).
		Render(fmt.Sprintf(" %d ", n))
	lipgloss.Printf("%s %s\n", num, label)
}

func BoldGreen(s string) string {
	return lipgloss.NewStyle().Foreground(Green).Bold(true).Render(s)
}

func BoldRed(s string) string {
	return lipgloss.NewStyle().Foreground(Red).Bold(true).Render(s)
}

func Faint(s string) string {
	return lipgloss.NewStyle().Foreground(Gray).Render(s)
}

