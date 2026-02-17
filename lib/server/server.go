package server

import (
	"context"
	"fmt"
	"sync"
	"time"

	taskerpb "github.com/wolves-fc/tasker/gen/proto/tasker"
	"github.com/wolves-fc/tasker/lib/job"
	"github.com/wolves-fc/tasker/lib/rpc"
)

// Compile time verification that Server implements taskerpb.TaskerServiceServer.
var _ taskerpb.TaskerServiceServer = (*Server)(nil)

// Server manages jobs on a single machine.
type Server struct {
	taskerpb.UnimplementedTaskerServiceServer

	mu struct {
		sync.RWMutex
		jobs map[string]*job.Job
	}
}

// New initializes cgroups, serves gRPC requests, and owns the lifecycle of all jobs.
func New(ctx context.Context, certDir, name, addr string) error {
	s := &Server{}
	s.mu.jobs = make(map[string]*job.Job)

	if err := job.Init(); err != nil {
		return fmt.Errorf("init cgroup: %w", err)
	}

	if err := rpc.Serve(ctx, s, certDir, name, addr); err != nil {
		return err
	}

	// Give each job 2 seconds to gracefully stop
	stopCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	s.mu.RLock()
	var wg sync.WaitGroup
	for _, j := range s.mu.jobs {
		wg.Go(func() {
			if j.Phase() != job.PhaseRunning {
				return
			}

			if err := j.Stop(stopCtx); err != nil {
				fmt.Printf("job force killed (id=%s): %v\n", j.ID(), err)
			} else {
				fmt.Printf("job stopped (id=%s)\n", j.ID())
			}
		})
	}
	s.mu.RUnlock()

	wg.Wait()

	fmt.Println("server stopped")

	return nil
}
