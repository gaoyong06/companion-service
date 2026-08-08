//go:build wireinject
// +build wireinject

package main

import (
	"companion-service/internal/biz"
	"companion-service/internal/client"
	"companion-service/internal/conf"
	"companion-service/internal/data"
	"companion-service/internal/server"
	"companion-service/internal/service"

	"github.com/go-kratos/kratos/v2"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/wire"
)

func wireApp(*conf.Bootstrap, log.Logger) (*kratos.App, func(), error) {
	wire.Build(conf.ProviderSet, data.ProviderSet, client.ProviderSet, biz.ProviderSet, service.ProviderSet, server.ProviderSet, newApp)
	return nil, nil, nil
}
