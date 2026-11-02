package tui

import (
	"strconv"
	"strings"

	"github.com/jamesofficer/gith/internal/github"
)

// list is the open items of one tab. Use its methods, not all, match, and
// cursor, because the cursor counts only the items that match the filter.
type list struct {
	loading bool
	err     error

	all []github.Item

	// words is the filter. match holds the index in all of each item that
	// matches it, in order.
	words  []string
	match  []int
	cursor int
}

// setItems puts in new items and keeps the selected item when it is there.
func (l *list) setItems(items []github.Item) {
	selected, ok := l.current()
	l.all = items
	l.refilter()
	l.cursor = 0

	if ok {
		l.selectNumber(selected.Number)
	}
}

// filter shows only the items that match each word. It selects the first
// match. An empty filter shows all items and keeps the selected item.
func (l *list) filter(words []string) {
	selected, ok := l.current()
	l.words = words
	l.refilter()
	l.cursor = 0

	if ok && len(words) == 0 {
		l.selectNumber(selected.Number)
	}
}

func (l *list) refilter() {
	l.match = l.match[:0]

	for i, item := range l.all {
		if matches(item, l.words) {
			l.match = append(l.match, i)
		}
	}
}

func (l *list) selectNumber(number int) {
	for i, index := range l.match {
		if l.all[index].Number == number {
			l.cursor = i
		}
	}
}

// filtered tells if the list has a filter.
func (l list) filtered() bool {
	return len(l.words) > 0
}

// total is the number of items before the filter.
func (l list) total() int {
	return len(l.all)
}

// shown is the items that match the filter.
func (l list) shown() []github.Item {
	items := make([]github.Item, len(l.match))

	for i, index := range l.match {
		items[i] = l.all[index]
	}

	return items
}

// position is the index of the selected item in shown.
func (l list) position() int {
	return l.cursor
}

// current is the selected item.
func (l list) current() (github.Item, bool) {
	if l.cursor < 0 || l.cursor >= len(l.match) {
		return github.Item{}, false
	}

	return l.all[l.match[l.cursor]], true
}

func (l *list) move(delta int) {
	l.cursor = step(l.cursor, delta, len(l.match))
}

func (l *list) setComments(number, count int) {
	for i := range l.all {
		if l.all[i].Number == number {
			l.all[i].Comments = count
		}
	}
}

// matches tells if each word is in the title, the author, or an assignee of
// the item. A number such as 12 or #12 also matches the start of the number.
func matches(item github.Item, words []string) bool {
	text := strings.ToLower(strings.Join(append([]string{item.Title, item.Author}, item.Assignees...), " "))
	number := strconv.Itoa(item.Number)

	for _, word := range words {
		if !strings.Contains(text, word) && !strings.HasPrefix("#"+number, word) && !strings.HasPrefix(number, word) {
			return false
		}
	}

	return true
}
