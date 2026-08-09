// Package i18n 提供 Companion 服务共用的语言标签解析和回退能力。
package i18n

import (
	"strings"

	"golang.org/x/text/language"
)

type Locale string

const (
	LocaleZhCN    Locale = "zh-CN"
	LocaleEnUS    Locale = "en-US"
	DefaultLocale        = LocaleZhCN
)

var (
	supportedTags    = []language.Tag{language.SimplifiedChinese, language.AmericanEnglish}
	supportedLocales = []Locale{LocaleZhCN, LocaleEnUS}
	localeMatcher    = language.NewMatcher(supportedTags)
)

// NormalizeLocale 将单个语言标签规范化为服务支持的 locale；不支持时回退默认语言。
func NormalizeLocale(rawLocale string) Locale {
	rawLocale = strings.TrimSpace(rawLocale)
	if rawLocale == "" {
		return DefaultLocale
	}
	switch strings.ToLower(rawLocale) {
	case "中文", "简体中文":
		return LocaleZhCN
	}
	_, index := language.MatchStrings(localeMatcher, rawLocale)
	if index < 0 || index >= len(supportedLocales) {
		return DefaultLocale
	}
	return supportedLocales[index]
}

// LocaleFromAcceptLanguage 从 HTTP Accept-Language 或 gRPC metadata 的语言头中选择最佳支持语言。
func LocaleFromAcceptLanguage(header string) Locale {
	if strings.TrimSpace(header) == "" {
		return DefaultLocale
	}
	_, index := language.MatchStrings(localeMatcher, header)
	if index < 0 || index >= len(supportedLocales) {
		return DefaultLocale
	}
	return supportedLocales[index]
}
