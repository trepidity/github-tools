// Command gh-triage is a gh extension for working through GitHub issues quickly.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/trepidity/gh-triage/internal/config"
	"github.com/trepidity/gh-triage/internal/github"
	"github.com/trepidity/gh-triage/internal/ui"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "gh triage:", err)
		os.Exit(1)
	}
}

func run() error {
	query := flag.String("query", "", "GitHub issue search query, e.g. \"org:foo assignee:@me\"")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: gh triage [owner/repo | --query \"<search>\"]")
		flag.PrintDefaults()
	}
	flag.Parse()

	q := *query
	switch {
	case flag.NArg() > 1 || (flag.NArg() == 1 && q != ""):
		flag.Usage()
		return errors.New("give either owner/repo or --query, not both")
	case flag.NArg() == 1:
		repo, err := github.ParseRepo(flag.Arg(0))
		if err != nil {
			return err
		}
		q = ui.RepoQuery(repo)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	cfgPath := filepath.Join(home, ".config", "gh-triage", "config.yml")
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return err
	}
	var pins []github.Repo
	for _, p := range cfg.Pins {
		r, err := github.ParseRepo(p)
		if err != nil {
			return fmt.Errorf("%s: pin: %w", cfgPath, err)
		}
		pins = append(pins, r)
	}

	rest, err := api.DefaultRESTClient()
	if err != nil {
		return fmt.Errorf("not authenticated with GitHub (run `gh auth login`): %w", err)
	}

	style := "light"
	if lipgloss.HasDarkBackground() {
		style = "dark"
	}
	model := ui.New(github.NewREST(rest), ui.Options{
		Query:     q,
		Pins:      pins,
		RepoCache: filepath.Join(home, ".cache", "gh-triage", "repos.json"),
		Style:     style,
	})
	_, err = tea.NewProgram(model, tea.WithAltScreen()).Run()
	return err
}
