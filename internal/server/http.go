package server

import (
	"net/http"

	v1 "companion-service/api/companion/v1"
	"companion-service/internal/conf"
	"companion-service/internal/service"

	"github.com/gaoyong06/go-pkg/health"
	"github.com/gaoyong06/go-pkg/middleware/user_id"
	"github.com/go-kratos/kratos/v2/middleware/recovery"
	kratoshttp "github.com/go-kratos/kratos/v2/transport/http"
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
	srv.Route("/").GET("/health", func(ctx kratoshttp.Context) error {
		return ctx.Result(http.StatusOK, health.NewResponse(serviceName))
	})
	return srv
}
