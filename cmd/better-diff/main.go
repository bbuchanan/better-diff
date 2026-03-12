package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"

	"better-diff/internal/app"
)

var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

func main() {
	showVersion := flag.Bool("version", false, "print version information")
	flag.BoolVar(showVersion, "v", false, "print version information")
	flag.Parse()

	if *showVersion {
		fmt.Printf("better-diff %s (%s, %s)\n", version, commit, date)
		return
	}

	cwd, err := resolveRepoPath(flag.Args())
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	program := tea.NewProgram(app.NewModel(cwd), tea.WithAltScreen())
	if _, err := program.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func resolveRepoPath(args []string) (string, error) {
	if len(args) == 0 {
		return os.Getwd()
	}
	if len(args) > 1 {
		return "", fmt.Errorf("usage: better-diff [path]")
	}
	return filepath.Abs(args[0])
}
