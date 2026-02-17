package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/wolves-fc/tasker/lib/tls"
)

func (c *CLI) certCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cert",
		Short: "Generate Tasker TLS certificates",
	}

	cmd.AddCommand(c.caCertCmd())
	cmd.AddCommand(c.clientCertCmd())
	cmd.AddCommand(c.serverCertCmd())

	return cmd
}

func (c *CLI) caCertCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ca",
		Short: "Generate a Tasker CA",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return tls.GenerateCA(c.certDir)
		},
	}
}

func (c *CLI) clientCertCmd() *cobra.Command {
	var user, role string

	cmd := &cobra.Command{
		Use:   "client",
		Short: "Generate a Tasker client certificate",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if role != "admin" && role != "user" {
				return fmt.Errorf("-r must be 'admin' or 'user'")
			}

			return tls.GenerateClient(c.certDir, user, tls.Role(role))
		},
	}

	cmd.Flags().StringVarP(&user, "user", "u", "", "User name")
	cmd.Flags().StringVarP(&role, "role", "r", "", "User role (admin or user)")
	must(cmd.MarkFlagRequired("user"))
	must(cmd.MarkFlagRequired("role"))

	return cmd
}

func (c *CLI) serverCertCmd() *cobra.Command {
	var name, hosts string

	cmd := &cobra.Command{
		Use:   "server",
		Short: "Generate a Tasker server certificate",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			var hostList []string
			for host := range strings.SplitSeq(hosts, ",") {
				host = strings.TrimSpace(host)
				if host != "" {
					hostList = append(hostList, host)
				}
			}

			return tls.GenerateServer(c.certDir, name, hostList)
		},
	}

	cmd.Flags().StringVarP(&name, "name", "n", "", "Server name")
	cmd.Flags().StringVarP(&hosts, "hosts", "H", "", "Comma separated hostnames or IPs")
	must(cmd.MarkFlagRequired("name"))
	must(cmd.MarkFlagRequired("hosts"))

	return cmd
}
