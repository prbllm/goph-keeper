package grpcserver

import (
	"context"

	"google.golang.org/grpc/metadata"
)

// testServerStream is a minimal grpc.ServerStream implementation for interceptor unit tests.
// Interceptors only need Context() here.
type testServerStream struct {
	ctx context.Context
}

func (s *testServerStream) SetHeader(metadata.MD) error { return nil }
func (s *testServerStream) SendHeader(metadata.MD) error { return nil }
func (s *testServerStream) SetTrailer(metadata.MD)       {}
func (s *testServerStream) Context() context.Context    { return s.ctx }
func (s *testServerStream) SendMsg(any) error           { return nil }
func (s *testServerStream) RecvMsg(any) error           { return nil }

