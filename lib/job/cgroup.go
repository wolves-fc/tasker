package job

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

const (
	// cgroupTaskerDir is the parent cgroup directory for all job cgroups.
	cgroupTaskerDir = "/sys/fs/cgroup/tasker"
	// cpuPeriod is the CPU period in microseconds.
	cpuPeriod = 100000
	// maxPIDs is the maximum number of concurrent processes per job.
	maxPIDs = 1000
)

// Init creates the tasker cgroup and enables controllers.
func Init() error {
	// Enable controllers at the root so they can be delegated to the tasker subtree.
	rootSubtreeControl := "/sys/fs/cgroup/cgroup.subtree_control"
	for _, controller := range []string{"cpu", "memory", "io", "pids"} {
		if err := writeCgroup(rootSubtreeControl, "+"+controller); err != nil {
			return fmt.Errorf("enable root controller (controller=%s): %w", controller, err)
		}
	}

	if err := os.MkdirAll(cgroupTaskerDir, 0o755); err != nil {
		return fmt.Errorf("create tasker cgroup: %w", err)
	}

	// Enable controllers in tasker subtree for job cgroups.
	subtreeControl := filepath.Join(cgroupTaskerDir, "cgroup.subtree_control")
	for _, controller := range []string{"cpu", "memory", "io", "pids"} {
		if err := writeCgroup(subtreeControl, "+"+controller); err != nil {
			return fmt.Errorf("enable controller (controller=%s): %w", controller, err)
		}
	}

	return nil
}

// createCgroup creates a cgroup for a job, applies resource limits, and returns an fd to the cgroup directory.
func createCgroup(id string, limits Limits) (fd int, err error) {
	dir := getCgroupDir(id)
	if err = os.Mkdir(dir, 0o755); err != nil {
		return -1, fmt.Errorf("create job cgroup: %w", err)
	}

	// defer removing the dir on error
	defer func() {
		if err != nil {
			err = errors.Join(err, os.Remove(dir))
		}
	}()

	if limits.CPU != nil {
		if err = writeCgroup(
			filepath.Join(dir, "cpu.max"),
			// quota = cores * period; max = quota period
			fmt.Sprintf("%d %d", int(*limits.CPU*cpuPeriod), cpuPeriod),
		); err != nil {
			return -1, fmt.Errorf("set cpu.max: %w", err)
		}
	}

	if limits.Memory != nil {
		if err = writeCgroup(
			filepath.Join(dir, "memory.max"),
			// max = MB -> bytes
			strconv.FormatUint(uint64(*limits.Memory)*1024*1024, 10),
		); err != nil {
			return -1, fmt.Errorf("set memory.max: %w", err)
		}
	}

	if limits.IO != nil {
		var device string
		device, err = lookupBlockDevice(limits.IO.Device)
		if err != nil {
			return -1, fmt.Errorf("lookup block device (device=%s): %w", limits.IO.Device, err)
		}

		rbps := "max"
		if limits.IO.Read != nil {
			// max = MB -> bytes
			rbps = strconv.FormatUint(uint64(*limits.IO.Read)*1024*1024, 10)
		}

		wbps := "max"
		if limits.IO.Write != nil {
			// max = MB -> bytes
			wbps = strconv.FormatUint(uint64(*limits.IO.Write)*1024*1024, 10)
		}

		if err = writeCgroup(
			filepath.Join(dir, "io.max"),
			// max = device rbps wbps
			fmt.Sprintf("%s rbps=%s wbps=%s", device, rbps, wbps),
		); err != nil {
			return -1, fmt.Errorf("set io.max: %w", err)
		}
	}

	if err = writeCgroup(filepath.Join(dir, "pids.max"), strconv.Itoa(maxPIDs)); err != nil {
		return -1, fmt.Errorf("set pids.max: %w", err)
	}

	fd, err = unix.Open(dir, unix.O_RDONLY|unix.O_DIRECTORY, 0)
	if err != nil {
		return -1, fmt.Errorf("open job cgroup: %w", err)
	}

	return fd, nil
}

// writeCgroup writes data to a cgroup file.
func writeCgroup(path, data string) error {
	return os.WriteFile(path, []byte(data), 0o644)
}

// killCgroup does a hard kill on all processes in the cgroup.
func killCgroup(id string) error {
	return writeCgroup(filepath.Join(getCgroupDir(id), "cgroup.kill"), "1")
}

// removeCgroup deletes a job's cgroup directory.
func removeCgroup(id string) error {
	return os.Remove(getCgroupDir(id))
}

// getCgroupDir returns a job's cgroup directory.
func getCgroupDir(id string) string {
	return filepath.Join(cgroupTaskerDir, id)
}

// lookupBlockDevice looks up a device's MAJ:MIN (deviceNum).
//
// If the device is a partition then the parent of the partition becomes the device since cgroup io.max requires whole
// disk devices.
func lookupBlockDevice(device string) (string, error) {
	var stat unix.Stat_t
	if err := unix.Stat(device, &stat); err != nil {
		return "", fmt.Errorf("stat (device=%s): %w", device, err)
	}

	if stat.Mode&unix.S_IFBLK == 0 {
		return "", fmt.Errorf("not a block device (device=%s)", device)
	}

	deviceNum := fmt.Sprintf("%d:%d", unix.Major(stat.Rdev), unix.Minor(stat.Rdev))
	sysPath := fmt.Sprintf("/sys/dev/block/%s", deviceNum)
	if _, err := os.Stat(filepath.Join(sysPath, "partition")); err != nil {
		return deviceNum, nil
	}

	// Use parent disk if the device is a partition
	parentDeviceNum, err := os.ReadFile(filepath.Join(sysPath, "..", "dev"))
	if err != nil {
		return "", fmt.Errorf("read parent disk device number: %w", err)
	}

	return strings.TrimSpace(string(parentDeviceNum)), nil
}
