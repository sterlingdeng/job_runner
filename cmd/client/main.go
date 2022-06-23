package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/urfave/cli/v2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"job_runner/lib/jobs"
	"job_runner/lib/utils"
)

func main() {
	app := cli.NewApp()
	app.Commands = []*cli.Command{
		clientListCommand,
		clientStartCommand,
		clientStopCommand,
		clientStreamCommand,
	}
	app.Flags = []cli.Flag{
		&cli.StringFlag{
			Name:  "client-cert",
			Usage: "path to client certificate file",
			Value: "/home/vagrant/fixtures/client-cert.pem",
		},
		&cli.StringFlag{
			Name:  "client-key",
			Usage: "path to client key file",
			Value: "/home/vagrant/fixtures/client-priv.key",
		},
		&cli.StringFlag{
			Name:  "ca-cert",
			Usage: "path to the CA certificate file",
			Value: "/home/vagrant/fixtures/ca-cert.pem",
		},
		&cli.StringFlag{
			Name:  "target",
			Usage: "target address of server",
			Value: "localhost:8080",
		},
	}
	if err := app.Run(os.Args); err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}
	os.Exit(0)
}

type ClientConfig struct {
	Target     string
	CACertPath string
	CertPath   string
	KeyPath    string
}

func GetDefaultConfigFromCLI(c *cli.Context) ClientConfig {
	return ClientConfig{
		Target:     c.String("target"),
		CACertPath: c.String("ca-cert"),
		CertPath:   c.String("client-cert"),
		KeyPath:    c.String("client-key"),
	}
}

func (c ClientConfig) Build(ctx context.Context) (*jobs.Client, error) {
	cert, key, ca, err := utils.GetCertsFromPath(c.CertPath, c.KeyPath, c.CACertPath)
	if err != nil {
		return nil, err
	}
	tlsConfig, err := utils.GetTlsConfig(ca, cert, key)
	if err != nil {
		return nil, err
	}
	tlsConfig.InsecureSkipVerify = true

	cc, err := grpc.DialContext(
		ctx,
		c.Target,
		grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig)),
	)
	if err != nil {
		return nil, err
	}
	return jobs.NewClient(cc, os.Stdout), nil
}

var clientListCommand = &cli.Command{
	Name: "get",
	Flags: []cli.Flag{
		&cli.IntFlag{
			Name:     "id",
			Required: true,
		},
	},
	Action: func(c *cli.Context) error {
		ctx := c.Context
		clientConf := GetDefaultConfigFromCLI(c)
		client, err := clientConf.Build(ctx)
		if err != nil {
			return fmt.Errorf("Build: %w", err)
		}
		jobID := c.Int("id")
		job, err := client.Get(ctx, int32(jobID))
		if err != nil {
			return err
		}
		fmt.Printf("id: %d cmd: %s\n", job.GetId(), strings.Join(job.GetCmd(), " "))
		return nil
	},
}

var clientStartCommand = &cli.Command{
	Name: "start",
	Action: func(c *cli.Context) error {
		ctx := c.Context
		clientConf := GetDefaultConfigFromCLI(c)
		client, err := clientConf.Build(ctx)
		if err != nil {
			return fmt.Errorf("Build: %w", err)
		}
		if len(c.Args().Slice()) == 0 {
			return fmt.Errorf("missing cmd")
		}
		job, err := client.Start(ctx, c.Args().Slice())
		if err != nil {
			return err
		}
		fmt.Printf("job id: %d\n", job.Id)
		return nil
	},
}

var clientStopCommand = &cli.Command{
	Name: "stop",
	Flags: []cli.Flag{
		&cli.IntFlag{
			Name:     "id",
			Required: true,
		},
	},
	Action: func(c *cli.Context) error {
		ctx := c.Context
		clientConf := GetDefaultConfigFromCLI(c)
		client, err := clientConf.Build(ctx)
		if err != nil {
			return fmt.Errorf("Build: %w", err)
		}
		_, err = client.Stop(ctx, int32(c.Int("id")))
		if err != nil {
			return err
		}
		fmt.Printf("job stopped")
		return nil
	},
}

var clientStreamCommand = &cli.Command{
	Name: "stream",
	Flags: []cli.Flag{
		&cli.IntFlag{
			Name:     "id",
			Required: true,
		},
	},
	Action: func(c *cli.Context) error {
		ctx := c.Context
		clientConf := GetDefaultConfigFromCLI(c)
		client, err := clientConf.Build(ctx)
		if err != nil {
			return fmt.Errorf("Build: %w", err)
		}
		if err := client.Stream(ctx, int32(c.Int("id"))); err != nil {
			return fmt.Errorf("Stream: %w", err)
		}
		return nil
	},
}
