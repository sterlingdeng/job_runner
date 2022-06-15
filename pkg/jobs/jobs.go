package jobs

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"syscall"

	"github.com/google/uuid"
	"go.uber.org/multierr"

	"job_runner/pkg/bufferz"
	"job_runner/pkg/cgroupz"
)

type Status string

const (
	// StatusUnknown is set when the Job is first initialized or when an error occured
	StatusUnknown Status = "unknown"
	// StatusRunning is set when the job is running
	StatusRunning Status = "running"
	// StatusStopped is set when the job is stopped by a signal
	StatusStopped Status = "stopped"
	// StatusExited is set when the job exits. Exit code is available when this status is set.
	StatusExited Status = "exited"
)

const cgroupMount = "/lib_cgroup" // this is mounted when the VM starts

// Job is a wrapper around exec.Cmd and provides additional functionality
// such as resource limits via cgroups and support for streaming output
// to multiple readers
type Job struct {
	Err    string // Err is the string returned from std err
	Status Status

	cmd     *exec.Cmd
	command []string

	// ctx for cancellation
	ctx context.Context

	// resource limit
	id     string
	limits cgroupz.ResourceLimit

	// streaming
	getReaderFn func(context.Context) io.Reader
	writeCloser io.WriteCloser

	cleanup    []io.Closer
	goroutines []func() error
	errch      chan error

	stdout io.Reader
	stderr io.Reader
}

// New creates an un-executed Job.
func New(ctx context.Context, command []string, limits cgroupz.ResourceLimit) Job {
	multireader := bufferz.NewMultiReaderBuffer()
	return Job{
		id:          uuid.New().String(),
		Status:      StatusUnknown,
		command:     command,
		limits:      limits,
		getReaderFn: multireader.GetReader,
		writeCloser: multireader,
		ctx:         ctx,
		cleanup:     []io.Closer{multireader},
	}
}

// Start starts the job and does not block.
// After Start is called, Wait needs to be called to release resources and
// set fields for the Job.
// Start must only be called once.
func (j *Job) Start() error {
	if j.cmd != nil && j.cmd.Process != nil {
		return errors.New("process already executed")
	}
	if err := j.start(); err != nil {
		j.close()
		return err
	}
	return nil
}

func (j *Job) start() error {
	cgroup, err := cgroupz.New(j.id, cgroupMount, j.limits)
	if err != nil {
		return fmt.Errorf("cgroupz.New: %w", err)
	}
	j.cleanup = append(j.cleanup, cgroup)

	j.cmd = exec.CommandContext(
		j.ctx,
		"/home/vagrant/bin/utility/cmd", // hard coded path to utility,
		append([]string{cgroup.Path}, j.command...)...,
	)
	j.cmd.SysProcAttr = &syscall.SysProcAttr{
		Pdeathsig: syscall.SIGKILL,
	}
	j.Status = StatusRunning

	j.stdout, err = j.cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("j.cmd.StdoutPipe: %w", err)
	}
	j.stderr, err = j.cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("j.cmd.StderrPipe: %w", err)
	}

	if err := j.cmd.Start(); err != nil {
		return fmt.Errorf("j.cmd.Start: %w", err)
	}

	j.goroutines = []func() error{j.stdoutFn, j.stderrFn}
	j.errch = make(chan error, len(j.goroutines))
	for _, pipeSetup := range j.goroutines {
		// run each std[out,err] pipe copy in a goroutine
		// copy finishes when the process exits
		go func(fn func() error) {
			j.errch <- fn()
		}(pipeSetup)
	}

	return nil
}

// Wait blocks until the job completes and afterwards, will make available the
// Status, exit code, and any Errs from stderr that may have written.
func (j *Job) Wait() error {
	defer j.close()

	var errs error
	// we want to block here for copying to finish or else we leave data unread
	// for each goroutine we created to copy,
	// we wait for each to return
	for range j.goroutines {
		if ierr := <-j.errch; ierr != nil {
			errs = multierr.Append(errs, ierr)
		}
	}

	if err := j.cmd.Wait(); err != nil {
		if _, ok := err.(*exec.ExitError); !ok {
			j.Status = StatusUnknown
			return fmt.Errorf("j.cmd.Wait: %w", err)
		}
	}

	waitStatus := j.cmd.ProcessState.Sys().(syscall.WaitStatus)
	if waitStatus.Signaled() {
		j.Status = StatusStopped
	} else if waitStatus.Exited() {
		j.Status = StatusExited
	} else {
		j.Status = StatusUnknown
	}

	if errs != nil {
		return fmt.Errorf("error from goroutine: %+v", errs)
	}

	return nil
}

// Stream streams the output of the command to the provided writer. Stream supports concurrent streaming and is allowed
// to be called multiple times. Internally, the entirety of the command output is saved in an internal buffer.
// When Stream is called, data written starts from the beginning of the command output and writes until
// the reader gets to the end of the internal buffer and blocks until new writes are made or when the writer is closed.
// Stream blocks until the command closes.
func (j *Job) Stream(ctx context.Context, writer io.Writer) error {
	if _, err := io.Copy(writer, j.getReaderFn(ctx)); err != nil {
		return fmt.Errorf("io.Copy: %w", err)
	}
	return nil
}

// Result returns the programs exit code and status. This is valid only after Wait is called and the program finishes
// otherwise it will return -1 and StatusUnknown
func (j *Job) Result() (int, Status) {
	if j.cmd == nil || j.cmd.ProcessState == nil {
		return -1, StatusUnknown
	}
	return j.cmd.ProcessState.ExitCode(), j.Status
}

// convenience method to access Cmd for local testing
func (j *Job) Cmd() *exec.Cmd {
	return j.cmd
}

func (j *Job) stderrFn() error {
	var errBuf bytes.Buffer
	if _, err := io.Copy(&errBuf, j.stderr); err != nil {
		return fmt.Errorf("stderr.Copy: %w", err)
	}
	j.Err = errBuf.String()
	return nil
}

func (j *Job) stdoutFn() error {
	if _, err := io.Copy(j.writeCloser, j.stdout); err != nil {
		return fmt.Errorf("stdout.Copy: %w", err)
	}
	if err := j.writeCloser.Close(); err != nil {
		return fmt.Errorf("wc.Close: %w", err)
	}
	return nil
}

func (j *Job) close() error {
	var errs error
	for _, closer := range j.cleanup {
		if err := closer.Close(); err != nil {
			errs = multierr.Append(errs, err)
		}
	}
	return errs
}
