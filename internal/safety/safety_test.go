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
