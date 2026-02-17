package job

import (
	"context"
	"errors"
	"io"
	"os/exec"
	"sync"

	"github.com/google/uuid"
	"golang.org/x/sys/unix"
)

// Limits holds resource limits for a job.
type Limits struct {
	CPU    *float32
	Memory *uint32
	IO     *IOLimits
}

// IOLimits holds IO throttle limits for a block device.
type IOLimits struct {
	Device string
	Read   *uint32
	Write  *uint32
}

// Phase represents the lifecycle phase of a job.
type Phase int

const (
	PhaseUnknown Phase = iota
	PhaseRunning
	PhaseStopped
	PhaseCompleted
)

// Job represents a managed process in a cgroup.
type Job struct {
	done chan struct{}

	id      string
	command string
	args    []string
	owner   string
	limits  Limits
	cmd     *exec.Cmd
	output  *outputBuffer

	mu struct {
		sync.Mutex
		err   error
		phase Phase
	}
}

// New creates and starts a job in a cgroup.
//
// Call Stop to shut down the job.
func New(command string, args []string, owner string, limits Limits) (*Job, error) {
	j := &Job{
		done:    make(chan struct{}),
		id:      uuid.Must(uuid.NewV7()).String(),
		command: command,
		args:    args,
		owner:   owner,
		limits:  limits,
		output:  newOutputBuffer(),
	}

	cgFD, err := createCgroup(j.id, j.limits)
	if err != nil {
		return nil, err
	}

	j.cmd = exec.Command(j.command, j.args...)
	j.cmd.Stdout = j.output
	j.cmd.Stderr = j.output
	j.cmd.SysProcAttr = &unix.SysProcAttr{
		Setpgid:     true,
		UseCgroupFD: true,
		CgroupFD:    cgFD,
	}

	if err := j.cmd.Start(); err != nil {
		return nil, errors.Join(err, unix.Close(cgFD), removeCgroup(j.id))
	}

	// fd was only needed to place the process in the cgroup
	unix.Close(cgFD)

	j.mu.phase = PhaseRunning
	go j.wait()

	return j, nil
}

// wait blocks until the process exits and cleans up resources.
func (j *Job) wait() {
	defer close(j.done)

	waitErr := j.cmd.Wait()

	j.mu.Lock()
	switch j.mu.phase {
	case PhaseRunning:
		j.mu.phase = PhaseCompleted
	case PhaseStopped:
		// Exit error is expected when stopped
		waitErr = nil
	}

	// Kill any stragglers, remove the cgroup, then close the output buffer
	j.mu.err = errors.Join(waitErr, killCgroup(j.id), removeCgroup(j.id), j.output.Close())
	j.mu.Unlock()
}

// Stop sends a SIGTERM and cgroup kill to the job.
func (j *Job) Stop(ctx context.Context) error {
	j.mu.Lock()
	if j.mu.phase != PhaseRunning {
		j.mu.Unlock()
		return nil
	}

	j.mu.phase = PhaseStopped
	j.mu.Unlock()

	// SIGTERM the process group
	_ = unix.Kill(-j.cmd.Process.Pid, unix.SIGTERM)

	select {
	case <-j.done:
		return nil
	case <-ctx.Done():
		// Context ended so jump to a cgroup kill
		_ = killCgroup(j.id)
		// Wait for done to be signaled by wait()
		<-j.done
		return ctx.Err()
	}
}

// NewReader returns a reader for the job's output from the beginning.
func (j *Job) NewReader(ctx context.Context) io.Reader {
	return newOutputReader(ctx, j.output)
}

// ID returns the job's ID.
func (j *Job) ID() string { return j.id }

// Command returns the job's command.
func (j *Job) Command() string { return j.command }

// Args returns the job's command arguments.
func (j *Job) Args() []string { return j.args }

// Owner returns the job's owner.
func (j *Job) Owner() string { return j.owner }

// Limits returns the job's resource limits.
func (j *Job) Limits() Limits { return j.limits }

// Err returns the job's error after it has exited.
func (j *Job) Err() error {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.mu.err
}

// Phase returns the job's current lifecycle phase.
func (j *Job) Phase() Phase {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.mu.phase
}
