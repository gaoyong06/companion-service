package lexicon

import (
	"context"

	companionI18n "companion-service/internal/i18n"
	kratosTransport "github.com/go-kratos/kratos/v2/transport"
	"google.golang.org/grpc/metadata"
)

// LocaleFromContext 从 HTTP/gRPC 请求头读取语言偏好；缺失或不支持时回退默认语言。
func LocaleFromContext(ctx context.Context) string {
	if ctx == nil {
		return string(DefaultLocale)
	}
	if transportContext, ok := kratosTransport.FromServerContext(ctx); ok {
		if locale := transportContext.RequestHeader().Get("Accept-Language"); locale != "" {
			return localeFromHeader(locale)
		}
	}
	if incoming, ok := metadata.FromIncomingContext(ctx); ok {
		if values := incoming.Get("accept-language"); len(values) > 0 {
			return localeFromHeader(values[0])
		}
	}
	return string(DefaultLocale)
}

func localeFromHeader(header string) string {
	return string(companionI18n.LocaleFromAcceptLanguage(header))
}
