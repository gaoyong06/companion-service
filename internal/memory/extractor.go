package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	companionclient "companion-service/internal/client"
	"companion-service/internal/lexicon"
	modelv1 "model-gateway/api/model_gateway/v1"
)

type Candidate struct {
	Kind       string  `json:"kind"`
	Content    string  `json:"content"`
	Confidence float64 `json:"confidence"`
	Importance int32   `json:"importance"`
}

type extractionResponse struct {
	Memories []Candidate `json:"memories"`
}

type Extractor struct {
	models companionclient.ModelGateway
}

func NewExtractor(models companionclient.ModelGateway) *Extractor {
	return &Extractor{models: models}
}

func (e *Extractor) Extract(ctx context.Context, content string) ([]Candidate, error) {
	if e == nil || e.models == nil {
		return nil, fmt.Errorf("memory model client is unavailable")
	}
	catalog := lexicon.ForLocale(lexicon.LocaleFromContext(ctx))
	response, err := e.models.Chat(ctx, &modelv1.ChatCompletionRequest{Messages: []*modelv1.ChatMessage{
		{Role: "system", Content: catalog.Prompts.MemoryExtraction},
		{Role: "user", Content: content},
	}})
	if err != nil {
		return nil, fmt.Errorf("extract memory candidates: %w", err)
	}
	return parseCandidates(response.Content)
}

func parseCandidates(content string) ([]Candidate, error) {
	catalog := lexicon.ForLocale(string(lexicon.DefaultLocale))
	start := strings.Index(content, "{")
	end := strings.LastIndex(content, "}")
	if start < 0 || end <= start {
		return nil, fmt.Errorf("memory extraction response is not JSON")
	}
	var response extractionResponse
	if err := json.Unmarshal([]byte(content[start:end+1]), &response); err != nil {
		return nil, fmt.Errorf("decode memory extraction response: %w", err)
	}
	result := make([]Candidate, 0, len(response.Memories))
	for _, candidate := range response.Memories {
		candidate.Kind = strings.TrimSpace(strings.ToLower(candidate.Kind))
		candidate.Content = strings.TrimSpace(candidate.Content)
		if !validKind(candidate.Kind) || candidate.Content == "" || len(candidate.Content) > 512 || containsSensitiveTerm(candidate.Content, catalog.Memory.SensitiveTerms) {
			continue
		}
		if candidate.Confidence < 0.75 {
			continue
		}
		if candidate.Confidence > 1 {
			candidate.Confidence = 1
		}
		if candidate.Importance < 1 {
			candidate.Importance = 1
		}
		if candidate.Importance > 5 {
			candidate.Importance = 5
		}
		result = append(result, candidate)
		if len(result) == 5 {
			break
		}
	}
	return result, nil
}

func validKind(kind string) bool {
	return kind == "preference" || kind == "fact" || kind == "goal"
}

func IsSupportedKind(kind string) bool {
	return validKind(strings.TrimSpace(strings.ToLower(kind)))
}

func containsSensitiveTerm(content string, terms []string) bool {
	lower := strings.ToLower(content)
	for _, term := range terms {
		if strings.Contains(lower, term) {
			return true
		}
	}
	return false
}
