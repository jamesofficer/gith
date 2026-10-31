package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"

	tea "charm.land/bubbletea/v2"

	"github.com/jamesofficer/gith/internal/github"
	"github.com/jamesofficer/gith/internal/tui"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	var repo string
	var limit int

	flag.StringVar(&repo, "R", "", "Repository as owner/name. The default is the repository in this directory.")
	flag.StringVar(&repo, "repo", "", "Repository as owner/name. The default is the repository in this directory.")
	flag.IntVar(&limit, "L", 30, "Maximum number of open items to load.")
	flag.IntVar(&limit, "limit", 30, "Maximum number of open items to load.")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, `Usage: gith [-R owner/name]

Browse open GitHub issues and pull requests.

  -R, --repo owner/name
    	Repository. The default is the repository in this directory.
  -L, --limit number
    	Maximum number of open items to load. The default is 30.
`)
	}
	flag.Parse()

	if flag.NArg() > 0 {
		return fmt.Errorf("unexpected argument %q", flag.Arg(0))
	}

	if _, err := exec.LookPath("gh"); err != nil {
		return errors.New("gith needs the GitHub CLI. Install it, then run: gh auth login")
	}

	model := tui.New(github.CLI{Repo: repo, Limit: limit})
	program := tea.NewProgram(model)

	if _, err := program.Run(); err != nil {
		return err
	}

	return nil
}
