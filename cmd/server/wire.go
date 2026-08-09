//go:build wireinject
// +build wireinject

package main

import (
	"companion-service/internal/biz"
	"companion-service/internal/client"
	"companion-service/internal/conf"
	"companion-service/internal/data"
	"companion-service/internal/memory"
	"companion-service/internal/server"
	"companion-service/internal/service"

	"github.com/go-kratos/kratos/v2"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/wire"
)

func wireApp(*conf.Bootstrap, log.Logger) (*kratos.App, func(), error) {
	wire.Build(conf.ProviderSet, data.ProviderSet, client.ProviderSet, memory.ProviderSet, biz.ProviderSet, service.ProviderSet, server.ProviderSet,
		wire.Bind(new(biz.ConversationStore), new(*data.Store)),
		wire.Bind(new(client.ModelGateway), new(*client.ModelGatewayClient)),
		wire.Bind(new(client.AssetStorage), new(*client.AssetClient)),
		wire.Bind(new(biz.MemoryProcessor), new(*memory.Processor)),
		wire.Bind(new(memory.MemoryStore), new(*data.Store)),
		newApp)
	return nil, nil, nil
}
