package lexicon

import (
	"context"
	"testing"

	"google.golang.org/grpc/metadata"
)

func TestLocaleFromContextReadsLanguageMetadataAndFallsBack(t *testing.T) {
	english := metadata.NewIncomingContext(context.Background(), metadata.Pairs("accept-language", "en-US,en;q=0.9"))
	if got := LocaleFromContext(english); got != string(LocaleEnUS) {
		t.Fatalf("expected English locale, got %q", got)
	}
	unknown := metadata.NewIncomingContext(context.Background(), metadata.Pairs("accept-language", "fr-FR"))
	if got := LocaleFromContext(unknown); got != string(LocaleZhCN) {
		t.Fatalf("expected default Chinese locale, got %q", got)
	}
	if got := LocaleFromContext(context.Background()); got != string(DefaultLocale) {
		t.Fatalf("expected default locale, got %q", got)
	}
}
