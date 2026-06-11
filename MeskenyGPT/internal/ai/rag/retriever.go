package rag

import (
	"context"

	"apartments-clone-server/MeskenyGPT/internal/ai/lang"
	"apartments-clone-server/MeskenyGPT/internal/ai/prompt"
)

// RAGContext is the structured knowledge we inject into prompts.
type RAGContext struct {
	MarketSummary string   // price ranges, trends
	FAQSnippets   []string // short Q&A snippets from vector store
	Notes         []string // any extra notes (zone profile, etc.)
}

// Retriever defines the interface for fetching knowledge given a message.
type Retriever interface {
	Retrieve(ctx context.Context, msgCtx lang.MessageContext) (RAGContext, error)
}

// noopRetriever is a safe default that returns empty context.
type noopRetriever struct{}

// NewNoopRetriever returns a retriever that currently does nothing.
func NewNoopRetriever() Retriever {
	return &noopRetriever{}
}

func (n *noopRetriever) Retrieve(_ context.Context, _ lang.MessageContext) (RAGContext, error) {
	return RAGContext{}, nil
}

// mauritaniaRetriever always injects Mauritania knowledge into prompts (no vector DB).
type mauritaniaRetriever struct{}

// NewMauritaniaRetriever returns a retriever that injects static Mauritania context.
func NewMauritaniaRetriever() Retriever {
	return &mauritaniaRetriever{}
}

func (m *mauritaniaRetriever) Retrieve(_ context.Context, _ lang.MessageContext) (RAGContext, error) {
	return RAGContext{
		MarketSummary: prompt.MauritaniaContext(),
		Notes:         nil,
	}, nil
}

