package biz

import (
	"strings"
	"testing"

	"companion-service/internal/data"
)

func TestBuildChatMessagesKeepsNewestHistoryWithinBudget(t *testing.T) {
	history := []data.MessageModel{
		{Role: "user", Content: strings.Repeat("old", 4000)},
		{Role: "assistant", Content: "recent answer"},
	}
	messages := BuildChatMessages(history, nil, "current question")

	if len(messages) != 4 {
		t.Fatalf("expected system prompt and both history messages, got %d", len(messages))
	}
	if messages[0].Role != "system" || messages[1].Content != history[0].Content || messages[2].Content != history[1].Content || messages[3].Content != "current question" {
		t.Fatalf("history order is invalid: %+v", messages)
	}
}

func TestBuildChatMessagesDropsOldHistoryWhenBudgetIsExceeded(t *testing.T) {
	history := []data.MessageModel{
		{Role: "user", Content: strings.Repeat("old", 9000)},
		{Role: "assistant", Content: "recent answer"},
	}
	messages := BuildChatMessages(history, nil, "current question")

	if len(messages) != 3 {
		t.Fatalf("expected newest history and current input, got %d", len(messages))
	}
	if messages[0].Role != "system" || messages[1].Content != "recent answer" || messages[2].Content != "current question" {
		t.Fatalf("unexpected messages: %+v", messages)
	}
}

func TestBuildChatMessagesIncludesMemoryAsSystemContext(t *testing.T) {
	messages := BuildChatMessages(nil, []data.MemoryModel{{Kind: "preference", Content: "likes jazz"}}, "current question")
	if len(messages) != 3 {
		t.Fatalf("expected memory and current input, got %d", len(messages))
	}
	if messages[0].Role != "system" || !strings.Contains(messages[1].Content, "likes jazz") {
		t.Fatalf("unexpected memory context: %+v", messages)
	}
}

func TestBuildChatMessagesCanBuildSummaryHistoryWithoutEmptyInput(t *testing.T) {
	messages := BuildChatMessages([]data.MessageModel{{Role: "user", Content: "hello"}}, nil, "")
	if len(messages) != 2 || messages[0].Role != "system" || messages[1].Role != "user" || messages[1].Content != "hello" {
		t.Fatalf("unexpected summary messages: %+v", messages)
	}
}

func TestBuildChatMessagesAddsFirstMeetingGuidanceOnlyForNewConversation(t *testing.T) {
	first := BuildChatMessages(nil, nil, "hello")
	if len(first) != 2 || !strings.Contains(first[0].Content, "我叫娜娜") || !strings.Contains(first[0].Content, "第一次见面") {
		t.Fatalf("expected first meeting guidance, got %+v", first)
	}
	ongoing := BuildChatMessages([]data.MessageModel{{Role: "user", Content: "hello"}}, nil, "how are you")
	if len(ongoing) != 3 || strings.Contains(ongoing[0].Content, "第一次见面") {
		t.Fatalf("unexpected ongoing conversation guidance, got %+v", ongoing)
	}
}

func TestBuildChatMessagesForLocaleUsesLocalizedCatalog(t *testing.T) {
	messages := BuildChatMessagesForLocale(nil, nil, "hello", "en-US")
	if len(messages) != 2 || messages[0].Role != "system" {
		t.Fatalf("unexpected English first-meeting context: %+v", messages)
	}
	if !strings.Contains(messages[0].Content, "My name is Nana") || !strings.Contains(messages[0].Content, "This is your first meeting") {
		t.Fatalf("expected localized English prompts, got %q", messages[0].Content)
	}
}

func TestBuildChatMessagesUsesOnboardingStagePrompt(t *testing.T) {
	for _, tc := range []struct {
		name  string
		stage string
		want  string
	}{
		{name: "first meeting", stage: OnboardingStageFirstMeeting, want: "第一次见面"},
		{name: "small talk", stage: OnboardingStageSmallTalk, want: "闲聊"},
		{name: "getting to know", stage: OnboardingStageGettingToKnow, want: "了解"},
		{name: "trust", stage: OnboardingStageTrust, want: "建立信任"},
		{name: "established", stage: OnboardingStageEstablished, want: "破冰"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			messages := BuildChatMessagesForLocaleWithStage(nil, nil, "", "hello", nil, tc.stage, "zh-CN")
			if len(messages) != 2 || !strings.Contains(messages[0].Content, tc.want) {
				t.Fatalf("stage %q did not select expected prompt %q: %+v", tc.stage, tc.want, messages)
			}
		})
	}
}

func TestBuildChatMessagesSkipsInvalidHistoryAndEmptyMemory(t *testing.T) {
	messages := BuildChatMessages([]data.MessageModel{
		{Role: "", Content: "ignored"},
		{Role: "user", Content: "kept"},
	}, []data.MemoryModel{{Kind: "fact", Content: "  "}, {Kind: "goal", Content: "ship MVP"}}, "")
	if len(messages) != 3 || messages[1].Role != "system" || !strings.Contains(messages[1].Content, "ship MVP") || messages[2].Content != "kept" {
		t.Fatalf("unexpected filtered context: %+v", messages)
	}
}

func TestBuildChatMessagesDropsOversizedMemoryButKeepsCurrentInput(t *testing.T) {
	memories := []data.MemoryModel{{Kind: "fact", Content: strings.Repeat("x", maxContextCharacters)}}
	messages := BuildChatMessages(nil, memories, "current")
	if len(messages) != 2 || messages[len(messages)-1].Content != "current" {
		t.Fatalf("expected oversized memory to be dropped: %+v", messages)
	}
}
