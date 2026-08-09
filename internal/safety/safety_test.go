package safety

import "testing"

func TestCheckCrisisContent(t *testing.T) {
	for _, content := range []string{"我不想活了", "I want to kill myself"} {
		if Check(content) != LevelCrisis {
			t.Fatalf("expected crisis level for %q", content)
		}
	}
	if Check("今天有点累") != LevelNormal {
		t.Fatal("expected normal level")
	}
}

func TestCrisisResponseIsNonEmpty(t *testing.T) {
	if Response(LevelCrisis) == "" || Response(LevelNormal) != "" {
		t.Fatal("unexpected safety responses")
	}
}

func TestCheckRecognizesEnglishAndChineseCrisisMarkersWithoutFalsePositive(t *testing.T) {
	for _, content := range []string{"I want to die", "I am suicidal", "我想自杀", "结束生命"} {
		if Check(content) != LevelCrisis {
			t.Fatalf("expected crisis level for %q", content)
		}
	}
	for _, content := range []string{"I want to live", "这个项目要结束了", "今天不想吃饭"} {
		if Check(content) != LevelNormal {
			t.Fatalf("expected normal level for %q", content)
		}
	}
}

func TestCrisisResponseSupportsEnglishLocale(t *testing.T) {
	response := ResponseForLocale(LevelCrisis, "en-US")
	if response == "" || response == Response(LevelCrisis) {
		t.Fatalf("expected localized English crisis response, got %q", response)
	}
}
