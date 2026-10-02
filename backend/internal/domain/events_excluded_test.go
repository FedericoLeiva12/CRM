package domain

import "testing"

func TestActorExcludedFromWebhook(t *testing.T) {
	excluded := map[string]struct{}{"agent:bot": {}, "user:ada": {}}

	if ActorExcludedFromWebhook(EventWebhookTest, "agent", "bot", "", "", excluded) {
		t.Fatal("webhook.test is never filtered")
	}
	if ActorExcludedFromWebhook(EventRecordCreated, "system", "webhooks", "", "", excluded) {
		t.Fatal("system actors are not filtered")
	}
	if !ActorExcludedFromWebhook(EventRecordCreated, "agent", "bot", "", "", excluded) {
		t.Fatal("excluded actor should skip")
	}
	if ActorExcludedFromWebhook(EventRecordCreated, "user", "nia", "", "", excluded) {
		t.Fatal("non-excluded actor should deliver")
	}
	if ActorExcludedFromWebhook(EventCommentMentioned, "user", "nia", "agent", "bot", excluded) {
		t.Fatal("mention of excluded agent should still deliver when author is not excluded")
	}
	if ActorExcludedFromWebhook(EventCommentMentioned, "agent", "bot", "agent", "bot", excluded) {
		t.Fatal("self-mention by excluded agent should still deliver")
	}
	if !ActorExcludedFromWebhook(EventCommentMentioned, "agent", "bot", "user", "nia", excluded) {
		t.Fatal("excluded author mentioning someone else should skip")
	}
}

func TestNormalizeWebhookExcludedActors(t *testing.T) {
	refs, err := NormalizeWebhookExcludedActors([]WebhookActorRef{
		{Kind: "user", ID: "ada"},
		{Kind: "user", ID: "ada"},
		{Kind: "agent", ID: "bot"},
	})
	if err != nil || len(refs) != 2 || refs[0].Kind != "agent" {
		t.Fatalf("normalized = %v %v", refs, err)
	}
	if _, err = NormalizeWebhookExcludedActors([]WebhookActorRef{{Kind: "team", ID: "x"}}); err == nil {
		t.Fatal("rejected invalid kind")
	}
}
