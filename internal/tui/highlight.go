package tui

import (
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
)

// highlightLines splits lines of code into tokens for syntax colors. It
// reads the lines as one text, so a comment over many lines gets the same
// color on each line. It returns one row of tokens for each line.
func highlightLines(filename string, lines []string) [][]chroma.Token {
	lexer := lexers.Match(filename)

	if lexer == nil {
		lexer = lexers.Fallback
	}

	text := strings.Join(lines, "\n") + "\n"
	iterator, err := chroma.Coalesce(lexer).Tokenise(nil, text)

	// Colors are only a help to read. Show plain text when they fail.
	if err != nil {
		iterator = chroma.Literator(chroma.Token{Type: chroma.Text, Value: text})
	}

	rows := chroma.SplitTokensIntoLines(iterator.Tokens())
	rows = append(rows, make([][]chroma.Token, max(0, len(lines)-len(rows)))...)

	for i, row := range rows[:len(lines)] {
		rows[i] = trimNewline(row)
	}

	return rows[:len(lines)]
}

func trimNewline(row []chroma.Token) []chroma.Token {
	if len(row) == 0 {
		return row
	}

	last := &row[len(row)-1]
	last.Value = strings.TrimSuffix(last.Value, "\n")

	if last.Value == "" {
		return row[:len(row)-1]
	}

	return row
}
