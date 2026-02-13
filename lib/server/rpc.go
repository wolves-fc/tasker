package server

import (
	"context"
	"fmt"
	"io"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	taskerpb "github.com/wolves-fc/tasker/gen/proto/tasker"
	"github.com/wolves-fc/tasker/lib/job"
	"github.com/wolves-fc/tasker/lib/rpc"
	"github.com/wolves-fc/tasker/lib/tls"
)

func (s *Server) StartJob(ctx context.Context, req *taskerpb.StartJobRequest) (*taskerpb.StartJobResponse, error) {
	if req.Command == "" {
		return nil, status.Errorf(codes.InvalidArgument, "command is required")
	}

	identity, err := rpc.IdentityFromContext(ctx)
	if err != nil {
		return nil, err
	}

	limits := job.Limits{}

	if req.Limits != nil {
		limits.CPU = req.Limits.Cpu
		limits.Memory = req.Limits.Memory

		if req.Limits.Io != nil {
			if req.Limits.Io.Device == "" {
				return nil, status.Error(codes.InvalidArgument, "device is required when IO limits are set")
			}

			limits.IO = &job.IOLimits{
				Device: req.Limits.Io.Device,
				Read:   req.Limits.Io.Read,
				Write:  req.Limits.Io.Write,
			}
		}
	}

	j, err := job.New(req.Command, req.Args, identity.Name, limits)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "start failed: %v", err)
	}

	s.mu.Lock()
	s.mu.jobs[j.ID()] = j
	s.mu.Unlock()

	fmt.Printf("job started (id=%s, owner=%s, command=%s)\n", j.ID(), j.Owner(), j.Command())

	return &taskerpb.StartJobResponse{Job: convertJob(j)}, nil
}

func (s *Server) StopJob(ctx context.Context, req *taskerpb.StopJobRequest) (*taskerpb.StopJobResponse, error) {
	identity, err := rpc.IdentityFromContext(ctx)
	if err != nil {
		return nil, err
	}

	s.mu.RLock()
	j, exists := s.mu.jobs[req.Id]
	s.mu.RUnlock()

	if !exists {
		return nil, status.Errorf(codes.NotFound, "job not found (id=%s)", req.Id)
	}

	if err := checkJobAccess(identity, j.Owner()); err != nil {
		return nil, err
	}

	// Give the job 2 seconds to gracefully stop
	stopCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := j.Stop(stopCtx); err != nil {
		fmt.Printf("job force killed (id=%s, owner=%s): %v\n", j.ID(), identity.Name, err)
	} else {
		fmt.Printf("job stopped (id=%s, owner=%s)\n", j.ID(), identity.Name)
	}

	return &taskerpb.StopJobResponse{Job: convertJob(j)}, nil
}

func (s *Server) GetJob(ctx context.Context, req *taskerpb.GetJobRequest) (*taskerpb.GetJobResponse, error) {
	identity, err := rpc.IdentityFromContext(ctx)
	if err != nil {
		return nil, err
	}

	s.mu.RLock()
	j, exists := s.mu.jobs[req.Id]
	s.mu.RUnlock()

	if !exists {
		return nil, status.Errorf(codes.NotFound, "job not found (id=%s)", req.Id)
	}

	if err := checkJobAccess(identity, j.Owner()); err != nil {
		return nil, err
	}

	return &taskerpb.GetJobResponse{Job: convertJob(j)}, nil
}

func (s *Server) AttachJob(req *taskerpb.AttachJobRequest, stream grpc.ServerStreamingServer[taskerpb.AttachJobResponse]) error {
	identity, err := rpc.IdentityFromContext(stream.Context())
	if err != nil {
		return err
	}

	s.mu.RLock()
	j, exists := s.mu.jobs[req.Id]
	s.mu.RUnlock()

	if !exists {
		return status.Errorf(codes.NotFound, "job not found (id=%s)", req.Id)
	}

	if err := checkJobAccess(identity, j.Owner()); err != nil {
		return err
	}

	// Read in 4KB chunks
	r := j.NewReader(stream.Context())
	buf := make([]byte, 4096)

	for {
		count, err := r.Read(buf)
		if count > 0 {
			if sendErr := stream.Send(&taskerpb.AttachJobResponse{Data: buf[:count]}); sendErr != nil {
				return sendErr
			}
		}

		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
	}
}

// checkJobAccess verifies the identity can manage the given job.
//
// Admins can manage any job; users can only manage their own.
func checkJobAccess(id rpc.Identity, owner string) error {
	if id.Role == tls.RoleAdmin {
		return nil
	}

	if owner != id.Name {
		return status.Errorf(codes.PermissionDenied, "user %s cannot manage job owned by %s", id.Name, owner)
	}
	return nil
}

// convertJob builds a proto Job from a job.Job.
func convertJob(j *job.Job) *taskerpb.Job {
	var phase taskerpb.JobPhase

	switch j.Phase() {
	case job.PhaseRunning:
		phase = taskerpb.JobPhase_JOB_PHASE_RUNNING
	case job.PhaseStopped:
		phase = taskerpb.JobPhase_JOB_PHASE_STOPPED
	case job.PhaseCompleted:
		phase = taskerpb.JobPhase_JOB_PHASE_COMPLETED
	}

	limits := j.Limits()

	jobpb := &taskerpb.Job{
		Id:      j.ID(),
		Owner:   j.Owner(),
		Command: j.Command(),
		Args:    j.Args(),
		Phase:   phase,
	}

	if limits.CPU != nil || limits.Memory != nil || limits.IO != nil {
		jobpb.Limits = &taskerpb.ResourceLimits{
			Cpu:    limits.CPU,
			Memory: limits.Memory,
		}

		if limits.IO != nil {
			jobpb.Limits.Io = &taskerpb.IOLimits{
				Device: limits.IO.Device,
				Read:   limits.IO.Read,
				Write:  limits.IO.Write,
			}
		}
	}

	return jobpb
}
