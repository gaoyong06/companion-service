package client

import (
	"context"
	"testing"

	"google.golang.org/grpc"
	modelv1 "model-gateway/api/model_gateway/v1"
)

type fakeChatStream struct {
	grpc.ClientStream
	closed bool
}

func (s *fakeChatStream) Recv() (*modelv1.ChatCompletionChunk, error) { return nil, nil }

func (s *fakeChatStream) CloseSend() error {
	s.closed = true
	return nil
}

func TestCancelableChatStreamCancelsOnClose(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	fake := &fakeChatStream{}
	stream := &cancelableChatStream{
		ModelGateway_ChatCompletionStreamClient: fake,
		cancel:                                  cancel,
	}
	select {
	case <-ctx.Done():
		t.Fatal("stream context must remain active before close")
	default:
	}

	if err := stream.CloseSend(); err != nil {
		t.Fatalf("close stream: %v", err)
	}
	if !fake.closed {
		t.Fatal("expected underlying stream to close")
	}
	select {
	case <-ctx.Done():
	default:
		t.Fatal("expected stream close to cancel request context")
	}
}
