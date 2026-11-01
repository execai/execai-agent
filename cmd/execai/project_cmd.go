package main

// execai project publish [name] — the same as /project publish in the chat:
// a web chat project made from the current directory, bound to it, its
// .execai/ sent at once when sync is on.

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/velesbsdllc/agent-vbai/internal/chat"
	"github.com/velesbsdllc/agent-vbai/internal/config"
	"github.com/velesbsdllc/agent-vbai/internal/i18n"
)

func newProjectCmd() *cobra.Command {
	prev := i18n.Locale()
	setCLILocale()
	defer i18n.SetLocale(prev)

	root := &cobra.Command{Use: "project", Short: i18n.T("project.cmd.short"), Args: cobra.NoArgs}
	root.AddCommand(&cobra.Command{
		Use:   "publish [name]",
		Short: i18n.T("project.cmd.publish"),
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			setCLILocale()
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			dir, err := os.Getwd()
			if err != nil {
				return err
			}
			fmt.Println(chat.PublishProject(cfg, strings.Join(args, " "), dir))
			return nil
		},
	})
	return root
}
