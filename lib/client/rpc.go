package client

import (
	"context"
	"fmt"

	"google.golang.org/grpc"

	taskerpb "github.com/wolves-fc/tasker/gen/proto/tasker"
)

// StartJob creates and starts a new job.
func (c *Client) StartJob(ctx context.Context, req *taskerpb.StartJobRequest) (*taskerpb.Job, error) {
	if req.Command == "" {
		return nil, fmt.Errorf("command is required")
	}

	resp, err := c.conn.Tasker.StartJob(ctx, req)
	if err != nil {
		return nil, err
	}

	return resp.Job, nil
}

// StopJob stops a running job.
func (c *Client) StopJob(ctx context.Context, id string) (*taskerpb.Job, error) {
	if id == "" {
		return nil, fmt.Errorf("job id is required")
	}

	resp, err := c.conn.Tasker.StopJob(ctx, &taskerpb.StopJobRequest{Id: id})
	if err != nil {
		return nil, err
	}

	return resp.Job, nil
}

// GetJob retrieves a job's current state.
func (c *Client) GetJob(ctx context.Context, id string) (*taskerpb.Job, error) {
	if id == "" {
		return nil, fmt.Errorf("job id is required")
	}

	resp, err := c.conn.Tasker.GetJob(ctx, &taskerpb.GetJobRequest{Id: id})
	if err != nil {
		return nil, err
	}

	return resp.Job, nil
}

// AttachJob opens a stream of the job's output.
func (c *Client) AttachJob(
	ctx context.Context,
	id string,
) (grpc.ServerStreamingClient[taskerpb.AttachJobResponse], error) {
	if id == "" {
		return nil, fmt.Errorf("job id is required")
	}

	return c.conn.Tasker.AttachJob(ctx, &taskerpb.AttachJobRequest{Id: id})
}
