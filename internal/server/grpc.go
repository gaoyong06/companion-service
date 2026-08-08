package server

import (
	v1 "companion-service/api/companion/v1"
	"companion-service/internal/conf"
	"companion-service/internal/service"

	"github.com/gaoyong06/go-pkg/middleware/user_id"
	"github.com/go-kratos/kratos/v2/middleware/recovery"
	kratosgrpc "github.com/go-kratos/kratos/v2/transport/grpc"
)

func NewGRPCServer(c *conf.GRPC, s *service.CompanionService) *kratosgrpc.Server {
	var opts []kratosgrpc.ServerOption
	opts = append(opts, kratosgrpc.Middleware(recovery.Recovery(), user_id.Middleware()))
	if c != nil {
		if c.Network != "" {
			opts = append(opts, kratosgrpc.Network(c.Network))
		}
		if c.Addr != "" {
			opts = append(opts, kratosgrpc.Address(c.Addr))
		}
		if c.Timeout != nil {
			opts = append(opts, kratosgrpc.Timeout(c.Timeout.AsDuration()))
		}
	}
	srv := kratosgrpc.NewServer(opts...)
	v1.RegisterCompanionServer(srv, s)
	return srv
}
