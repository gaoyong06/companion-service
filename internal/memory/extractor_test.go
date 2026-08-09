package memory

import (
	"context"
	"errors"
	"strings"
	"testing"

	"companion-service/internal/client"
	modelv1 "model-gateway/api/model_gateway/v1"
)

type extractorModel struct {
	content string
	err     error
	request *modelv1.ChatCompletionRequest
}

func (m *extractorModel) Chat(_ context.Context, request *modelv1.ChatCompletionRequest) (*modelv1.ChatCompletionReply, error) {
	m.request = request
	if m.err != nil {
		return nil, m.err
	}
	return &modelv1.ChatCompletionReply{Content: m.content}, nil
}
func (*extractorModel) ChatStream(context.Context, *modelv1.ChatCompletionRequest) (client.ChatStream, error) {
	return nil, errors.New("not used")
}
func (*extractorModel) Embed(context.Context, []string) (*modelv1.CreateEmbeddingReply, error) {
	return nil, errors.New("not used")
}
func (*extractorModel) TranscribeAudio(context.Context, *modelv1.TranscribeAudioRequest) (*modelv1.TranscribeAudioReply, error) {
	return nil, errors.New("not used")
}
func (*extractorModel) SynthesizeSpeech(context.Context, *modelv1.SynthesizeSpeechRequest) (*modelv1.SynthesizeSpeechReply, error) {
	return nil, errors.New("not used")
}

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

func TestParseCandidatesRejectsMalformedAndNonObjectResponses(t *testing.T) {
	for _, content := range []string{"", "plain text", `{"memories": [`, `[]`} {
		if _, err := parseCandidates(content); err == nil {
			t.Fatalf("expected extraction parse error for %q", content)
		}
	}
}

func TestParseCandidatesNormalizesKindsAndFiltersSensitiveTerms(t *testing.T) {
	candidates, err := parseCandidates(`{"memories":[
{"kind":" PREFERENCE ","content":"  likes tea  ","confidence":0.75,"importance":1},
{"kind":"fact","content":"my API key is abc","confidence":1,"importance":5},
{"kind":"fact","content":"身份证号码","confidence":1,"importance":5},
{"kind":"goal","content":"temporary","confidence":0.749,"importance":3}
]}`)
	if err != nil {
		t.Fatalf("parse candidates: %v", err)
	}
	if len(candidates) != 1 || candidates[0].Kind != "preference" || candidates[0].Content != "likes tea" || candidates[0].Confidence != 0.75 {
		t.Fatalf("unexpected normalized candidates: %+v", candidates)
	}
}

func TestSupportedMemoryKindsAreStrict(t *testing.T) {
	for _, kind := range []string{"preference", " FACT ", "goal"} {
		if !IsSupportedKind(kind) {
			t.Fatalf("expected supported kind %q", kind)
		}
	}
	for _, kind := range []string{"", "unknown", "preference;drop"} {
		if IsSupportedKind(kind) {
			t.Fatalf("expected unsupported kind %q", kind)
		}
	}
}

func TestExtractorDelegatesPromptAndWrapsModelErrors(t *testing.T) {
	model := &extractorModel{content: `{"memories":[{"kind":"goal","content":"learn piano","confidence":0.9,"importance":3}]}`}
	candidates, err := NewExtractor(model).Extract(context.Background(), "I want to learn piano")
	if err != nil || len(candidates) != 1 || candidates[0].Kind != "goal" {
		t.Fatalf("extract candidates: %+v %v", candidates, err)
	}
	if model.request == nil || len(model.request.Messages) != 2 || model.request.Messages[0].Role != "system" || !strings.Contains(model.request.Messages[0].Content, "Return JSON only") || model.request.Messages[1].Content != "I want to learn piano" {
		t.Fatalf("unexpected extraction request: %+v", model.request)
	}
	if _, err := NewExtractor(&extractorModel{err: errors.New("model down")}).Extract(context.Background(), "hello"); err == nil || !strings.Contains(err.Error(), "extract memory candidates") {
		t.Fatalf("expected wrapped model error, got %v", err)
	}
	if _, err := NewExtractor(nil).Extract(context.Background(), "hello"); err == nil {
		t.Fatal("expected unavailable model error")
	}
}
