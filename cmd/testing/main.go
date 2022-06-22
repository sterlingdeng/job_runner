package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"

	"job_runner/pkg/cgroupz"
	"job_runner/pkg/jobs"
)

func main() {
	if len(os.Args) < 4 {
		log.Fatal("not enough args")
	}
	if err := run(os.Args); err != nil {
		log.Fatalln(err)
	}
}

// args: cpu memory command
func run(args []string) error {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT)
	defer cancel()

	cpuWeight, err := strconv.Atoi(args[1])
	if err != nil {
		return err
	}
	mem, err := strconv.Atoi(args[2])
	if err != nil {
		return err
	}

	limits := cgroupz.ResourceLimit{
		CpuWeight: cpuWeight,
		MaxMem:    mem,
		MaxIO: &cgroupz.IOLimit{
			MaxIO: 419,
			Maj:   8,
			Min:   0,
		},
	}
	fmt.Printf("limits %+v\n", limits)
	fmt.Printf("args: %v\n", args)

	job := jobs.New(ctx, args[3:], limits)

	var wg sync.WaitGroup

	if err := job.Start(); err != nil {
		return fmt.Errorf("start: %v", err)
	}

	for i := 0; i < 1; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = job.Stream(ctx, os.Stdout)
		}()
	}

	if err := job.Wait(); err != nil {
		return fmt.Errorf("wait: %w", err)
	}

	code, status := job.Result()
	fmt.Printf("code: %d status: %s\n", code, status)
	if job.Err != "" {
		fmt.Printf("received err: %s\n", job.Err)
	}
	wg.Wait()

	return nil
}
