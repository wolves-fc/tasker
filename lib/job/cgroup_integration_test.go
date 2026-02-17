//go:build integration

package job

import (
	"context"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

func TestMain(m *testing.M) {
	if err := Init(); err != nil {
		panic(err)
	}

	os.Exit(m.Run())
}

func readCgroupFile(t *testing.T, id, file string) string {
	t.Helper()

	data, err := os.ReadFile(getCgroupDir(id) + "/" + file)
	if err != nil {
		t.Fatalf("read cgroup file (%s): %v", file, err)
	}

	return strings.TrimSpace(string(data))
}

// findBlockDevice attempts to find a block device that can handle io limits.
func findBlockDevice(t *testing.T) string {
	t.Helper()

	entries, err := os.ReadDir("/sys/block")
	if err != nil {
		t.Fatalf("read /sys/block: %v", err)
	}

	// Skip virtual and non disk devices that do not support IO limits (I think this should cover most cases)
	skip := []string{"dm-", "fd", "loop", "nbd", "ram", "sr", "zram"}
	for _, entry := range entries {
		name := entry.Name()
		if slices.ContainsFunc(skip, func(prefix string) bool { return strings.HasPrefix(name, prefix) }) {
			continue
		}

		device := "/dev/" + name
		if _, err := os.Stat(device); err == nil {
			return device
		}
	}

	t.Skip("could not find a block device")
	return ""
}

func TestCgroup_CPULimit(t *testing.T) {
	cpu := float32(0.5)
	// Sleep for 60 seconds to allow time to check the cgroup settings.
	j, err := New("sleep", []string{"60"}, "test", Limits{CPU: &cpu})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	defer j.Stop(context.Background())

	got := readCgroupFile(t, j.ID(), "cpu.max")
	// quota = period * cpu; max = quota period
	want := "50000 100000"
	if got != want {
		t.Fatalf("cpu.max (got=%q, want=%q)", got, want)
	}
}

func TestCgroup_MemoryLimit(t *testing.T) {
	memory := uint32(512)
	// Sleep for 60 seconds to allow time to check the cgroup settings.
	j, err := New("sleep", []string{"60"}, "test", Limits{Memory: &memory})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	defer j.Stop(context.Background())

	got := readCgroupFile(t, j.ID(), "memory.max")
	// max = memory * 1024 * 1024
	want := "536870912"
	if got != want {
		t.Fatalf("memory.max (got=%q, want=%q)", got, want)
	}
}

func TestCgroup_IOLimit(t *testing.T) {
	device := findBlockDevice(t)
	read := uint32(100)
	write := uint32(50)
	// Sleep for 60 seconds to allow time to check the cgroup settings.
	j, err := New("sleep", []string{"60"}, "test", Limits{
		IO: &IOLimits{
			Device: device,
			Read:   &read,
			Write:  &write,
		},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	defer j.Stop(context.Background())

	deviceNum, err := lookupBlockDevice(device)
	if err != nil {
		t.Fatalf("lookupBlockDevice: %v", err)
	}

	got := readCgroupFile(t, j.ID(), "io.max")
	// rbps = read * 1024 * 1024; wbps = write * 1024 * 1024; max = deviceNum rbps wbps
	wantRbps := fmt.Sprintf("rbps=%d", 100*1024*1024)
	wantWbps := fmt.Sprintf("wbps=%d", 50*1024*1024)
	if !strings.Contains(got, deviceNum) || !strings.Contains(got, wantRbps) || !strings.Contains(got, wantWbps) {
		t.Fatalf("io.max (got=%q, want deviceNum=%s rbps=%s wbps=%s)", got, deviceNum, wantRbps, wantWbps)
	}
}

func TestCgroup_Remove(t *testing.T) {
	id := "test-remove"
	fd, err := createCgroup(id, Limits{})
	if err != nil {
		t.Fatalf("createCgroup: %v", err)
	}

	unix.Close(fd)
	defer removeCgroup(id)

	dir := getCgroupDir(id)
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("cgroup dir should exist: %v", err)
	}

	if err := removeCgroup(id); err != nil {
		t.Fatalf("removeCgroup: %v", err)
	}

	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("cgroup dir still exists after removeCgroup")
	}
}
