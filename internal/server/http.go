package server

import (
	"context"
	"fmt"
	"net/http"

	v1 "companion-service/api/companion/v1"
	"companion-service/internal/conf"
	"companion-service/internal/service"

	"github.com/gaoyong06/go-pkg/health"
	"github.com/gaoyong06/go-pkg/middleware/user_id"
	"github.com/go-kratos/kratos/v2/middleware/recovery"
	kratoshttp "github.com/go-kratos/kratos/v2/transport/http"
	"google.golang.org/protobuf/encoding/protojson"
)

const serviceName = "companion-service"

func NewHTTPServer(c *conf.HTTP, s *service.CompanionService) *kratoshttp.Server {
	var opts []kratoshttp.ServerOption
	opts = append(opts, kratoshttp.Middleware(recovery.Recovery(), user_id.Middleware()))
	if c != nil {
		if c.Network != "" {
			opts = append(opts, kratoshttp.Network(c.Network))
		}
		if c.Addr != "" {
			opts = append(opts, kratoshttp.Address(c.Addr))
		}
		if c.Timeout != nil {
			opts = append(opts, kratoshttp.Timeout(c.Timeout.AsDuration()))
		}
	}
	srv := kratoshttp.NewServer(opts...)
	v1.RegisterCompanionHTTPServer(srv, s)
	srv.Route("/").POST("/companion/v1/conversations/{conversation_id}/messages:stream", func(ctx kratoshttp.Context) error {
		var request v1.SendMessageRequest
		if err := ctx.Bind(&request); err != nil {
			return err
		}
		if err := ctx.BindQuery(&request); err != nil {
			return err
		}
		if err := ctx.BindVars(&request); err != nil {
			return err
		}
		httpContext := ctx
		handler := ctx.Middleware(func(ctx context.Context, req interface{}) (interface{}, error) {
			writer := httpContext.Response()
			writer.Header().Set("Content-Type", "text/event-stream")
			writer.Header().Set("Cache-Control", "no-cache")
			writer.Header().Set("X-Accel-Buffering", "no")
			flusher, ok := writer.(http.Flusher)
			if !ok {
				return nil, fmt.Errorf("streaming response is not supported")
			}
			responseStarted := false
			err := s.SendMessageStreamWithEmitter(ctx, req.(*v1.SendMessageRequest), func(chunk *v1.MessageChunk) error {
				started, writeErr := writeSSEChunk(writer, flusher, chunk)
				responseStarted = responseStarted || started
				return writeErr
			})
			if err != nil && responseStarted {
				// HTTP headers are already committed; returning the error would make Kratos write a second status line.
				return nil, nil
			}
			return nil, err
		})
		_, err := handler(ctx, &request)
		return err
	})
	srv.Route("/").GET("/health", func(ctx kratoshttp.Context) error {
		return ctx.Result(http.StatusOK, health.NewResponse(serviceName))
	})
	return srv
}

func writeSSEChunk(writer http.ResponseWriter, flusher http.Flusher, chunk *v1.MessageChunk) (bool, error) {
	payload, err := protojson.Marshal(chunk)
	if err != nil {
		return false, err
	}
	n, err := fmt.Fprintf(writer, "data: %s\n\n", payload)
	if n > 0 {
		flusher.Flush()
	}
	return n > 0, err
}
