package capture

import (
	"fmt"
	"strings"

	"apartments-clone-server/MeskenyGPT/internal/ai/lang"
)

// TurnPath identifies which handler answered the user.
type TurnPath string

const (
	TurnPathBlocked   TurnPath = "blocked"
	TurnPathClarify   TurnPath = "clarify"
	TurnPathGreeting  TurnPath = "greeting"
	TurnPathDBSearch  TurnPath = "db_search"
	TurnPathNoResults TurnPath = "no_results"
	TurnPathFollowUp  TurnPath = "follow_up"
	TurnPathLLM       TurnPath = "llm"
)

// ParsedContext is the structured understanding logged for each turn.
type ParsedContext struct {
	Lang     string
	Intent   string
	Type     string
	City     string
	Zone     string
	Quartier string
	Budget   string
	Purpose  string
}

// TurnLogInput is the full payload written to server stdout.
type TurnLogInput struct {
	Path        TurnPath
	SessionID   string
	UserID      uint
	UserMessage string
	AIResponse  string
	Parsed      ParsedContext
	ResultCount int // -1 when not applicable
	LatencyMS   int64
	ModelUsed   string
}

const maxStdoutResponseLen = 4000

// ParsedFromMessageContext builds a log-friendly view of parsed filters.
func ParsedFromMessageContext(ctx lang.MessageContext) ParsedContext {
	return ParsedContext{
		Lang:     LangLabel(ctx.Lang),
		Intent:   IntentLabel(ctx.Intent),
		Type:     strings.TrimSpace(ctx.Type),
		City:     strings.TrimSpace(ctx.City),
		Zone:     strings.TrimSpace(ctx.Zone),
		Quartier: strings.TrimSpace(ctx.Quartier),
		Budget:   budgetLabel(ctx),
		Purpose:  purposeLabel(ctx),
	}
}

// LogTurnToStdout prints a full user/AI turn for server-side debugging.
func LogTurnToStdout(in TurnLogInput) {
	fmt.Println("────────── MeskenyGPT turn ──────────")
	fmt.Printf("  path=%s session=%s user_id=%d latency_ms=%d\n",
		in.Path, in.SessionID, in.UserID, in.LatencyMS)
	fmt.Printf("  parsed: intent=%s lang=%s type=%q city=%q zone=%q quartier=%q budget=%s purpose=%s\n",
		in.Parsed.Intent, in.Parsed.Lang, in.Parsed.Type,
		in.Parsed.City, in.Parsed.Zone, in.Parsed.Quartier,
		in.Parsed.Budget, in.Parsed.Purpose)
	fmt.Printf("  USER >>>\n%s\n", in.UserMessage)
	fmt.Printf("  AI <<<\n%s\n", truncateLong(in.AIResponse, maxStdoutResponseLen))
	if in.ResultCount >= 0 {
		fmt.Printf("  listings=%d model=%s\n", in.ResultCount, in.ModelUsed)
	}
	fmt.Println("────────────────────────────────────")
}

func LangLabel(l lang.Lang) string {
	switch l {
	case lang.LangAR:
		return "ar"
	case lang.LangEN:
		return "en"
	case lang.LangZH:
		return "zh"
	default:
		return "fr"
	}
}

func IntentLabel(it lang.Intent) string {
	switch it {
	case lang.IntentSearchRent:
		return "search_rent"
	case lang.IntentSearchBuy:
		return "search_buy"
	case lang.IntentSearchAny:
		return "search_any"
	case lang.IntentSearchLand:
		return "search_land"
	case lang.IntentSearchCommercial:
		return "search_commercial"
	case lang.IntentSearchByBudget:
		return "search_by_budget"
	case lang.IntentSearchByLocation:
		return "search_by_location"
	case lang.IntentSearchByRooms:
		return "search_by_rooms"
	case lang.IntentSearchByType:
		return "search_by_type"
	case lang.IntentGreeting:
		return "greeting"
	case lang.IntentHelp:
		return "help"
	case lang.IntentInfoProcedure:
		return "info_procedure"
	default:
		return "unknown"
	}
}

func purposeLabel(ctx lang.MessageContext) string {
	switch ctx.Intent {
	case lang.IntentSearchRent:
		return "rent"
	case lang.IntentSearchBuy, lang.IntentSearchLand, lang.IntentSearchCommercial:
		return "sale"
	default:
		return ""
	}
}

func budgetLabel(ctx lang.MessageContext) string {
	if strings.TrimSpace(ctx.Budget) != "" {
		return strings.TrimSpace(ctx.Budget)
	}
	if ctx.BudgetMin > 0 || ctx.BudgetMax > 0 {
		return fmt.Sprintf("%d-%d MRU", ctx.BudgetMin, ctx.BudgetMax)
	}
	if ctx.BudgetMRU > 0 {
		return fmt.Sprintf("%d MRU", ctx.BudgetMRU)
	}
	return ""
}

func truncateLong(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
