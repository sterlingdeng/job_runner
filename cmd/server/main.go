package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"job_runner/lib/jobs"
	"job_runner/lib/utils"
	"job_runner/pkg/authn"
	"job_runner/proto"
)

const (
	caCertPath = "/home/vagrant/fixtures/ca-cert.pem"
	certPath   = "/home/vagrant/fixtures/server-cert.pem"
	keyPath    = "/home/vagrant/fixtures/server-priv.key"
	port       = ":8080"
)

func main() {
	if err := cmd(); err != nil {
		fmt.Printf("%v\n", err)
		os.Exit(1)
	}
	os.Exit(0)
}

func cmd() error {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGKILL)
	defer cancel()

	cert, key, ca, err := utils.GetCertsFromPath(certPath, keyPath, caCertPath)
	if err != nil {
		return fmt.Errorf("GetCertsFromPath: %w", err)
	}

	tlsConfig, err := utils.GetTlsConfig(ca, cert, key)
	if err != nil {
		return fmt.Errorf("GetTlsConfig: %w", err)
	}

	server := grpc.NewServer(
		grpc.Creds(credentials.NewTLS(tlsConfig)),
		grpc.ChainUnaryInterceptor(authn.UnaryServerInterceptor),
		grpc.ChainStreamInterceptor(authn.StreamServerInterceptor),
	)
	jobService := jobs.NewService(ctx)
	jobsAPI := jobs.NewJobs(ctx, jobService)
	proto.RegisterJobServiceServer(server, jobsAPI)

	listener, err := net.Listen("tcp", port)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}

	go func() {
		defer cancel()
		fmt.Printf("starting grpc server on %s\n", port)
		if err := server.Serve(listener); err != nil {
			fmt.Printf("server error: %s\n", err.Error())
		}
	}()

	<-ctx.Done()
	// stop api first
	server.GracefulStop()
	// stop service
	jobService.Shutdown()
	return nil
}
