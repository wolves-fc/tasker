package cli

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/wolves-fc/tasker/lib/client"
)

// CLI holds the connection and root command.
type CLI struct {
	clt     *client.Client
	root    *cobra.Command
	certDir string
	user    string
	addr    string
}

// Start creates a CLI with all commands registered and executes it.
func Start(ctx context.Context) error {
	c := &CLI{
		root: &cobra.Command{
			Use:   "taskerctl",
			Short: "taskerctl is the CLI for Tasker",
			PersistentPreRun: func(cmd *cobra.Command, args []string) {
				cmd.SilenceUsage = true
			},
			SilenceErrors: true,
		},
	}

	// certs-dir is relative to the working directory
	c.root.PersistentFlags().StringVarP(&c.certDir, "certs-dir", "C", "certs", "Certificates directory")
	c.root.AddCommand(c.certCmd())
	c.root.AddCommand(c.jobCmd())
	c.root.AddCommand(c.serverCmd())
	c.root.CompletionOptions.DisableDefaultCmd = true

	return c.root.ExecuteContext(ctx)
}

// withClient wraps RunE to create a Tasker client before the command runs and closes it after.
func (c *CLI) withClient(cmd *cobra.Command) {
	runE := cmd.RunE
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		clt, err := client.New(c.certDir, c.user, c.addr)
		if err != nil {
			return err
		}

		c.clt = clt

		defer func() {
			c.clt.Close()
			c.clt = nil
		}()

		return runE(cmd, args)
	}
}

// must panics of errors when building up a cobra cli
func must(err error) {
	if err != nil {
		panic(err)
	}
}
