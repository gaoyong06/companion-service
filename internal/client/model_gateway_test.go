package client

import (
	"context"
	"errors"
	"testing"
	"time"

	"companion-service/internal/conf"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
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

type fakeModelGatewayClient struct {
	chatReply         *modelv1.ChatCompletionReply
	chatErr           error
	stream            modelv1.ModelGateway_ChatCompletionStreamClient
	streamErr         error
	embedReply        *modelv1.CreateEmbeddingReply
	embedErr          error
	transcribeReply   *modelv1.TranscribeAudioReply
	transcribeErr     error
	speechReply       *modelv1.SynthesizeSpeechReply
	speechErr         error
	chatRequest       *modelv1.ChatCompletionRequest
	streamRequest     *modelv1.ChatCompletionRequest
	embedRequest      *modelv1.CreateEmbeddingRequest
	transcribeRequest *modelv1.TranscribeAudioRequest
	speechRequest     *modelv1.SynthesizeSpeechRequest
	lastOutgoingKey   string
	streamContext     context.Context
}

func (f *fakeModelGatewayClient) capture(ctx context.Context) {
	if md, ok := metadata.FromOutgoingContext(ctx); ok {
		if values := md.Get("x-model-gateway-key"); len(values) > 0 {
			f.lastOutgoingKey = values[0]
		}
	}
}
func (f *fakeModelGatewayClient) ChatCompletion(ctx context.Context, req *modelv1.ChatCompletionRequest, _ ...grpc.CallOption) (*modelv1.ChatCompletionReply, error) {
	f.capture(ctx)
	f.chatRequest = req
	return f.chatReply, f.chatErr
}
func (f *fakeModelGatewayClient) ChatCompletionStream(ctx context.Context, req *modelv1.ChatCompletionRequest, _ ...grpc.CallOption) (modelv1.ModelGateway_ChatCompletionStreamClient, error) {
	f.capture(ctx)
	f.streamRequest = req
	f.streamContext = ctx
	return f.stream, f.streamErr
}
func (f *fakeModelGatewayClient) CreateEmbedding(ctx context.Context, req *modelv1.CreateEmbeddingRequest, _ ...grpc.CallOption) (*modelv1.CreateEmbeddingReply, error) {
	f.capture(ctx)
	f.embedRequest = req
	return f.embedReply, f.embedErr
}
func (f *fakeModelGatewayClient) TranscribeAudio(ctx context.Context, req *modelv1.TranscribeAudioRequest, _ ...grpc.CallOption) (*modelv1.TranscribeAudioReply, error) {
	f.capture(ctx)
	f.transcribeRequest = req
	return f.transcribeReply, f.transcribeErr
}
func (f *fakeModelGatewayClient) SynthesizeSpeech(ctx context.Context, req *modelv1.SynthesizeSpeechRequest, _ ...grpc.CallOption) (*modelv1.SynthesizeSpeechReply, error) {
	f.capture(ctx)
	f.speechRequest = req
	return f.speechReply, f.speechErr
}
func (*fakeModelGatewayClient) ListModels(context.Context, *modelv1.ListModelsRequest, ...grpc.CallOption) (*modelv1.ListModelsReply, error) {
	return nil, errors.New("not implemented")
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

func TestModelGatewayClientDelegatesAllUnaryCapabilitiesAndAddsAPIKey(t *testing.T) {
	fake := &fakeModelGatewayClient{
		chatReply:       &modelv1.ChatCompletionReply{Content: "hello"},
		embedReply:      &modelv1.CreateEmbeddingReply{Model: "embed"},
		transcribeReply: &modelv1.TranscribeAudioReply{Text: "spoken"},
		speechReply:     &modelv1.SynthesizeSpeechReply{AudioData: []byte("audio")},
	}
	client := &ModelGatewayClient{client: fake, timeout: time.Second, apiKey: "secret"}
	ctx := context.Background()
	if reply, err := client.Chat(ctx, &modelv1.ChatCompletionRequest{Messages: []*modelv1.ChatMessage{{Role: "user", Content: "hi"}}}); err != nil || reply.Content != "hello" {
		t.Fatalf("chat: reply=%+v err=%v", reply, err)
	}
	if reply, err := client.Embed(ctx, []string{"hello"}); err != nil || reply.Model != "embed" || len(fake.embedRequest.Input) != 1 {
		t.Fatalf("embed: reply=%+v request=%+v err=%v", reply, fake.embedRequest, err)
	}
	if reply, err := client.TranscribeAudio(ctx, &modelv1.TranscribeAudioRequest{AudioData: []byte("raw")}); err != nil || reply.Text != "spoken" {
		t.Fatalf("transcribe: reply=%+v err=%v", reply, err)
	}
	if reply, err := client.SynthesizeSpeech(ctx, &modelv1.SynthesizeSpeechRequest{Text: "hello"}); err != nil || string(reply.AudioData) != "audio" {
		t.Fatalf("speech: reply=%+v err=%v", reply, err)
	}
	if fake.lastOutgoingKey != "secret" {
		t.Fatalf("expected API key metadata, got %q", fake.lastOutgoingKey)
	}
}

func TestModelGatewayClientValidatesEmptyCapabilityInputs(t *testing.T) {
	client := &ModelGatewayClient{client: &fakeModelGatewayClient{}, timeout: time.Second, apiKey: "secret"}
	if _, err := client.Embed(context.Background(), nil); err == nil {
		t.Fatal("expected empty embedding input error")
	}
	if _, err := client.TranscribeAudio(context.Background(), nil); err == nil {
		t.Fatal("expected nil transcription request error")
	}
	if _, err := client.TranscribeAudio(context.Background(), &modelv1.TranscribeAudioRequest{}); err == nil {
		t.Fatal("expected empty audio error")
	}
	if _, err := client.SynthesizeSpeech(context.Background(), nil); err == nil {
		t.Fatal("expected nil speech request error")
	}
	if _, err := client.SynthesizeSpeech(context.Background(), &modelv1.SynthesizeSpeechRequest{}); err == nil {
		t.Fatal("expected empty speech text error")
	}
}

func TestModelGatewayClientChatStreamClonesRequestAndClosesContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	fakeStream := &fakeChatStream{}
	fake := &fakeModelGatewayClient{stream: fakeStream}
	client := &ModelGatewayClient{client: fake, timeout: time.Second, apiKey: "secret"}
	original := &modelv1.ChatCompletionRequest{Stream: false, Model: "chat", Messages: []*modelv1.ChatMessage{{Role: "user", Content: "hello"}}}
	stream, err := client.ChatStream(ctx, original)
	if err != nil {
		t.Fatalf("chat stream: %v", err)
	}
	if original.Stream {
		t.Fatal("chat stream must not mutate caller request")
	}
	if fake.streamRequest == original || !fake.streamRequest.Stream || fake.streamRequest.Model != "chat" {
		t.Fatalf("expected cloned streaming request, got %+v", fake.streamRequest)
	}
	if err := stream.CloseSend(); err != nil {
		t.Fatalf("close stream: %v", err)
	}
	select {
	case <-fake.streamContext.Done():
	default:
		t.Fatal("expected stream close to cancel request context")
	}
}

func TestNewModelGatewayClientValidatesConfiguration(t *testing.T) {
	if _, _, err := NewModelGatewayClient(nil); err == nil {
		t.Fatal("expected nil configuration error")
	}
	if _, _, err := NewModelGatewayClient(&conf.ModelGateway{}); err == nil {
		t.Fatal("expected missing address error")
	}
	t.Setenv("COMPANION_MODEL_GATEWAY_TEST_KEY", "")
	if _, _, err := NewModelGatewayClient(&conf.ModelGateway{GrpcAddr: "127.0.0.1:1", ApiKeyEnv: "COMPANION_MODEL_GATEWAY_TEST_KEY"}); err == nil {
		t.Fatal("expected missing API key error")
	}
	t.Setenv("COMPANION_MODEL_GATEWAY_TEST_KEY", "secret")
	client, cleanup, err := NewModelGatewayClient(&conf.ModelGateway{GrpcAddr: "127.0.0.1:1", ApiKeyEnv: "COMPANION_MODEL_GATEWAY_TEST_KEY"})
	if err != nil || client == nil || cleanup == nil {
		t.Fatalf("expected valid client configuration, client nil=%t cleanup nil=%t err=%v", client == nil, cleanup == nil, err)
	}
	cleanup()
}
