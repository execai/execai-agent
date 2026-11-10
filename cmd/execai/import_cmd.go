package main

// execai import — bring other agents' instructions into execai's memory from
// a shell or CI, without opening the chat. Same discovery and plan as
// /memory import; --dry-run prints the plan and writes nothing.

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/velesbsdllc/agent-vbai/internal/agent"
	"github.com/velesbsdllc/agent-vbai/internal/i18n"
)

type importOptions struct {
	dir    string
	from   string
	dryRun bool
	yes    bool
}

func newImportCmd() *cobra.Command {
	// Help texts are built now, in the user's language; the global locale is
	// put back so other commands behave exactly as before.
	prev := i18n.Locale()
	setCLILocale()
	defer i18n.SetLocale(prev)

	var opts importOptions
	cmd := &cobra.Command{
		Use:   "import",
		Short: i18n.T("import.cmd.short"),
		Long:  i18n.T("import.cmd.long"),
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			setCLILocale()
			if opts.dir == "" {
				wd, err := os.Getwd()
				if err != nil {
					return err
				}
				opts.dir = wd
			}
			// Questions only when a person can answer them: in CI stdin is
			// not a terminal, and the explicit command is the consent.
			interactive := term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd()))
			return runImport(opts, os.Stdin, cmd.OutOrStdout(), interactive)
		},
	}
	cmd.Flags().BoolVar(&opts.dryRun, "dry-run", false, i18n.T("import.cmd.flagDryRun"))
	cmd.Flags().StringVar(&opts.from, "from", "all", i18n.T("import.cmd.flagFrom"))
	cmd.Flags().BoolVarP(&opts.yes, "yes", "y", false, i18n.T("import.cmd.flagYes"))
	return cmd
}

// setCLILocale picks the language like the chat does — config, then $LANG —
// but without config.Load, which would create files (and --dry-run writes
// nothing).
func setCLILocale() {
	loc := ""
	if base, err := os.UserConfigDir(); err == nil {
		if data, err := os.ReadFile(filepath.Join(base, "execai", "config.json")); err == nil {
			var c struct {
				Locale string `json:"locale"`
			}
			if json.Unmarshal(data, &c) == nil {
				loc = c.Locale
			}
		}
	}
	if loc == "" {
		loc = i18n.Detect()
	}
	if loc != "" {
		i18n.SetLocale(loc)
	}
}

// runImport is the command without cobra and the terminal, for tests.
func runImport(opts importOptions, in io.Reader, out io.Writer, interactive bool) error {
	agents, err := agent.ParseImportFrom(opts.from)
	if err != nil {
		return err
	}
	srcs := agent.FilterSourcesByAgent(agent.DiscoverAllMemorySources(opts.dir), agents)
	if len(srcs) == 0 {
		fmt.Fprintln(out, i18n.Tf("import.cmd.nothing", opts.dir))
		return nil
	}
	plan, err := agent.PlanImport(srcs)
	if err != nil {
		return err
	}
	memDir, _ := agent.MemoryDirPath()
	fmt.Fprintln(out, i18n.Tf("import.cmd.found", len(srcs), memDir))
	fmt.Fprint(out, agent.DescribeImportPlan(plan))
	fmt.Fprintln(out, i18n.T("import.cmd.notTransferred"))

	if opts.dryRun {
		fmt.Fprintln(out, i18n.T("import.cmd.dryRunDone"))
		return nil
	}
	todo := agent.Importable(plan)
	if len(todo) == 0 {
		fmt.Fprintln(out, i18n.T("import.cmd.nothingNew"))
		return nil
	}
	if interactive && !opts.yes {
		fmt.Fprint(out, i18n.Tf("import.cmd.confirm", len(todo)))
		answer, _ := bufio.NewReader(in).ReadString('\n')
		if !isYes(answer) {
			fmt.Fprintln(out, i18n.T("import.cmd.cancelled"))
			return nil
		}
	}

	res, err := agent.ImportSources(todo)
	if err != nil {
		return err
	}
	fmt.Fprintln(out, i18n.Tf("import.cmd.done", len(res.Imported)))
	for _, n := range res.Imported {
		fmt.Fprintln(out, "  + "+n)
	}
	for _, s := range res.Skipped {
		fmt.Fprintln(out, "  ~ "+s)
	}
	return nil
}

// isYes reads a [Y/n] answer: Enter means yes.
func isYes(answer string) bool {
	switch strings.ToLower(strings.TrimSpace(answer)) {
	case "", "y", "yes", "д", "да":
		return true
	}
	return false
}
