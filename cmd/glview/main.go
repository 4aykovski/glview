// Command glview is a terminal client for GitLab.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/4aykovski/glview/internal/config"
	"github.com/4aykovski/glview/internal/debuglog"
	"github.com/4aykovski/glview/internal/ui/common"
	"github.com/4aykovski/glview/internal/xdg"
)

// version is set at build time via -ldflags "-X main.version=...".
var version = "dev"

type options struct {
	project     string
	configPath  string
	debug       bool
	debugStyles bool
	version     bool
}

func parseFlags(args []string, stderr io.Writer) (options, error) {
	var o options
	fs := flag.NewFlagSet("glview", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.StringVar(&o.project, "p", "", "open project directly: group/project or project URL")
	fs.StringVar(&o.project, "project", "", "same as -p")
	fs.StringVar(&o.configPath, "config", "", "path to config file")
	fs.BoolVar(&o.debug, "debug", false, "write debug log to $XDG_CACHE_HOME/glview/debug.log")
	fs.BoolVar(&o.version, "version", false, "print version and exit")
	fs.BoolVar(&o.debugStyles, "debug-styles", false, "print a sample of all styles and exit")
	if err := fs.Parse(args); err != nil {
		return o, err
	}
	if fs.NArg() > 0 {
		return o, fmt.Errorf("unexpected arguments: %v", fs.Args())
	}
	return o, nil
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	opts, err := parseFlags(args, stderr)
	if err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		fmt.Fprintln(stderr, err)
		return 2
	}
	if opts.version {
		fmt.Fprintf(stdout, "glview %s\n", version)
		return 0
	}
	if opts.debugStyles {
		dark := lipgloss.HasDarkBackground(os.Stdin, os.Stdout)
		cfgPath := opts.configPath
		if cfgPath == "" {
			cfgPath = config.DefaultPath()
		}
		cfg, _, _ := config.Load(cfgPath) // the demo falls back to the default theme
		lipgloss.Fprintln(stdout, common.StyleDemo(common.NewStyles(common.ThemeByName(cfg.Theme, dark))))
		return 0
	}
	if opts.debug {
		closer, err := debuglog.Open(debugLogPath())
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		defer closer.Close()
		debuglog.L().Info("start", "version", version)
	}

	root, err := buildRoot(opts)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if _, err := tea.NewProgram(root).Run(); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}

func debugLogPath() string { return filepath.Join(xdg.CacheDir(), "debug.log") }
