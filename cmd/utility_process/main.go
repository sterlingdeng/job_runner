package main

import (
	"errors"
	"fmt"
	"job_runner/pkg/cgroupz"
	"os"
	"os/exec"
)

// this builds a program that runs as a parent process that eventually forks
// the target process. the parent process starts and places itself in the
// designated cgroup and then executes the target process
//
// the inputs are passed via os.Args
// the first arg is the cgroup path
// the rest of the args is the command to execute
// the stdout and stderr of the target process is piped back to this programs stdout and stderr
func main() {
	code, err := run(os.Args)
	if err != nil {
		os.Stderr.WriteString(err.Error())
	}
	os.Exit(code)
}

func run(args []string) (int, error) {
	if len(args) < 3 {
		return -1, errors.New("not enough arguments")
	}

	cgroupPath := os.Args[1]
	command := os.Args[2]
	var cmdargs []string
	if len(os.Args) >= 3 {
		cmdargs = os.Args[3:]
	}

	if err := cgroupz.AddProcess(cgroupPath, os.Getpid()); err != nil {
		return -1, fmt.Errorf("utility process: failed to add pid %d into cgroup at path %s", os.Getpid(), cgroupPath)
	}
	cmd := exec.Command(command, cmdargs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		if _, ok := err.(*exec.ExitError); !ok {
			return -1, err
		}
	}
	return cmd.ProcessState.ExitCode(), nil
}
