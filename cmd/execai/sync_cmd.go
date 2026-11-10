package main

// execai sync — the same as /sync in the chat, from a shell: status, on,
// off, now, password, forget, keys export, keys import. The questions of
// the setup are asked in the terminal; a password is read without echo.

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/velesbsdllc/agent-vbai/internal/config"
	"github.com/velesbsdllc/agent-vbai/internal/i18n"
	agentsync "github.com/velesbsdllc/agent-vbai/internal/sync"
)

func newSyncCmd() *cobra.Command {
	// Help texts in the user's language; the global locale is put back.
	prev := i18n.Locale()
	setCLILocale()
	defer i18n.SetLocale(prev)

	withService := func(run func(ctx context.Context, svc *agentsync.Service, args []string) error) func(*cobra.Command, []string) error {
		return func(cmd *cobra.Command, args []string) error {
			setCLILocale()
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			svc, err := agentsync.New(cfg.APIBase)
			if err != nil {
				return err
			}
			svc.Cmd = "execai sync"
			return run(cmd.Context(), svc, args)
		}
	}
	show := func(text func(ctx context.Context, svc *agentsync.Service) string) func(*cobra.Command, []string) error {
		return withService(func(ctx context.Context, svc *agentsync.Service, _ []string) error {
			fmt.Println(text(ctx, svc))
			return nil
		})
	}
	converse := func(flow func(svc *agentsync.Service, args []string) agentsync.Flow) func(*cobra.Command, []string) error {
		return withService(func(ctx context.Context, svc *agentsync.Service, args []string) error {
			return runSyncFlow(ctx, flow(svc, args), os.Stdin, os.Stdout)
		})
	}
	status := show(func(_ context.Context, svc *agentsync.Service) string { return svc.Status() })

	root := &cobra.Command{
		Use:   "sync",
		Short: i18n.T("sync.cmd.short"),
		Long:  i18n.T("sync.cmd.long"),
		Args:  cobra.NoArgs,
		RunE:  status,
	}
	keys := &cobra.Command{Use: "keys", Short: i18n.T("sync.cmd.keys"), Args: cobra.NoArgs}
	keys.AddCommand(
		&cobra.Command{Use: "export", Short: i18n.T("sync.cmd.keysExport"), Args: cobra.NoArgs,
			RunE: show(func(_ context.Context, svc *agentsync.Service) string { return svc.KeysExport() })},
		&cobra.Command{Use: "import [code]", Short: i18n.T("sync.cmd.keysImport"), Args: cobra.MaximumNArgs(1),
			RunE: converse(func(svc *agentsync.Service, args []string) agentsync.Flow {
				return svc.KeysImportFlow(strings.Join(args, ""))
			})},
	)
	root.AddCommand(
		&cobra.Command{Use: "status", Short: i18n.T("sync.cmd.status"), Args: cobra.NoArgs, RunE: status},
		&cobra.Command{Use: "on", Short: i18n.T("sync.cmd.on"), Args: cobra.NoArgs,
			RunE: converse(func(svc *agentsync.Service, _ []string) agentsync.Flow { return svc.OnFlow() })},
		&cobra.Command{Use: "off", Short: i18n.T("sync.cmd.off"), Args: cobra.NoArgs,
			RunE: show(func(ctx context.Context, svc *agentsync.Service) string { return svc.Off(ctx) })},
		&cobra.Command{Use: "now", Short: i18n.T("sync.cmd.now"), Args: cobra.NoArgs,
			RunE: show(func(ctx context.Context, svc *agentsync.Service) string { return svc.SyncNow(ctx) })},
		&cobra.Command{Use: "password", Short: i18n.T("sync.cmd.password"), Args: cobra.NoArgs,
			RunE: converse(func(svc *agentsync.Service, _ []string) agentsync.Flow { return svc.PasswordFlow() })},
		&cobra.Command{Use: "forget", Short: i18n.T("sync.cmd.forget"), Args: cobra.NoArgs,
			RunE: converse(func(svc *agentsync.Service, _ []string) agentsync.Flow { return svc.ForgetFlow() })},
		keys,
	)
	return root
}

// runSyncFlow asks the questions of a flow in the terminal. A password is
// read without echo when stdin is a terminal; from a pipe it is one line,
// so scripts can drive it.
func runSyncFlow(ctx context.Context, f agentsync.Flow, in io.Reader, out io.Writer) error {
	r := bufio.NewReader(in)
	fd := -1
	if file, ok := in.(*os.File); ok && term.IsTerminal(int(file.Fd())) {
		fd = int(file.Fd())
	}
	step := f.Start(ctx)
	for {
		if t := strings.TrimSpace(step.Text); t != "" {
			fmt.Fprintln(out, t)
		}
		if step.Done() {
			return nil
		}
		fmt.Fprintln(out, step.Prompt)
		fmt.Fprint(out, "> ")
		var answer string
		var err error
		if step.Secret && fd >= 0 {
			var b []byte
			b, err = term.ReadPassword(fd)
			fmt.Fprintln(out)
			answer = string(b)
		} else {
			answer, err = r.ReadString('\n')
			if errors.Is(err, io.EOF) && answer != "" {
				err = nil
			}
		}
		if err != nil {
			fmt.Fprintln(out)
			fmt.Fprintln(out, i18n.T("sync.cmd.needTerminal"))
			return nil
		}
		step = f.Answer(ctx, strings.TrimSpace(answer))
	}
}

// syncInBackground — sync for a long-running command (serve, ide): a pull
// at start, then in the background; returns what to call on exit.
func syncInBackground(ctx context.Context, cfg *config.Config, notify func(string)) func() {
	i18n.Setup(cfg.Locale)
	return agentsync.Start(ctx, cfg.APIBase, agentsync.Hooks{Notify: notify})
}
