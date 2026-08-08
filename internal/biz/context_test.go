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
	if len(first) != 2 || !strings.Contains(first[0].Content, "第一次见面") {
		t.Fatalf("expected first meeting guidance, got %+v", first)
	}
	ongoing := BuildChatMessages([]data.MessageModel{{Role: "user", Content: "hello"}}, nil, "how are you")
	if len(ongoing) != 3 || strings.Contains(ongoing[0].Content, "第一次见面") {
		t.Fatalf("unexpected ongoing conversation guidance, got %+v", ongoing)
	}
}
