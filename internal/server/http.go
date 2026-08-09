package server

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

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
	srv.Route("/").POST("/companion/v1/media-messages", func(ctx kratoshttp.Context) error {
		request := ctx.Request()
		request.Body = http.MaxBytesReader(ctx.Response(), request.Body, 100*1024*1024+1)
		if err := request.ParseMultipartForm(100 << 20); err != nil {
			return fmt.Errorf("parse media upload: %w", err)
		}
		file, header, err := request.FormFile("file")
		if err != nil {
			return fmt.Errorf("media file is required: %w", err)
		}
		defer file.Close()
		data, err := io.ReadAll(file)
		if err != nil {
			return fmt.Errorf("read media file: %w", err)
		}
		contentType := header.Header.Get("Content-Type")
		if contentType == "" {
			contentType = http.DetectContentType(data)
		}
		mediaType := strings.SplitN(contentType, "/", 2)[0]
		result, err := s.SendMediaMessage(ctx, &v1.SendMediaMessageRequest{Data: data, Filename: header.Filename, ContentType: contentType, MediaType: mediaType, Caption: request.FormValue("caption"), Synthesize: request.FormValue("synthesize") == "true", Voice: request.FormValue("voice")})
		if err != nil {
			return err
		}
		return ctx.JSON(http.StatusOK, result)
	})
	srv.Route("/").POST("/companion/v1/messages:stream", func(ctx kratoshttp.Context) error {
		var request v1.SendMessageRequest
		if err := ctx.Bind(&request); err != nil {
			return err
		}
		if err := ctx.BindQuery(&request); err != nil {
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
