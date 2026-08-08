package server

import (
	"net/http/httptest"
	"strings"
	"testing"

	v1 "companion-service/api/companion/v1"
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
