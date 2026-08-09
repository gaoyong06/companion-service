package client

import (
	"context"
	"errors"
	"testing"
	"time"

	assetv1 "asset-service/api/asset/v1"
	"companion-service/internal/conf"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

type fakeAssetServiceClient struct {
	req      *assetv1.UploadFileRequest
	metadata metadata.MD
	reply    *assetv1.UploadFileReply
	err      error
}

func (f *fakeAssetServiceClient) UploadFile(ctx context.Context, req *assetv1.UploadFileRequest, _ ...grpc.CallOption) (*assetv1.UploadFileReply, error) {
	f.req = req
	f.metadata, _ = metadata.FromOutgoingContext(ctx)
	return f.reply, f.err
}
func (*fakeAssetServiceClient) DownloadFile(context.Context, *assetv1.DownloadFileRequest, ...grpc.CallOption) (*assetv1.DownloadFileReply, error) {
	return nil, errors.New("not implemented")
}
func (*fakeAssetServiceClient) GetFileInfo(context.Context, *assetv1.GetFileInfoRequest, ...grpc.CallOption) (*assetv1.GetFileInfoReply, error) {
	return nil, errors.New("not implemented")
}
func (*fakeAssetServiceClient) GetFileURL(context.Context, *assetv1.GetFileURLRequest, ...grpc.CallOption) (*assetv1.GetFileURLReply, error) {
	return nil, errors.New("not implemented")
}
func (*fakeAssetServiceClient) DeleteFile(context.Context, *assetv1.DeleteFileRequest, ...grpc.CallOption) (*assetv1.DeleteFileReply, error) {
	return nil, errors.New("not implemented")
}
func (*fakeAssetServiceClient) ListFiles(context.Context, *assetv1.ListFilesRequest, ...grpc.CallOption) (*assetv1.ListFilesReply, error) {
	return nil, errors.New("not implemented")
}

func TestAssetClientUploadValidatesAndAddsAppMetadata(t *testing.T) {
	fake := &fakeAssetServiceClient{reply: &assetv1.UploadFileReply{FileId: "asset-1", Url: "https://asset.test/1"}}
	assetClient := &AssetClient{client: fake, appID: "companion-service", timeout: time.Second}
	reply, err := assetClient.Upload(context.Background(), &assetv1.UploadFileRequest{Filename: "photo.png", ContentType: "image/png", Data: []byte("data")})
	if err != nil || reply.FileId != "asset-1" {
		t.Fatalf("upload: %+v %v", reply, err)
	}
	if fake.req.Filename != "photo.png" || string(fake.req.Data) != "data" || fake.metadata.Get("x-app-id")[0] != "companion-service" {
		t.Fatalf("upload request metadata mismatch: req=%+v metadata=%v", fake.req, fake.metadata)
	}
	if _, err := assetClient.Upload(context.Background(), nil); err == nil {
		t.Fatal("expected nil request validation error")
	}
	if _, err := assetClient.Upload(context.Background(), &assetv1.UploadFileRequest{}); err == nil {
		t.Fatal("expected empty data validation error")
	}
}

func TestAssetClientUploadPropagatesRemoteError(t *testing.T) {
	fake := &fakeAssetServiceClient{err: errors.New("asset service unavailable")}
	assetClient := &AssetClient{client: fake, appID: "companion-service", timeout: time.Second}
	if _, err := assetClient.Upload(context.Background(), &assetv1.UploadFileRequest{Data: []byte("x")}); err == nil || !errors.Is(err, fake.err) {
		t.Fatalf("expected remote error, got %v", err)
	}
}

func TestNewAssetClientValidatesConfiguration(t *testing.T) {
	for _, cfg := range []*conf.AssetService{nil, {}, {GrpcAddr: "127.0.0.1:1"}} {
		if client, cleanup, err := NewAssetClient(cfg); err == nil || client != nil || cleanup != nil {
			t.Fatalf("expected asset client config error, client=%v cleanup=%T err=%v", client, cleanup, err)
		}
	}
}
