package jobs

import (
	"context"
	"fmt"
	"io"
	"sync"

	"job_runner/pkg/cgroupz"
	"job_runner/pkg/jobs"
)

type JobRecord struct {
	ID     int32
	Job    jobs.Job
	cancel func()
	ctx    context.Context
}

type Service struct {
	ider
	sync.Mutex
	store map[int32]JobRecord

	wg        sync.WaitGroup
	parentCtx context.Context
	cancel    func()
}

func NewService(ctx context.Context) *Service {
	parentCtx, cancel := context.WithCancel(ctx)
	return &Service{
		parentCtx: parentCtx,
		cancel:    cancel,
		store:     make(map[int32]JobRecord),
	}
}

func (s *Service) StartJob(ctx context.Context, cmdStr []string, limits cgroupz.ResourceLimit) (JobRecord, error) {
	jobCtx, cancel := context.WithCancel(ctx)
	job := jobs.New(jobCtx, cmdStr, limits)
	id := s.nextID()

	record := JobRecord{ID: id, Job: job, cancel: cancel, ctx: jobCtx}

	s.Lock()
	if _, ok := s.store[id]; ok {
		return JobRecord{}, fmt.Errorf("id %d already exists", id)
	}
	s.store[id] = record
	s.Unlock()

	if err := job.Start(); err != nil {
		return JobRecord{}, err
	}

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		err := job.Wait()
		if err != nil {
			fmt.Printf("error executing job with id %d: %v\n", id, err)
		}
	}()

	return record, nil
}

func (s *Service) GetJob(ctx context.Context, jobID int32) (JobRecord, error) {
	s.Lock()
	defer s.Unlock()
	job, ok := s.store[jobID]
	if !ok {
		return JobRecord{}, fmt.Errorf("job not found")
	}
	return job, nil
}

func (s *Service) StopJob(ctx context.Context, jobID int32) (int, jobs.Status, error) {
	s.Lock()
	job, ok := s.store[jobID]
	if !ok {
		s.Unlock()
		return -1, "", fmt.Errorf("job not found")
	}
	job.cancel()
	s.Unlock()
	select {
	case <-job.ctx.Done():
		code, status := job.Job.Result()
		return code, status, nil
	case <-ctx.Done():
		return -1, "", ctx.Err()
	}
}

func (s *Service) StreamJob(ctx context.Context, jobID int32, writer io.Writer) error {
	job, err := s.GetJob(ctx, jobID)
	if err != nil {
		return fmt.Errorf("getJob: %w", err)
	}
	if err := job.Job.Stream(ctx, writer); err != nil {
		return fmt.Errorf("job.Stream: %w", err)
	}
	return nil
}

func (s *Service) Shutdown() {
	s.cancel()
	s.wg.Wait()
}
