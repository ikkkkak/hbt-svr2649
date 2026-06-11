package ai

import (
	"context"
	"fmt"
	"strings"
	"time"

	"apartments-clone-server/MeskenyGPT/internal/ai/client"
	"apartments-clone-server/MeskenyGPT/internal/ai/lang"
	"apartments-clone-server/MeskenyGPT/internal/ai/response"
)

func (s *service) agentRunBrokerPortfolio(
	ctx context.Context,
	in AgentRunInput,
	emit func(AgentEvent),
	runID string,
	runStart time.Time,
	msgCtx lang.MessageContext,
	tier string,
) (ChatOutput, error) {
	if !isBrokerProTier(tier) || in.UserID == 0 {
		msg := response.Message{
			ID:      fmt.Sprintf("msg_%d", time.Now().UnixNano()),
			Role:    "assistant",
			Content: brokerProLockedMessage(msgCtx.Lang),
		}
		emitVerification(emit, runID, msgCtx, in.Text, true, 0.9,
			[]string{"Broker Pro required"}, nil)
		out := ChatOutput{Message: msg, SessionID: in.SessionID}
		emitFinal(emit, runID, out, runStart, nil, msgCtx.Lang)
		return out, nil
	}

	tG := time.Now()
	emit(AgentEvent{Type: AgentEventStepStart, RunID: runID, StepID: "gather", Label: stepLabel(msgCtx.Lang, "gather")})
	emit(AgentEvent{Type: AgentEventToolCall, RunID: runID, Tool: "portfolio_summary", Args: map[string]any{"user_id": in.UserID}})

	summary, err := loadHostPortfolioSummary(s.gdb, in.UserID)
	emit(AgentEvent{
		Type: AgentEventStepDone, RunID: runID, StepID: "gather",
		MS: time.Since(tG).Milliseconds(),
		Detail: map[string]any{"ok": err == nil},
	})

	content := formatPortfolioSummary(msgCtx.Lang, summary, err)
	msg := response.Message{
		ID: fmt.Sprintf("msg_%d", time.Now().UnixNano()), Role: "assistant", Content: content,
	}
	emitVerification(emit, runID, msgCtx, in.Text, true, 0.9,
		[]string{"Data from your Meskeny listings only"}, nil)
	out := ChatOutput{Message: msg, SessionID: in.SessionID, QuickReplies: response.GenerateQuickReplies(msgCtx, content)}
	emitFinal(emit, runID, out, runStart, nil, msgCtx.Lang)
	return out, nil
}

func formatPortfolioSummary(l lang.Lang, s *HostPortfolioSummary, err error) string {
	if err != nil || s == nil {
		switch l {
		case lang.LangAR:
			return "تعذر تحميل ملخص محفظتك الآن. حاول مرة أخرى بعد لحظات."
		case lang.LangZH:
			return "暂时无法加载您的房源组合数据，请稍后再试。"
		case lang.LangEN:
			return "I couldn't load your portfolio summary right now. Please try again shortly."
		default:
			return "Impossible de charger le résumé de votre portefeuille pour le moment."
		}
	}
	var b strings.Builder
	switch l {
	case lang.LangAR:
		fmt.Fprintf(&b, "ملخص محفظتك على Meskeny:\n• إعلانات نشطة: %d\n• إجمالي المشاهدات (عقارات البيع): %d\n", s.ActiveListings, s.TotalViews)
	case lang.LangZH:
		fmt.Fprintf(&b, "您的 Meskeny 房源概览：\n• 活跃 listings：%d\n• 出售房源总浏览量：%d\n", s.ActiveListings, s.TotalViews)
	case lang.LangEN:
		fmt.Fprintf(&b, "Your Meskeny portfolio snapshot:\n• Active listings: %d\n• Sale listing views (tracked): %d\n", s.ActiveListings, s.TotalViews)
	default:
		fmt.Fprintf(&b, "Aperçu de votre portefeuille Meskeny :\n• Annonces actives : %d\n• Vues (ventes suivies) : %d\n", s.ActiveListings, s.TotalViews)
	}
	if len(s.TopListings) > 0 {
		b.WriteString("\nTop performers:\n")
		for i, line := range s.TopListings {
			fmt.Fprintf(&b, "%d. %s\n", i+1, line)
		}
	}
	return strings.TrimSpace(b.String())
}

