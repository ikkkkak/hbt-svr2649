package rag

import "testing"

func TestIntentMatches_searchAnyCoversUnknown(t *testing.T) {
	if !intentMatches("search_any", "unknown") {
		t.Fatal("search_any scope should match unknown intent (FAQ / general questions)")
	}
	if !intentMatches("search_any", "help") {
		t.Fatal("search_any should match help")
	}
	if !intentMatches("search_any", "search_buy") {
		t.Fatal("search_any should match other search_* intents")
	}
	if intentMatches("search_buy", "unknown") {
		t.Fatal("search_buy alone must not match unknown")
	}
	if !intentMatches("all", "unknown") {
		t.Fatal("all should match unknown")
	}
}
