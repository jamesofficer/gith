package tui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/jamesofficer/gith/internal/github"
)

// ago is a short age such as now, 5m, 3h, 2d, 4mo, or 1y.
func ago(t, now time.Time) string {
	if t.IsZero() {
		return ""
	}

	d := now.Sub(t)
	day := 24 * time.Hour

	switch {
	case d < time.Minute:
		return "now"

	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))

	case d < day:
		return fmt.Sprintf("%dh", int(d.Hours()))

	case d < 30*day:
		return fmt.Sprintf("%dd", int(d/day))

	case d < 365*day:
		return fmt.Sprintf("%dmo", int(d/(30*day)))

	default:
		return fmt.Sprintf("%dy", int(d/(365*day)))
	}
}

// agoLong is ago with "ago" added, for a detail header.
func agoLong(t, now time.Time) string {
	short := ago(t, now)

	if short == "" || short == "now" {
		return "just now"
	}

	return short + " ago"
}

// count is a number with its noun, such as "1 file" or "3 files".
func count(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}

	return fmt.Sprintf("%d %ss", n, noun)
}

func kindTitle(kind github.Kind) string {
	if kind == github.KindPull {
		return "Pull requests"
	}

	return "Issues"
}

func kindName(kind github.Kind) string {
	return strings.ToLower(kindTitle(kind))
}

func stateStyle(state string, colors tone) lipgloss.Style {
	switch strings.ToUpper(state) {
	case "OPEN":
		return lipgloss.NewStyle().Foreground(colors.good)

	case "MERGED":
		return lipgloss.NewStyle().Foreground(colors.accent)

	default:
		return lipgloss.NewStyle().Foreground(colors.muted)
	}
}

// decisionName is the review decision of a pull request.
func decisionName(decision string) string {
	switch decision {
	case "APPROVED":
		return "approved"

	case "CHANGES_REQUESTED":
		return "changes requested"

	case "REVIEW_REQUIRED":
		return "review required"

	default:
		return ""
	}
}

// verdictName is the state of one review in the thread.
func verdictName(state string) string {
	switch state {
	case "":
		return ""

	case "APPROVED":
		return "approved"

	case "CHANGES_REQUESTED":
		return "requested changes"

	case "DISMISSED":
		return "review dismissed"

	default:
		return "reviewed"
	}
}

// spread puts left and right at the two ends of a line. It cuts left
// when the line is too narrow, so that right stays on screen.
func spread(left, right string, width int) string {
	rightW := lipgloss.Width(right)

	if rightW == 0 || rightW+2 > width {
		return fitLine(left, width)
	}

	left = fitLine(left, width-rightW-1)

	return left + strings.Repeat(" ", width-lipgloss.Width(left)-rightW) + right
}

// fitLine cuts or pads text to exactly width cells.
func fitLine(text string, width int) string {
	if width <= 0 {
		return ""
	}

	if lipgloss.Width(text) > width {
		return ansi.Truncate(text, width, "…")
	}

	return padRight(text, width)
}

func padRight(text string, width int) string {
	return text + strings.Repeat(" ", max(0, width-lipgloss.Width(text)))
}

func padLeft(text string, width int) string {
	return strings.Repeat(" ", max(0, width-lipgloss.Width(text))) + text
}