func (s *service) agentRunBrokerMarketing(
	ctx context.Context,
	in AgentRunInput,
	emit func(AgentEvent),
	runID string,
	runStart time.Time,
	msgCtx lang.MessageContext,
	tier string,
) (ChatOutput, error) {
	if !isBrokerProTier(tier) {
		msg := response.Message{
			ID:      fmt.Sprintf("msg_%d", time.Now().UnixNano()),
			Role:    "assistant",
			Content: brokerProLockedMessage(msgCtx.Lang),
		}
		emitVerification(emit, runID, msgCtx, in.Text, true, 0.9, nil, nil)
		out := ChatOutput{Message: msg, SessionID: in.SessionID}
		emitFinal(emit, runID, out, runStart, nil, msgCtx.Lang)
		return out, nil
	}

	tG := time.Now()
	emit(AgentEvent{Type: AgentEventStepStart, RunID: runID, StepID: "gather", Label: stepLabel(msgCtx.Lang, "gather")})
	emit(AgentEvent{Type: AgentEventToolCall, RunID: runID, Tool: "marketing_pack", Args: map[string]any{"locales": []string{"zh", "fr", "en"}}})

	tA := time.Now()
	emit(AgentEvent{Type: AgentEventStepStart, RunID: runID, StepID: "analyze", Label: stepLabel(msgCtx.Lang, "analyze")})

	content := s.generateMarketingPack(ctx, msgCtx, in.Text)
	emit(AgentEvent{
		Type: AgentEventStepDone, RunID: runID, StepID: "gather",
		MS: time.Since(tG).Milliseconds(),
	})
	emit(AgentEvent{
		Type: AgentEventStepDone, RunID: runID, StepID: "analyze",
		MS: time.Since(tA).Milliseconds(),
	})

	assumptions := []string{
		"Marketing copy is draft only — verify prices and legal papers on the listing",
		"Not legal advice; titre foncier must be confirmed with authorities",
	}
	if msgCtx.Lang == lang.LangZH || containsHan(in.Text) {
		assumptions = append(assumptions, "Includes Simplified Chinese section for investor outreach")
	}
	emitVerification(emit, runID, msgCtx, in.Text, true, 0.85, assumptions, nil)

	msg := response.Message{
		ID: fmt.Sprintf("msg_%d", time.Now().UnixNano()), Role: "assistant", Content: content,
	}
	out := ChatOutput{Message: msg, SessionID: in.SessionID}
	emitFinal(emit, runID, out, runStart, nil, msgCtx.Lang)
	return out, nil
}

func (s *service) generateMarketingPack(ctx context.Context, msgCtx lang.MessageContext, userText string) string {
	sys := `You are MeskenyGPT Broker Pro. Generate a structured marketing pack for a Mauritania real-estate listing.
Output exactly these sections with clear headers:
## 中文 (Simplified Chinese)
Short investor-facing summary (title hook, location, price in MRU, 3 bullet highlights). Cultural tone for Chinese buyers; no false claims.
## Français
Same structure for local staff.
## English
Same structure for international buyers.
Rules:
- Use only facts the user provided; if missing price/city, say [à compléter / to complete].
- Mention paper_types / verification only as "confirm with authorities" — never claim government registry approval.
- No markdown tables with | pipes.
- Keep each section under 120 words.`

	msgs := []client.Message{
		{Role: "system", Content: sys},
		{Role: "user", Content: userText},
	}
	ctx2, cancel := context.WithTimeout(ctx, time.Duration(s.cfg.TimeoutSeconds)*time.Second)
	defer cancel()
	resp, err := s.or.Chat(ctx2, msgs)
	if err != nil || strings.TrimSpace(resp.Content) == "" {
		switch msgCtx.Lang {
		case lang.LangZH:
			return "暂时无法生成营销文案，请稍后再试。"
		case lang.LangAR:
			return "تعذر إنشاء حزمة التسويق الآن."
		default:
			return "Could not generate the marketing pack right now. Please try again."
		}
	}
	return enforceMeskenyIdentity(msgCtx, resp.Content)
}
