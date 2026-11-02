package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"runtime/debug"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/jamesofficer/gith/internal/github"
	"github.com/jamesofficer/gith/internal/tui"
)

const usage = `Usage: gith [-R [HOST/]OWNER/NAME] [-L number]

Browse open GitHub issues and pull requests.

  -R, --repo [HOST/]OWNER/NAME
        Repository. The default is the repository in this directory.
  -L, --limit number
        Maximum number of open items to load. The default is %d.
  --version
        Show the version.
`

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "gith:", err)
		os.Exit(1)
	}
}

func run() error {
	var repo string
	var limit int
	var version bool

	flag.StringVar(&repo, "R", "", "")
	flag.StringVar(&repo, "repo", "", "")
	flag.IntVar(&limit, "L", github.DefaultLimit, "")
	flag.IntVar(&limit, "limit", github.DefaultLimit, "")
	flag.BoolVar(&version, "version", false, "")
	flag.Usage = func() { fmt.Fprintf(flag.CommandLine.Output(), usage, github.DefaultLimit) }
	flag.Parse()

	if version {
		fmt.Println("gith", buildVersion())

		return nil
	}

	if flag.NArg() > 0 {
		return fmt.Errorf("unexpected argument %q. Run gith -h for help", flag.Arg(0))
	}

	if _, err := exec.LookPath("gh"); err != nil {
		return errors.New("gith needs the GitHub CLI. Install it, then run: gh auth login")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	source, err := github.NewCLI(ctx, repo, limit)
	cancel()

	if err != nil {
		return err
	}

	_, err = tea.NewProgram(tui.New(source, source.Repo().FullName())).Run()

	return err
}

func buildVersion() string {
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" {
		return info.Main.Version
	}

	return "(unknown)"
}
