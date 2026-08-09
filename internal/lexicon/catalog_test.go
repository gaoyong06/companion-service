package lexicon

import "testing"

func TestForLocaleReturnsStableLocalizedCatalog(t *testing.T) {
	chinese := ForLocale("zh-CN")
	if chinese.Locale != LocaleZhCN || chinese.Version != CatalogVersion || chinese.Prompts.FirstMeeting == "" || chinese.Safety.CrisisResponse == "" {
		t.Fatalf("unexpected Chinese catalog: %+v", chinese)
	}
	if len(chinese.Safety.CrisisMarkers) == 0 || len(chinese.Memory.SkipMarkers) == 0 || len(chinese.Memory.SensitiveTerms) == 0 {
		t.Fatal("Chinese policy word lists must not be empty")
	}
	english := ForLocale("en-US")
	if english.Locale != LocaleEnUS || english.Prompts.CompanionSystem == chinese.Prompts.CompanionSystem || english.Safety.CrisisResponse == chinese.Safety.CrisisResponse {
		t.Fatal("English catalog must provide localized content")
	}
	if english.Prompts.MemoryExtraction != chinese.Prompts.MemoryExtraction {
		t.Fatal("memory extraction contract should remain provider-language stable")
	}
	entries := english.Entries()
	if len(entries) != 12 || entries[0].Key != KeyCompanionSystem || entries[0].Version != CatalogVersion || len(entries[9].Values) == 0 {
		t.Fatalf("unexpected stable catalog entries: %+v", entries)
	}
}

func TestForLocaleNormalizesAliasesAndFallsBackSafely(t *testing.T) {
	for _, locale := range []string{"en", " EN_us ", "en-US"} {
		if got := ForLocale(locale).Locale; got != LocaleEnUS {
			t.Fatalf("expected English locale for %q, got %q", locale, got)
		}
	}
	for _, locale := range []string{"zh", "简体中文", "unknown-locale", ""} {
		if got := ForLocale(locale).Locale; got != LocaleZhCN {
			t.Fatalf("expected Chinese fallback for %q, got %q", locale, got)
		}
	}
}
