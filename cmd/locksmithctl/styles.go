package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Colour palette.
var (
	colorBrand   = lipgloss.Color("#7C3AED")
	colorSuccess = lipgloss.Color("#10B981")
	colorError   = lipgloss.Color("#EF4444")
	colorInfo    = lipgloss.Color("#60A5FA")
	colorWarn    = lipgloss.Color("#F59E0B")
	colorMuted   = lipgloss.Color("#9CA3AF")
	colorLock    = lipgloss.Color("#FBBF24")
)

// Text styles.
var (
	titleStyle   = lipgloss.NewStyle().Bold(true).Foreground(colorBrand)
	successStyle = lipgloss.NewStyle().Bold(true).Foreground(colorSuccess)
	errorStyle   = lipgloss.NewStyle().Bold(true).Foreground(colorError)
	infoStyle    = lipgloss.NewStyle().Foreground(colorInfo)
	warnStyle    = lipgloss.NewStyle().Bold(true).Foreground(colorWarn)
	mutedStyle   = lipgloss.NewStyle().Foreground(colorMuted)
	lockTagStyle = lipgloss.NewStyle().Bold(true).Foreground(colorLock)
	cmdColStyle  = lipgloss.NewStyle().Foreground(colorBrand).Width(24)
)

// Box styles.
var (
	bannerStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorBrand).
			Padding(0, 2)

	lockBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorLock).
			Padding(0, 1)

	warnBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorWarn).
			Padding(0, 1)
)

// promptStr returns the REPL prompt string.
// This must be plain text with no ANSI escape sequences: liner validates every
// rune in the prompt and returns ErrInvalidPrompt (breaking the REPL loop) the
// moment it encounters any control character such as ESC (\x1b).
func promptStr() string {
	return "locksmith> "
}

// renderBanner renders the startup banner box.
func renderBanner(addr string) string {
	content := titleStyle.Render("🔑 locksmithctl") + "\n" +
		mutedStyle.Render("Interactive Locksmith Lock Manager") + "\n" +
		infoStyle.Render("Connecting to "+addr)
	return bannerStyle.Render(content)
}

// renderLockList renders the active-lock table inside a rounded box.
func renderLockList(locks []string) string {
	var sb strings.Builder
	sb.WriteString(lockTagStyle.Render(fmt.Sprintf("Active Locks (%d)", len(locks))))
	for i, l := range locks {
		sb.WriteString(fmt.Sprintf("\n  %s  %s",
			mutedStyle.Render(fmt.Sprintf("%2d.", i+1)),
			l,
		))
	}
	return lockBoxStyle.Render(sb.String())
}

// renderDisconnectWarning renders the disconnect notification box.
func renderDisconnectWarning() string {
	return warnBoxStyle.Render(
		warnStyle.Render("⚠  Disconnected from server") + "\n" +
			mutedStyle.Render("Type 'reconnect' to reconnect"),
	)
}

// printHelp prints the styled command reference.
func printHelp() {
	type row struct{ cmd, desc string }
	rows := []row{
		{"acquire [lock]", "Acquire a lock (interactive prompt if omitted)"},
		{"release [lock]", "Release a lock (picker if omitted)"},
		{"list", "Show all acquired locks"},
		{"reconnect", "Reconnect to the server"},
		{"help", "Show this help"},
		{"exit / quit", "Exit locksmithctl"},
	}
	var sb strings.Builder
	sb.WriteString(titleStyle.Render("Commands") + "\n")
	for _, r := range rows {
		sb.WriteString("  " + cmdColStyle.Render(r.cmd) + mutedStyle.Render(r.desc) + "\n")
	}
	fmt.Print(sb.String())
}
