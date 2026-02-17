//go:build integration

package job

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"
)

func waitPhase(t *testing.T, j *Job, want Phase, timeout time.Duration) {
	t.Helper()

	// Periodic check of the job phase
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if j.Phase() == want {
			return
		}

		time.Sleep(10 * time.Millisecond)
	}

	t.Fatalf("phase (got=%d, want=%d) after %s", j.Phase(), want, timeout)
}

func cgroupExists(id string) bool {
	_, err := os.Stat(getCgroupDir(id))
	return err == nil
}

func TestJob_Lifecycle(t *testing.T) {
	j, err := New("echo", []string{"hello"}, "test", Limits{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	defer j.Stop(context.Background())

	waitPhase(t, j, PhaseCompleted, 2*time.Second)

	if cgroupExists(j.ID()) {
		t.Fatal("cgroup dir still exists after completion")
	}

	got, err := io.ReadAll(j.NewReader(context.Background()))
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}

	if strings.TrimSpace(string(got)) != "hello" {
		t.Fatalf("output (got=%q, want=%q)", string(got), "hello\n")
	}
}

func TestJob_Output(t *testing.T) {
	j, err := New("sh", []string{"-c", "echo one; echo two; echo three"}, "test", Limits{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	defer j.Stop(context.Background())

	waitPhase(t, j, PhaseCompleted, 2*time.Second)

	got, err := io.ReadAll(j.NewReader(context.Background()))
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}

	want := "one\ntwo\nthree\n"
	if string(got) != want {
		t.Fatalf("output (got=%q, want=%q)", string(got), want)
	}
}

func TestJob_Stop(t *testing.T) {
	j, err := New("sleep", []string{"60"}, "test", Limits{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	defer j.Stop(context.Background())

	if j.Phase() != PhaseRunning {
		t.Fatalf("phase after New (got=%d, want=%d)", j.Phase(), PhaseRunning)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := j.Stop(ctx); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	if j.Phase() != PhaseStopped {
		t.Fatalf("phase after Stop (got=%d, want=%d)", j.Phase(), PhaseStopped)
	}

	if cgroupExists(j.ID()) {
		t.Fatal("cgroup dir still exists after stop")
	}
}

func TestJob_Kill(t *testing.T) {
	// The shell sets up a trap to ignore SIGTERM so it will skip to force kill
	j, err := New("sh", []string{"-c", "trap '' TERM; echo ready; while true; do sleep 60; done"}, "test", Limits{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	defer j.Stop(context.Background())

	// Wait for the shell to set up the trap before sending SIGTERM
	buf := make([]byte, 16)
	j.NewReader(context.Background()).Read(buf)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err = j.Stop(ctx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Stop error (got=%v, want=context.DeadlineExceeded)", err)
	}

	if j.Phase() != PhaseStopped {
		t.Fatalf("phase after force kill (got=%d, want=%d)", j.Phase(), PhaseStopped)
	}

	if cgroupExists(j.ID()) {
		t.Fatal("cgroup dir still exists after force kill")
	}
}
