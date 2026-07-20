// SPDX-License-Identifier: MIT

package ai

import (
	"context"
	"strings"
	"testing"
)

func TestChatCopilotAskAndHistory(t *testing.T) {
	doc := textDoc(t, "The Eiffel Tower is 330 metres tall and stands in Paris.")
	client := &fakeClient{replies: []string{"330 metres.", "In Paris."}}
	chat := NewChatCopilot(client, ChatOptions{Document: doc})

	a1, err := chat.Ask(context.Background(), "How tall is it?")
	if err != nil {
		t.Fatal(err)
	}
	if a1 != "330 metres." {
		t.Errorf("answer 1 = %q", a1)
	}

	// First request: system(with document) + user question.
	req1 := client.requests[0]
	if len(req1.Messages) != 2 {
		t.Fatalf("request 1 messages = %d; want 2", len(req1.Messages))
	}
	if req1.Messages[0].Role != RoleSystem || !strings.Contains(req1.Messages[0].Text, "Eiffel Tower is 330 metres") {
		t.Errorf("system message missing document context: %q", req1.Messages[0].Text)
	}
	if req1.Messages[1].Role != RoleUser || req1.Messages[1].Text != "How tall is it?" {
		t.Errorf("user message wrong: %+v", req1.Messages[1])
	}

	a2, err := chat.Ask(context.Background(), "Where is it?")
	if err != nil {
		t.Fatal(err)
	}
	if a2 != "In Paris." {
		t.Errorf("answer 2 = %q", a2)
	}

	// Second request carries the prior turn as history: system, user1,
	// assistant1, user2.
	req2 := client.requests[1]
	wantRoles := []string{RoleSystem, RoleUser, RoleAssistant, RoleUser}
	if len(req2.Messages) != len(wantRoles) {
		t.Fatalf("request 2 messages = %d; want %d", len(req2.Messages), len(wantRoles))
	}
	for i, r := range wantRoles {
		if req2.Messages[i].Role != r {
			t.Errorf("request 2 message %d role = %q; want %q", i, req2.Messages[i].Role, r)
		}
	}
	if req2.Messages[2].Text != "330 metres." {
		t.Errorf("history assistant turn = %q", req2.Messages[2].Text)
	}

	// History() reflects both turns.
	h := chat.History()
	if len(h) != 4 {
		t.Fatalf("history len = %d; want 4", len(h))
	}
	if h[0].Text != "How tall is it?" || h[3].Text != "In Paris." {
		t.Errorf("history content wrong: %+v", h)
	}
	// History() is a copy — mutating it must not affect the copilot.
	h[0].Text = "mutated"
	if chat.History()[0].Text != "How tall is it?" {
		t.Error("History() leaked its internal slice")
	}
}

func TestChatCopilotReset(t *testing.T) {
	doc := textDoc(t, "Some content.")
	client := &fakeClient{replies: []string{"a", "b"}}
	chat := NewChatCopilot(client, ChatOptions{Document: doc})

	if _, err := chat.Ask(context.Background(), "q1"); err != nil {
		t.Fatal(err)
	}
	chat.Reset()
	if len(chat.History()) != 0 {
		t.Errorf("history not cleared: %+v", chat.History())
	}
	if _, err := chat.Ask(context.Background(), "q2"); err != nil {
		t.Fatal(err)
	}
	// After reset the second Ask has no prior history: system + user only.
	if n := len(client.requests[1].Messages); n != 2 {
		t.Errorf("post-reset request messages = %d; want 2", n)
	}
}

func TestChatCopilotContextTruncation(t *testing.T) {
	long := strings.Repeat("word ", 5000) // 25000 runes
	client := &fakeClient{replies: []string{"ok"}}
	chat := NewChatCopilot(client, ChatOptions{Texts: []string{long}, MaxContextRunes: 1000})

	if _, err := chat.Ask(context.Background(), "summarize"); err != nil {
		t.Fatal(err)
	}
	sys := client.requests[0].Messages[0].Text
	if !strings.Contains(sys, "too long to include in full") {
		t.Errorf("truncation not signalled in system prompt")
	}
	// The document section must be bounded near the budget (plus framing).
	if n := len([]rune(sys)); n > 1500 {
		t.Errorf("context not truncated: system prompt is %d runes", n)
	}
}

func TestChatCopilotSystemPromptOverride(t *testing.T) {
	client := &fakeClient{replies: []string{"ответ"}}
	chat := NewChatCopilot(client, ChatOptions{
		Texts:        []string{"Договор №5 на сумму 1000 рублей."},
		SystemPrompt: "Ты юридический ассистент. Отвечай по-русски.",
	})
	if _, err := chat.Ask(context.Background(), "Какая сумма?"); err != nil {
		t.Fatal(err)
	}
	sys := client.requests[0].Messages[0].Text
	if !strings.Contains(sys, "юридический ассистент") {
		t.Errorf("custom system prompt not used: %q", sys)
	}
	if !strings.Contains(sys, "Договор") {
		t.Errorf("document context missing under custom prompt")
	}
}

func TestChatCopilotErrors(t *testing.T) {
	// No content source.
	chat := NewChatCopilot(&fakeClient{}, ChatOptions{})
	if _, err := chat.Ask(context.Background(), "q"); err == nil {
		t.Error("missing document must error")
	}
	// Empty question.
	chat = NewChatCopilot(&fakeClient{}, ChatOptions{Texts: []string{"content"}})
	if _, err := chat.Ask(context.Background(), "  "); err == nil {
		t.Error("empty question must error")
	}
}
