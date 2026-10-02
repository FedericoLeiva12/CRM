package domain

import (
	"slices"
	"strings"
	"testing"
)

func TestParseMentionHandles(t *testing.T) {
	cases := []struct {
		name string
		body string
		want []string
	}{
		{"single", "hello @ada", []string{"ada"}},
		{"start of text", "@ada please look", []string{"ada"}},
		{"case folded and deduplicated in order", "@Bob then @ada then @BOB again", []string{"bob", "ada"}},
		{"punctuation ends the handle", "thanks @ada, @bob. and (@cy)!", []string{"ada", "bob", "cy"}},
		{"hyphen and underscore are part of a handle", "cc @nia-cole-2 @scout_bot", []string{"nia-cole-2", "scout_bot"}},
		{"trailing hyphen is trimmed", "ping @ada- now", []string{"ada"}},
		{"email addresses are not mentions", "write to nia@example.test or a.b+c@host.io", []string{}},
		{"double at is not a mention", "@@ada and foo@@bar", []string{}},
		{"inline code is ignored", "use `@ada` literally but @bob for real", []string{"bob"}},
		{"fenced code is ignored", "```\n@ada\n```\n@bob", []string{"bob"}},
		{"unterminated fence ignores the rest", "@cy ```\n@ada", []string{"cy"}},
		{"newline separated", "line one\n@ada\n@bob", []string{"ada", "bob"}},
		{"bare at sign", "meet @ noon", []string{}},
		{"path-like text is ignored", "see /home/@ada and https://x.test/@bob", []string{}},
		{"long handles are cut at 32 characters", "@" + strings.Repeat("a", 40), []string{strings.Repeat("a", 32)}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			got := ParseMentionHandles(test.body)
			if !slices.Equal(got, test.want) {
				t.Fatalf("ParseMentionHandles(%q) = %v, want %v", test.body, got, test.want)
			}
		})
	}
}

func TestValidateCommentBody(t *testing.T) {
	body, err := ValidateCommentBody("  hello\nworld  ")
	if err != nil || body != "hello\nworld" {
		t.Fatalf("body = %q, %v", body, err)
	}
	for name, input := range map[string]string{
		"empty":      "",
		"whitespace": " \n\t ",
		"too long":   strings.Repeat("x", MaxCommentRunes+1),
		"nul":        "a\x00b",
	} {
		if _, err = ValidateCommentBody(input); err == nil || !IsValidationError(err) {
			t.Fatalf("%s was accepted: %v", name, err)
		}
	}
	if _, err = ValidateCommentBody(strings.Repeat("é", MaxCommentRunes)); err != nil {
		t.Fatalf("the limit counts characters, not bytes: %v", err)
	}
}

func TestParseMentionRef(t *testing.T) {
	for raw, want := range map[string]MentionRef{
		"abc123":       {ID: "abc123"},
		" user:abc ":   {Kind: "user", ID: "abc"},
		"agent:xyz789": {Kind: "agent", ID: "xyz789"},
	} {
		got, err := ParseMentionRef(raw)
		if err != nil || got != want {
			t.Fatalf("ParseMentionRef(%q) = %+v, %v; want %+v", raw, got, err, want)
		}
	}
	for _, raw := range []string{"", "  ", "group:abc", "user:", "has space", strings.Repeat("a", 81)} {
		if _, err := ParseMentionRef(raw); err == nil {
			t.Fatalf("ParseMentionRef(%q) was accepted", raw)
		}
	}
}

func TestCommentMentionedIsSubscribable(t *testing.T) {
	if !Subscribable(EventCommentMentioned) || EventCommentMentioned != "comment.mentioned" {
		t.Fatal("comment.mentioned must be selectable on a webhook endpoint")
	}
	types, err := NormalizeEventTypes([]string{EventCommentMentioned, EventTimelineEntryCreated})
	if err != nil || !slices.Equal(types, []string{EventCommentMentioned, EventTimelineEntryCreated}) {
		t.Fatalf("normalized = %v %v", types, err)
	}
}

func TestReservedSectionIdentifier(t *testing.T) {
	if err := ValidateSection("mentions", "Mentions"); err == nil {
		t.Fatal("a section named mentions would shadow the mentions_* tools")
	}
	if err := ValidateSection("deals", "Deals"); err != nil {
		t.Fatal(err)
	}
}
