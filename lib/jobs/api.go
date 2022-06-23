package jobs

import (
	"context"
	"fmt"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"job_runner/pkg/authn"
	"job_runner/pkg/authorizer"
	"job_runner/pkg/cgroupz"
	"job_runner/proto"
)

var _ proto.JobServiceServer = (*Jobs)(nil)

type Jobs struct {
	lib   *Service
	authz *authorizer.Authorizer

	ctx context.Context // used to control cancellation of actively running commands
}

// NewJobs returns a jobs api struct that implements the JobServiceServer grpc interface
func NewJobs(ctx context.Context, lib *Service) *Jobs {
	svc := Jobs{
		lib: lib,
		ctx: ctx,
	}
	return &svc
}

func (j *Jobs) Get(ctx context.Context, req *proto.GetRequest) (*proto.Job, error) {
	userID, err := authn.FromMD(ctx)
	if err != nil {
		return nil, status.Error(codes.Unauthenticated, "missing id")
	}
	ok, err := j.authz.HasAccess(string(userID), authorizer.ActionGet)
	if err != nil {
		return nil, status.Error(codes.Unknown, "")
	}
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "unauthenticated")
	}
	cmd, err := j.lib.GetJob(ctx, req.GetId())
	if err != nil {
		return nil, err
	}

	job := proto.Job{
		Id:     cmd.ID,
		Status: string(cmd.Job.Status),
	}

	return &job, nil
}

func (j *Jobs) Start(ctx context.Context, req *proto.StartRequest) (*proto.Job, error) {
	fmt.Println("Starting..")
	userID, err := authn.FromMD(ctx)
	if err != nil {
		return nil, status.Error(codes.Unauthenticated, "missing id")
	}
	ok, err := j.authz.HasAccess(string(userID), authorizer.ActionStart)
	if err != nil {
		return nil, status.Error(codes.Unknown, "")
	}
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "unauthenticated")
	}

	cmd := req.GetCmd()

	job, err := j.lib.StartJob(ctx, cmd, cgroupz.ResourceLimit{
		CpuWeight: 100,
		MaxMem:    1e8,
		MaxIO:     nil,
	})
	if err != nil {
		return nil, err
	}

	resp := proto.Job{
		Id:  job.ID,
		Cmd: cmd,
	}
	return &resp, nil
}

func (j *Jobs) Stop(ctx context.Context, req *proto.StopRequest) (*proto.StopResponse, error) {
	fmt.Println("Stopping..")

	userID, err := authn.FromMD(ctx)
	if err != nil {
		return nil, status.Error(codes.Unauthenticated, "missing id")
	}
	ok, err := j.authz.HasAccess(string(userID), authorizer.ActionStop)
	if err != nil {
		return nil, status.Error(codes.Unknown, "")
	}
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "unauthenticated")
	}

	exitCode, jobStatus, err := j.lib.StopJob(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	return &proto.StopResponse{
		ExitCode: int32(exitCode),
		Status:   string(jobStatus),
	}, nil
}

// Stream starts from the beginning of the log
func (j *Jobs) Stream(req *proto.StreamRequest, server proto.JobService_StreamServer) error {
	userID, err := authn.FromMD(server.Context())
	if err != nil {
		return status.Error(codes.Unauthenticated, "missing id")
	}
	ok, err := j.authz.HasAccess(string(userID), authorizer.ActionStream)
	if err != nil {
		return status.Error(codes.Unknown, "")
	}
	if !ok {
		return status.Error(codes.Unauthenticated, "unauthenticated")
	}

	fmt.Println("Streaming..")
	err = j.lib.StreamJob(server.Context(), req.GetId(), &streamWriter{server})
	if err != nil {
		return err
	}
	return nil
}

type streamWriter struct {
	proto.JobService_StreamServer
}

func (s *streamWriter) Write(p []byte) (int, error) {
	select {
	case <-s.Context().Done():
		return 0, s.Context().Err()
	default:
		if err := s.Send(&proto.StreamResponse{Stream: p}); err != nil {
			return 0, err
		}
	}
	return len(p), nil
}
