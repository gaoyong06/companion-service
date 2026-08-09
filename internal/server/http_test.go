package server

import (
	"net/http/httptest"
	"strings"
	"testing"

	v1 "companion-service/api/companion/v1"
	"companion-service/internal/conf"
	"companion-service/internal/service"
)

func TestWriteSSEChunk(t *testing.T) {
	writer := httptest.NewRecorder()
	chunk := &v1.MessageChunk{MessageId: "msg-1", Delta: "hello", Done: true, FinishReason: "stop"}

	started, err := writeSSEChunk(writer, writer, chunk)
	if err != nil {
		t.Fatalf("write SSE chunk: %v", err)
	}
	if !started {
		t.Fatal("expected SSE response to start")
	}
	if writer.Code != 200 {
		t.Fatalf("unexpected status code: %d", writer.Code)
	}
	if !strings.Contains(writer.Body.String(), `"messageId":"msg-1"`) || !strings.HasSuffix(writer.Body.String(), "\n\n") {
		t.Fatalf("unexpected SSE body: %q", writer.Body.String())
	}
}

func TestServerConstructorsAcceptDefaultAndExplicitConfig(t *testing.T) {
	application := service.NewCompanionService(nil)
	if server := NewHTTPServer(nil, application); server == nil {
		t.Fatal("expected HTTP server")
	}
	if server := NewGRPCServer(nil, application); server == nil {
		t.Fatal("expected gRPC server")
	}
	if server := NewHTTPServer(&conf.HTTP{Network: "tcp", Addr: "127.0.0.1:0"}, application); server == nil {
		t.Fatal("expected configured HTTP server")
	}
	if server := NewGRPCServer(&conf.GRPC{Network: "tcp", Addr: "127.0.0.1:0"}, application); server == nil {
		t.Fatal("expected configured gRPC server")
	}
}
