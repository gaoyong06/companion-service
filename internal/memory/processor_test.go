package memory

import "testing"

func TestShouldSkipMemory(t *testing.T) {
	for _, content := range []string{"不要记住这件事", "Please do not remember this", "don't save this"} {
		if !ShouldSkipMemory(content) {
			t.Fatalf("expected memory skip for %q", content)
		}
	}
	if ShouldSkipMemory("I prefer jazz music") {
		t.Fatal("did not expect memory skip")
	}
}
