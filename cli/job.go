package cli

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	taskerpb "github.com/wolves-fc/tasker/gen/proto/tasker"
)

func (c *CLI) jobCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "job",
		Short: "Manage Tasker jobs",
	}

	cmd.PersistentFlags().StringVarP(&c.user, "user", "u", "", "User name")
	cmd.PersistentFlags().StringVarP(&c.addr, "addr", "a", "", "Server address (e.g. localhost:50051)")
	must(cmd.MarkPersistentFlagRequired("user"))
	must(cmd.MarkPersistentFlagRequired("addr"))
	cmd.AddCommand(c.startJobCmd())
	cmd.AddCommand(c.stopJobCmd())
	cmd.AddCommand(c.getJobCmd())
	cmd.AddCommand(c.attachJobCmd())

	return cmd
}

func (c *CLI) startJobCmd() *cobra.Command {
	var cpu float32
	var memory, read, write uint32
	var device string

	cmd := &cobra.Command{
		Use:   "start [flags] <command> [args...]",
		Short: "Start a new Tasker job",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			changed := cmd.Flags().Changed

			if (changed("read") || changed("write")) && !changed("device") {
				return fmt.Errorf("-d is required when -r or -w is set")
			}

			var limits *taskerpb.ResourceLimits
			if changed("cpu") || changed("memory") || changed("device") {
				limits = &taskerpb.ResourceLimits{}
				if changed("cpu") {
					limits.Cpu = &cpu
				}

				if changed("memory") {
					limits.Memory = &memory
				}

				if changed("device") {
					io := &taskerpb.IOLimits{Device: device}
					if changed("read") {
						io.Read = &read
					}

					if changed("write") {
						io.Write = &write
					}

					limits.Io = io
				}
			}

			j, err := c.clt.StartJob(cmd.Context(), &taskerpb.StartJobRequest{
				Command: args[0],
				Args:    args[1:],
				Limits:  limits,
			})
			if err != nil {
				return err
			}

			printJob(j)
			return nil
		},
	}

	cmd.Flags().Float32VarP(&cpu, "cpu", "c", 0, "CPU limit in cores (e.g. 0.5)")
	cmd.Flags().Uint32VarP(&memory, "memory", "m", 0, "Memory limit in MB")
	cmd.Flags().StringVarP(&device, "device", "d", "", "Block device for IO limits")
	cmd.Flags().Uint32VarP(&read, "read", "r", 0, "IO read limit in MB/s (requires -d)")
	cmd.Flags().Uint32VarP(&write, "write", "w", 0, "IO write limit in MB/s (requires -d)")

	c.withClient(cmd)
	return cmd
}

func (c *CLI) stopJobCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "stop <id>",
		Short: "Stop a Tasker job",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			j, err := c.clt.StopJob(cmd.Context(), args[0])
			if err != nil {
				return err
			}

			printJob(j)
			return nil
		},
	}

	c.withClient(cmd)
	return cmd
}

func (c *CLI) getJobCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get <id>",
		Short: "Get a Tasker job status",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			j, err := c.clt.GetJob(cmd.Context(), args[0])
			if err != nil {
				return err
			}

			printJob(j)
			return nil
		},
	}

	c.withClient(cmd)
	return cmd
}

func (c *CLI) attachJobCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "attach <id>",
		Short: "Attach to a Tasker job",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			stream, err := c.clt.AttachJob(cmd.Context(), args[0])
			if err != nil {
				return err
			}

			for {
				resp, err := stream.Recv()
				switch {
				case err == nil:
					os.Stdout.Write(resp.Data)
				case err == io.EOF, status.Code(err) == codes.Canceled:
					return nil
				default:
					return err
				}
			}
		},
	}

	c.withClient(cmd)
	return cmd
}

// printJob prints a job's info to stdout.
func printJob(j *taskerpb.Job) {
	var phase string
	switch j.Phase {
	case taskerpb.JobPhase_JOB_PHASE_RUNNING:
		phase = "running"
	case taskerpb.JobPhase_JOB_PHASE_STOPPED:
		phase = "stopped"
	case taskerpb.JobPhase_JOB_PHASE_COMPLETED:
		phase = "completed"
	default:
		phase = "unknown"
	}

	fmt.Printf("id: %s\nowner: %s\ncommand: %s\nargs: %v\nphase: %s\n", j.Id, j.Owner, j.Command, j.Args, phase)

	if j.Limits != nil {
		if j.Limits.Cpu != nil {
			fmt.Printf("cpu limit: %.2f cores\n", *j.Limits.Cpu)
		}

		if j.Limits.Memory != nil {
			fmt.Printf("memory limit: %d MB\n", *j.Limits.Memory)
		}

		if j.Limits.Io != nil {
			fmt.Printf("io device: %s\n", j.Limits.Io.Device)
			if j.Limits.Io.Read != nil {
				fmt.Printf("io read limit: %d MB/s\n", *j.Limits.Io.Read)
			}

			if j.Limits.Io.Write != nil {
				fmt.Printf("io write limit: %d MB/s\n", *j.Limits.Io.Write)
			}
		}
	}
}
