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
