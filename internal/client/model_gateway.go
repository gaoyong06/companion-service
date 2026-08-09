package client

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"companion-service/internal/conf"
	modelv1 "model-gateway/api/model_gateway/v1"

	"github.com/go-kratos/kratos/v2/middleware/recovery"
	kratosgrpc "github.com/go-kratos/kratos/v2/transport/grpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"
)

type ModelGatewayClient struct {
	client  modelv1.ModelGatewayClient
	conn    *grpc.ClientConn
	timeout time.Duration
	apiKey  string
}

func NewModelGatewayClient(c *conf.ModelGateway) (*ModelGatewayClient, func(), error) {
	if c == nil || c.GrpcAddr == "" {
		return nil, nil, fmt.Errorf("model gateway grpc_addr is required")
	}
	timeout := 120 * time.Second
	if c.Timeout != nil && c.Timeout.AsDuration() > 0 {
		timeout = c.Timeout.AsDuration()
	}
	apiKey := strings.TrimSpace(os.Getenv(c.ApiKeyEnv))
	if apiKey == "" {
		return nil, nil, fmt.Errorf("model gateway API key environment variable %q is empty", c.ApiKeyEnv)
	}
	conn, err := kratosgrpc.DialInsecure(
		context.Background(),
		kratosgrpc.WithEndpoint(c.GrpcAddr),
		kratosgrpc.WithTimeout(timeout),
		kratosgrpc.WithMiddleware(recovery.Recovery()),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("dial model gateway: %w", err)
	}
	return &ModelGatewayClient{client: modelv1.NewModelGatewayClient(conn), conn: conn, timeout: timeout, apiKey: apiKey}, func() { _ = conn.Close() }, nil
}

func (c *ModelGatewayClient) Chat(ctx context.Context, req *modelv1.ChatCompletionRequest) (*modelv1.ChatCompletionReply, error) {
	callCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	callCtx = metadata.AppendToOutgoingContext(callCtx, "x-model-gateway-key", c.apiKey)
	return c.client.ChatCompletion(callCtx, req)
}

// Embed 将文本转换为供应商无关的向量表示。
func (c *ModelGatewayClient) Embed(ctx context.Context, input []string) (*modelv1.CreateEmbeddingReply, error) {
	if len(input) == 0 {
		return nil, fmt.Errorf("embedding input is required")
	}
	callCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	callCtx = metadata.AppendToOutgoingContext(callCtx, "x-model-gateway-key", c.apiKey)
	return c.client.CreateEmbedding(callCtx, &modelv1.CreateEmbeddingRequest{Input: input})
}

func (c *ModelGatewayClient) TranscribeAudio(ctx context.Context, req *modelv1.TranscribeAudioRequest) (*modelv1.TranscribeAudioReply, error) {
	if req == nil || len(req.AudioData) == 0 {
		return nil, fmt.Errorf("audio data is required")
	}
	callCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	callCtx = metadata.AppendToOutgoingContext(callCtx, "x-model-gateway-key", c.apiKey)
	return c.client.TranscribeAudio(callCtx, req)
}

func (c *ModelGatewayClient) SynthesizeSpeech(ctx context.Context, req *modelv1.SynthesizeSpeechRequest) (*modelv1.SynthesizeSpeechReply, error) {
	if req == nil || req.Text == "" {
		return nil, fmt.Errorf("speech text is required")
	}
	callCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	callCtx = metadata.AppendToOutgoingContext(callCtx, "x-model-gateway-key", c.apiKey)
	return c.client.SynthesizeSpeech(callCtx, req)
}

func (c *ModelGatewayClient) ChatStream(ctx context.Context, req *modelv1.ChatCompletionRequest) (modelv1.ModelGateway_ChatCompletionStreamClient, error) {
	callCtx, cancel := context.WithTimeout(ctx, c.timeout)
	callCtx = metadata.AppendToOutgoingContext(callCtx, "x-model-gateway-key", c.apiKey)
	request := proto.Clone(req).(*modelv1.ChatCompletionRequest)
	request.Stream = true
	stream, err := c.client.ChatCompletionStream(callCtx, request)
	if err != nil {
		cancel()
		return nil, err
	}
	return &cancelableChatStream{ModelGateway_ChatCompletionStreamClient: stream, cancel: cancel}, nil
}

// cancelableChatStream 将请求 context 的释放绑定到流关闭动作，避免 ChatStream 返回时提前取消 gRPC 流。
type cancelableChatStream struct {
	modelv1.ModelGateway_ChatCompletionStreamClient
	cancel context.CancelFunc
}

func (s *cancelableChatStream) CloseSend() error {
	err := s.ModelGateway_ChatCompletionStreamClient.CloseSend()
	s.cancel()
	return err
}
