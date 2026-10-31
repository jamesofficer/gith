package tui

import (
	"fmt"
	"image/color"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/jamesofficer/gith/internal/github"
)

const chipGray = "6B7280"

// fitLabels shows as many label chips as fit, then a count of the rest.
func fitLabels(labels []github.Label, draft bool, maxWidth int, fade color.Color) string {
	if maxWidth <= 0 {
		return ""
	}

	var chips []string

	if draft {
		chips = append(chips, chip("draft", chipGray))
	}

	for _, label := range labels {
		chips = append(chips, chip(label.Name, label.Color))
	}

	more := lipgloss.NewStyle().Foreground(fade)

	for n := len(chips); n >= 0; n-- {
		parts := chips[:n:n]

		if hidden := len(chips) - n; hidden > 0 {
			parts = append(parts, more.Render(fmt.Sprintf("+%d", hidden)))
		}

		if line := strings.Join(parts, " "); line != "" && lipgloss.Width(line) <= maxWidth {
			return line
		}
	}

	return ""
}

func commentChip(comments int) string {
	if comments < 1 {
		return ""
	}

	return chip(fmt.Sprintf("💬 %d", comments), chipGray)
}

// chip is text on a colored background. An invalid hex color shows as gray.
func chip(text, hex string) string {
	hex = strings.TrimPrefix(hex, "#")

	if len(hex) == 3 {
		hex = string([]byte{hex[0], hex[0], hex[1], hex[1], hex[2], hex[2]})
	}

	rgb, err := strconv.ParseUint(hex, 16, 24)

	if err != nil || len(hex) != 6 {
		rgb, _ = strconv.ParseUint(chipGray, 16, 24)
	}

	r, g, b := float64(rgb>>16&0xff), float64(rgb>>8&0xff), float64(rgb&0xff)
	ink := "#FAFAF9"

	if (0.299*r+0.587*g+0.114*b)/255 > 0.62 {
		ink = "#1C1917"
	}

	return lipgloss.NewStyle().
		Foreground(lipgloss.Color(ink)).
		Background(lipgloss.Color(fmt.Sprintf("#%06X", rgb))).
		Padding(0, 1).
		Render(text)
}
