package tui

import (
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/jamesofficer/gith/internal/github"
)

// searchSource has three issues with different titles, authors, and
// assignees.
func searchSource() *fakeSource {
	src := newSource()
	src.issues = []github.Item{
		{Number: 12, Title: "Fix the timer", Author: "grace", Assignees: []string{"lin"}, UpdatedAt: when},
		{Number: 9, Title: "Document the cache", Author: "ada", UpdatedAt: when},
		{Number: 120, Title: "Speed up the timer", Author: "grace", UpdatedAt: when},
	}

	return src
}

func TestSearchMatchesTitleAuthorAssigneeAndNumber(t *testing.T) {
	titles := []string{"Fix the timer", "Document the cache", "Speed up the timer"}
	cases := []struct {
		query string
		want  []string
	}{
		{"timer", []string{"Fix the timer", "Speed up the timer"}},
		{"ADA", []string{"Document the cache"}},
		{"lin", []string{"Fix the timer"}},
		{"grace speed", []string{"Speed up the timer"}},
		{"#12", []string{"Fix the timer", "Speed up the timer"}},
		{"9", []string{"Document the cache"}},
	}

	for _, c := range cases {
		t.Run(c.query, func(t *testing.T) {
			m := typeText(t, press(t, start(t, searchSource()), char('/')), c.query)
			view := screen(m)

			for _, title := range titles {
				if strings.Contains(view, title) != slices.Contains(c.want, title) {
					t.Fatalf("%q shows = %t:\n%s", title, !slices.Contains(c.want, title), view)
				}
			}

			if !strings.Contains(lineWith(view, c.want[0]), "❯") {
				t.Fatalf("first match is not selected:\n%s", view)
			}

			assertFillsScreen(t, m)
		})
	}
}

func TestSearchShowsWhenNothingMatches(t *testing.T) {
	m := typeText(t, press(t, start(t, searchSource()), char('/')), "nothing")

	if view := screen(m); !strings.Contains(view, "No open issues match the search.") || !strings.Contains(view, "Issues  0 of 3") {
		t.Fatalf("view =\n%s", view)
	}
}

func TestSearchTakesTypedKeysAndEnterOpensTheMatch(t *testing.T) {
	src := searchSource()
	src.details[120] = github.Detail{Item: src.issues[2], Body: "Make it faster."}
	m := press(t, start(t, src), char('/'))
	m, cmd := update(t, m, char('q'))

	if quits(cmd) || !strings.Contains(screen(m), "/ q") {
		t.Fatalf("q did not go into the search:\n%s", screen(m))
	}

	m = press(t, m, key(tea.KeyBackspace))
	m = press(t, typeText(t, m, "timer"), key(tea.KeyDown))

	if m = press(t, m, key(tea.KeyEnter)); !strings.Contains(screen(m), "Make it faster.") {
		t.Fatalf("enter did not open the second match:\n%s", screen(m))
	}

	m = press(t, m, key(tea.KeyEsc))

	if view := screen(m); !strings.Contains(view, "/ timer") || strings.Contains(view, "Document the cache") {
		t.Fatalf("search did not stay after the item closed:\n%s", view)
	}

	m = press(t, m, key(tea.KeyEsc))

	if view := screen(m); strings.Contains(view, "/ timer") || !strings.Contains(view, "Document the cache") || !strings.Contains(lineWith(view, "Speed up the timer"), "❯") {
		t.Fatalf("esc did not clear the search and keep the selection:\n%s", view)
	}
}

func TestSearchFiltersBothTabs(t *testing.T) {
	m := typeText(t, press(t, start(t, searchSource()), char('/')), "ada")
	m = press(t, press(t, m, key(tea.KeyEnter)), key(tea.KeyEsc))
	m = press(t, m, key(tea.KeyRight))
	view := screen(m)

	if !strings.Contains(view, "Pull requests  1 of 1") || !strings.Contains(view, "Add a setting") || !strings.Contains(view, "Issues  1 of 3") {
		t.Fatalf("view =\n%s", view)
	}
}

func TestRefreshKeepsTheSelectedItem(t *testing.T) {
	src := searchSource()
	m := press(t, start(t, src), key(tea.KeyDown))
	src.issues = append([]github.Item{{Number: 130, Title: "New issue", Author: "sam", UpdatedAt: when}}, src.issues...)

	if m = press(t, m, char('r')); !strings.Contains(lineWith(screen(m), "Document the cache"), "❯") {
		t.Fatalf("selection moved after the refresh:\n%s", screen(m))
	}
}

func TestRowsShowTheAuthor(t *testing.T) {
	if row := lineWith(screen(start(t, searchSource())), "Document the cache"); !strings.Contains(row, "by ada") {
		t.Fatalf("row = %q", row)
	}
}
