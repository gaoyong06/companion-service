package i18n

import "testing"

func TestNormalizeLocaleMatchesSupportedLanguageTags(t *testing.T) {
	cases := map[string]Locale{
		"zh-CN":   LocaleZhCN,
		"zh-Hans": LocaleZhCN,
		"中文":      LocaleZhCN,
		"en":      LocaleEnUS,
		"en-GB":   LocaleEnUS,
		"fr-FR":   LocaleZhCN,
		"":        LocaleZhCN,
	}
	for rawLocale, want := range cases {
		if got := NormalizeLocale(rawLocale); got != want {
			t.Errorf("NormalizeLocale(%q) = %q, want %q", rawLocale, got, want)
		}
	}
}

func TestLocaleFromAcceptLanguageHonorsQualityAndFallback(t *testing.T) {
	cases := map[string]Locale{
		"en-US,en;q=0.9,zh-CN;q=0.8": LocaleEnUS,
		"fr-FR,en-GB;q=0.9":          LocaleEnUS,
		"fr-FR":                      LocaleZhCN,
		"":                           LocaleZhCN,
	}
	for header, want := range cases {
		if got := LocaleFromAcceptLanguage(header); got != want {
			t.Errorf("LocaleFromAcceptLanguage(%q) = %q, want %q", header, got, want)
		}
	}
}
