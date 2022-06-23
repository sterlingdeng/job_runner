package authn

import (
	"context"
	"encoding/base64"
	"fmt"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

var ErrMissingID = fmt.Errorf("missing authn id")

const authnKeyString = "authn_id"

type UserID string

func FromMD(ctx context.Context) (UserID, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return "", ErrMissingID
	}
	id := md.Get(authnKeyString)
	if len(id) == 1 {
		return UserID(id[0]), nil
	}
	return "", ErrMissingID
}

func UnaryServerInterceptor(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	ctx, err := putHashedCertOnCtx(ctx)
	if err != nil {
		return nil, status.Error(codes.Unauthenticated, "failed to get id")
	}
	return handler(ctx, req)
}

type wrapper struct {
	grpc.ServerStream
	ctx context.Context
}

func (w *wrapper) Context() context.Context {
	return w.ctx
}

func StreamServerInterceptor(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
	ctx, err := putHashedCertOnCtx(ss.Context())
	if err != nil {
		return status.Error(codes.Unauthenticated, "failed to get id")
	}
	return handler(srv, &wrapper{ss, ctx})
}

func putHashedCertOnCtx(ctx context.Context) (context.Context, error) {
	peer, ok := peer.FromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "missing cert")
	}
	peerInfo, ok := peer.AuthInfo.(credentials.TLSInfo)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "missing cert")
	}
	if len(peerInfo.State.PeerCertificates) == 0 {
		return nil, status.Error(codes.Unauthenticated, "missing cert")
	}
	cert := peerInfo.State.PeerCertificates[0]
	str := base64.StdEncoding.EncodeToString(cert.RawSubject)
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		md = metadata.New(make(map[string]string))
	}
	md.Append("authn_id", str)
	ctx = metadata.NewIncomingContext(ctx, md)
	return ctx, nil
}
