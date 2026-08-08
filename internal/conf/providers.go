package conf

import "github.com/google/wire"

func NewHTTPConfig(b *Bootstrap) *HTTP {
	if b == nil || b.Server == nil {
		return nil
	}
	return b.Server.GetHttp()
}

func NewGRPCConfig(b *Bootstrap) *GRPC {
	if b == nil || b.Server == nil {
		return nil
	}
	return b.Server.GetGrpc()
}

func NewDataConfig(b *Bootstrap) *Data {
	if b == nil {
		return nil
	}
	return b.Data
}

func NewModelGatewayConfig(b *Bootstrap) *ModelGateway {
	if b == nil {
		return nil
	}
	return b.ModelGateway
}

var ProviderSet = wire.NewSet(NewHTTPConfig, NewGRPCConfig, NewDataConfig, NewModelGatewayConfig)
