package jobs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"job_runner/proto"

	"google.golang.org/grpc"
)

type Client struct {
	conn proto.JobServiceClient
	out  io.Writer
}

func NewClient(conn grpc.ClientConnInterface, out io.Writer) *Client {
	client := proto.NewJobServiceClient(conn)
	return &Client{
		conn: client,
		out:  out,
	}
}

func (c *Client) Get(ctx context.Context, jobID int32) (*proto.Job, error) {
	return c.conn.Get(ctx, &proto.GetRequest{Id: jobID})
}

func (c *Client) Start(ctx context.Context, cmd []string) (*proto.Job, error) {
	return c.conn.Start(ctx, &proto.StartRequest{Cmd: cmd})
}

func (c *Client) Stop(ctx context.Context, id int32) (*proto.StopResponse, error) {
	return c.conn.Stop(ctx, &proto.StopRequest{Id: id})
}

func (c *Client) Stream(ctx context.Context, id int32) error {
	stream, err := c.conn.Stream(ctx, &proto.StreamRequest{Id: id})
	if err != nil {
		return err
	}
	defer func() {
		err = stream.CloseSend()
	}()

loop:
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			resp, err := stream.Recv()
			if len(resp.GetStream()) > 0 {
				fmt.Fprint(c.out, string(resp.GetStream()))
			}
			if errors.Is(err, io.EOF) {
				break loop
			}
			if err != nil {
				return fmt.Errorf("recv: %w", err)
			}
		}
	}
	return nil
}
