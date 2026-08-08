package memory

import "testing"

func TestParseCandidatesFiltersUnsafeAndLowConfidenceItems(t *testing.T) {
	candidates, err := parseCandidates(`prefix {"memories":[
{"kind":"preference","content":"likes jazz","confidence":0.95,"importance":4},
{"kind":"fact","content":"password is secret","confidence":1,"importance":5},
{"kind":"fact","content":"temporary mood","confidence":0.5,"importance":2},
{"kind":"unknown","content":"ignored","confidence":1,"importance":3}
]} suffix`)
	if err != nil {
		t.Fatalf("parse candidates: %v", err)
	}
	if len(candidates) != 1 || candidates[0].Content != "likes jazz" {
		t.Fatalf("unexpected candidates: %+v", candidates)
	}
}

func TestParseCandidatesClampsValuesAndLimitsCount(t *testing.T) {
	content := `{"memories":[
{"kind":"goal","content":"finish project","confidence":2,"importance":99},
{"kind":"goal","content":"second","confidence":0.8,"importance":0},
{"kind":"goal","content":"third","confidence":0.8,"importance":2},
{"kind":"goal","content":"fourth","confidence":0.8,"importance":2},
{"kind":"goal","content":"fifth","confidence":0.8,"importance":2},
{"kind":"goal","content":"sixth","confidence":0.8,"importance":2}
]}`
	candidates, err := parseCandidates(content)
	if err != nil {
		t.Fatalf("parse candidates: %v", err)
	}
	if len(candidates) != 5 || candidates[0].Confidence != 1 || candidates[0].Importance != 5 {
		t.Fatalf("unexpected normalized candidates: %+v", candidates)
	}
}
