package cli

import (
	"github.com/spf13/cobra"

	"github.com/wolves-fc/tasker/lib/server"
)

func (c *CLI) serverCmd() *cobra.Command {
	var name, addr string

	cmd := &cobra.Command{
		Use:   "server",
		Short: "Start a Tasker server",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return server.New(cmd.Context(), c.certDir, name, addr)
		},
	}

	cmd.Flags().StringVarP(&name, "name", "n", "wolfpack1", "Server name (cert name)")
	cmd.Flags().StringVarP(&addr, "addr", "a", ":50051", "Listen address")

	return cmd
}
