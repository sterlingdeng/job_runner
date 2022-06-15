package cgroupz

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

type ResourceLimit struct {
	CpuWeight int
	MaxMem    int
	MaxIO     *IOLimit
}

type IOLimit struct {
	MaxIO int
	Maj   int
	Min   int
}

type CgroupController struct {
	Path string
}

// AddProcess adds a process with pid to the cgroup managed by the controller
func (c *CgroupController) AddProcess(pid int) error {
	return AddProcess(c.Path, pid)
}

// Close deletes the cgroup hierarchy managed by the contorller
func (c *CgroupController) Close() error {
	return cleanUp(c.Path)
}

func cleanUp(path string) error {
	var err error
	duration := 10 * time.Millisecond
	for i := 0; i < 4; i++ {
		err = os.RemoveAll(path)
		if err == nil {
			return nil
		}
		time.Sleep(duration)
		duration *= 2
	}
	return fmt.Errorf("os.RemoveAll: %w", err)
}

// New initializes a v2 cgroup at the mount point with the provided name and limits.
// After this successfully returns, it is the caller's responsibility to call Close to
// clean up the existing resources.
func New(name string, mountPoint string, limits ResourceLimit) (*CgroupController, error) {
	// ensure cpu, mem, and io is available
	err := os.WriteFile(
		filepath.Join(mountPoint, "cgroup.subtree_control"),
		[]byte("+cpu +memory +io"),
		0644,
	)
	if err != nil {
		return nil, fmt.Errorf("cgroup.subtree_control write file: %w", err)
	}

	path := filepath.Join(mountPoint, name)
	if err = os.MkdirAll(path, 0755); err != nil {
		return nil, fmt.Errorf("os.MkdirAll: %w", err)
	}

	defer func() {
		if err != nil {
			cleanErr := cleanUp(path)
			if cleanErr != nil {
				fmt.Printf("failed to clean up cgroup: %v\n", cleanErr)
			}
		}
	}()

	if limits.CpuWeight != 0 {
		err = os.WriteFile(
			filepath.Join(path, "cpu.weight"),
			[]byte(strconv.Itoa(limits.CpuWeight)),
			0644,
		)
		if err != nil {
			return nil, fmt.Errorf("write cpu weight: %w", err)
		}
	}
	if limits.MaxMem != 0 {
		err = os.WriteFile(
			filepath.Join(path, "memory.max"),
			[]byte(strconv.Itoa(limits.MaxMem)),
			0644,
		)
		if err != nil {
			return nil, fmt.Errorf("write memory max: %w", err)
		}
	}
	if limits.MaxIO != nil {
		err = os.WriteFile(
			filepath.Join(path, "io.max"),
			[]byte(fmt.Sprintf("%d:%d %s=%d", limits.MaxIO.Maj, limits.MaxIO.Min, "wiops", limits.MaxIO.MaxIO)),
			0644,
		)
		if err != nil {
			return nil, fmt.Errorf("write io limit: %w", err)
		}
	}

	ctrl := &CgroupController{
		Path: path,
	}
	return ctrl, nil
}

func AddProcess(path string, pid int) error {
	err := os.WriteFile(
		filepath.Join(path, "cgroup.procs"),
		[]byte(strconv.Itoa(pid)),
		0644,
	)
	if err != nil {
		return fmt.Errorf("open cgroup.procs: %w", err)
	}
	return nil
}
