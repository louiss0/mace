package main

import (
	"github.com/spf13/cobra"

	"github.com/louiss0/mace/internal/lsp"
)

func newLSPCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "lsp",
		Short: "Run the Mace language server over stdio",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, args []string) error {
			return lsp.New().RunStdio()
		},
	}
}
