package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/velesbsdllc/agent-vbai/internal/config"
	"github.com/velesbsdllc/agent-vbai/internal/i18n"
	"github.com/velesbsdllc/agent-vbai/internal/plan"
)

// newPlanCmd — `execai plan`: the same report as /plan in the chat.
func newPlanCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "plan",
		Short: "ExecAI plan: state and what it allows",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			i18n.Setup(cfg.Locale)
			fmt.Println(plan.New(cfg.APIBase).Report(cmd.Context()))
			return nil
		},
	}
}
