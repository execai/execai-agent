package main

// execai ui — the editor's chat panel in a browser tab on 127.0.0.1: the
// ide server runs in this process, internal/uiserve carries its protocol
// over HTTP (SSE out, POST in). Tasks from the site and the plan gate are the
// ide server's own, so this window takes the inbox like an editor window.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/velesbsdllc/agent-vbai/internal/auth"
	"github.com/velesbsdllc/agent-vbai/internal/config"
	"github.com/velesbsdllc/agent-vbai/internal/i18n"
	"github.com/velesbsdllc/agent-vbai/internal/ide"
	"github.com/velesbsdllc/agent-vbai/internal/uiserve"
	agentver "github.com/velesbsdllc/agent-vbai/internal/version"
)

func newUICmd() *cobra.Command {
	// Help texts in the user's language; the global locale is put back.
	prev := i18n.Locale()
	setCLILocale()
	defer i18n.SetLocale(prev)

	var cwd string
	var port int
	var noOpen bool
	cmd := &cobra.Command{
		Use:   "ui",
		Short: i18n.T("ui.cmd.short"),
		Long:  i18n.T("ui.cmd.long"),
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			i18n.Setup(cfg.Locale)
			dir, err := uiDir(cwd)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			defer syncInBackground(cmd.Context(), cfg, func(s string) { fmt.Fprintln(out, s) })()
			opts := uiserve.Options{Port: port, Cwd: dir, Version: agentver.Get(), Locale: cfg.Locale}
			run := func(ctx context.Context, in io.Reader, w io.Writer) error {
				return ide.Run(ctx, cfg, ide.Options{Cwd: dir, In: in, Out: w})
			}
			err = uiserve.Run(cmd.Context(), opts, run, func(url string) {
				fmt.Fprintln(out, i18n.Tf("ui.address", url))
				fmt.Fprintln(out, i18n.T("ui.hint"))
				if !noOpen {
					if err := auth.OpenBrowser(url); err != nil {
						fmt.Fprintln(out, i18n.Tf("ui.openFailed", err))
					}
				}
			})
			fmt.Fprintln(out, i18n.T("ui.stopped"))
			return err
		},
	}
	cmd.Flags().StringVar(&cwd, "cwd", "", i18n.T("ui.flag.cwd"))
	cmd.Flags().IntVar(&port, "port", 0, i18n.T("ui.flag.port"))
	cmd.Flags().BoolVar(&noOpen, "no-open", false, i18n.T("ui.flag.noOpen"))
	return cmd
}

// uiDir — the project directory as an absolute path: the page shows it and
// uploads are checked against it.
func uiDir(cwd string) (string, error) {
	if cwd == "" {
		wd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		cwd = wd
	}
	abs, err := filepath.Abs(cwd)
	if err != nil {
		return "", err
	}
	if st, err := os.Stat(abs); err != nil || !st.IsDir() {
		return "", errors.New(i18n.Tf("ui.badCwd", abs))
	}
	return abs, nil
}
