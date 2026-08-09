package client

import (
	"context"
	"fmt"
	"strings"
	"time"

	assetv1 "asset-service/api/asset/v1"
	"companion-service/internal/conf"
	kratosgrpc "github.com/go-kratos/kratos/v2/transport/grpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

type AssetStorage interface {
	Upload(context.Context, *assetv1.UploadFileRequest) (*assetv1.UploadFileReply, error)
}

type AssetClient struct {
	client  assetv1.AssetServiceClient
	conn    *grpc.ClientConn
	timeout time.Duration
	appID   string
}

func NewAssetClient(c *conf.AssetService) (*AssetClient, func(), error) {
	if c == nil || strings.TrimSpace(c.GrpcAddr) == "" {
		return nil, nil, fmt.Errorf("asset service grpc_addr is required")
	}
	if strings.TrimSpace(c.AppId) == "" {
		return nil, nil, fmt.Errorf("asset service app_id is required")
	}
	timeout := 10 * time.Minute
	if c.Timeout != nil && c.Timeout.AsDuration() > 0 {
		timeout = c.Timeout.AsDuration()
	}
	conn, err := kratosgrpc.DialInsecure(context.Background(), kratosgrpc.WithEndpoint(c.GrpcAddr), kratosgrpc.WithTimeout(timeout), kratosgrpc.WithOptions(grpc.WithDefaultCallOptions(
		grpc.MaxCallSendMsgSize(110*1024*1024),
		grpc.MaxCallRecvMsgSize(110*1024*1024),
	)))
	if err != nil {
		return nil, nil, fmt.Errorf("dial asset service: %w", err)
	}
	return &AssetClient{client: assetv1.NewAssetServiceClient(conn), conn: conn, timeout: timeout, appID: c.AppId}, func() { _ = conn.Close() }, nil
}

func (c *AssetClient) Upload(ctx context.Context, req *assetv1.UploadFileRequest) (*assetv1.UploadFileReply, error) {
	if req == nil || len(req.Data) == 0 {
		return nil, fmt.Errorf("asset upload data is required")
	}
	callCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	callCtx = metadata.AppendToOutgoingContext(callCtx, "x-app-id", c.appID)
	return c.client.UploadFile(callCtx, req)
}
