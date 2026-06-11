package agent

import (
	"fmt"
	"strings"

	"apartments-clone-server/MeskenyGPT/internal/ai/lang"
)

// Step is one visible reasoning step in the agent timeline.
type Step struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

// Path describes which orchestration branch runs after understand.
type Path string

const (
	PathGreeting       Path = "greeting"
	PathClarify        Path = "clarify"
	PathSearch         Path = "search"
	PathConversational Path = "conversational"
	PathMarket         Path = "market"
)

func ResolvePath(role string, msgCtx lang.MessageContext, clarify bool) Path {
	if msgCtx.Intent == lang.IntentGreeting || msgCtx.Intent == lang.IntentHelp {
		return PathGreeting
	}
	if clarify {
		return PathClarify
	}
	if role == RoleMarketAnalyst {
		return PathMarket
	}
	if lang.IsPropertySearchIntent(msgCtx.Intent) {
		return PathSearch
	}
	return PathConversational
}

func BuildStepPlan(role string, msgCtx lang.MessageContext, path Path) []Step {
	loc := displayLocation(msgCtx)
	switch path {
	case PathGreeting:
		return []Step{
			planStep("understand", stepText(msgCtx.Lang, "greet_understand")),
			planStep("deliver", stepText(msgCtx.Lang, "greet_deliver")),
		}
	case PathClarify:
		return []Step{
			planStep("understand", stepText(msgCtx.Lang, "clarify_understand")),
			planStep("plan", stepText(msgCtx.Lang, "clarify_plan")),
			planStep("verify", stepText(msgCtx.Lang, "verify")),
			planStep("deliver", stepText(msgCtx.Lang, "deliver")),
		}
	case PathSearch:
		return []Step{
			planStep("understand", stepText(msgCtx.Lang, "search_understand", loc)),
			planStep("plan", stepText(msgCtx.Lang, "search_plan", loc)),
			planStep("gather", stepText(msgCtx.Lang, "search_gather", loc)),
			planStep("analyze", stepText(msgCtx.Lang, "search_analyze")),
			planStep("verify", stepText(msgCtx.Lang, "verify_db")),
			planStep("deliver", stepText(msgCtx.Lang, "deliver_results")),
		}
	case PathMarket:
		return []Step{
			planStep("understand", stepText(msgCtx.Lang, "market_understand", loc)),
			planStep("gather", stepText(msgCtx.Lang, "market_gather", loc)),
			planStep("analyze", stepText(msgCtx.Lang, "market_analyze")),
			planStep("verify", stepText(msgCtx.Lang, "verify")),
			planStep("deliver", stepText(msgCtx.Lang, "deliver")),
		}
	default:
		return []Step{
			planStep("understand", stepText(msgCtx.Lang, "conv_understand")),
			planStep("gather", stepText(msgCtx.Lang, "conv_gather")),
			planStep("analyze", stepText(msgCtx.Lang, "conv_compose")),
			planStep("verify", stepText(msgCtx.Lang, "verify")),
			planStep("deliver", stepText(msgCtx.Lang, "deliver")),
		}
	}
}

func planStep(id, label string) Step { return Step{ID: id, Label: label} }

func displayLocation(ctx lang.MessageContext) string {
	if z := strings.TrimSpace(ctx.Zone); z != "" {
		return z
	}
	if c := strings.TrimSpace(ctx.City); c != "" {
		return c
	}
	return ""
}

func stepText(l lang.Lang, key string, loc ...string) string {
	place := ""
	if len(loc) > 0 {
		place = loc[0]
	}
	switch key {
	case "greet_understand":
		return pick(l, "Detecting language", "Détection de la langue", "تحديد اللغة", "识别语言")
	case "greet_deliver":
		return pick(l, "Preparing welcome", "Préparation de l'accueil", "تجهيز الترحيب", "准备问候")
	case "clarify_understand":
		return pick(l, "Understanding what you need", "Comprendre votre besoin", "فهم طلبك", "理解您的需求")
	case "clarify_plan":
		return pick(l, "Identifying missing filters", "Repérer les critères manquants", "تحديد المعايير الناقصة", "识别缺失条件")
	case "search_understand":
		if place != "" {
			return pick(l,
				fmt.Sprintf("Understanding search in %s", place),
				fmt.Sprintf("Comprendre la recherche à %s", place),
				fmt.Sprintf("فهم البحث في %s", place),
				fmt.Sprintf("理解在 %s 的搜索", place))
		}
		return pick(l, "Understanding your property search", "Comprendre votre recherche", "فهم طلب البحث", "理解找房需求")
	case "search_plan":
		return pick(l, "Planning Meskeny database query", "Planifier la requête Meskeny", "تخطيط استعلام قاعدة البيانات", "规划数据库查询")
	case "search_gather":
		return pick(l, "Searching real Meskeny listings", "Recherche d'annonces réelles", "البحث في إعلانات Meskeny", "搜索 Meskeny 真实房源")
	case "search_analyze":
		return pick(l, "Ranking matches by relevance", "Classement par pertinence", "ترتيب النتائج حسب الملاءمة", "按相关性排序")
	case "verify_db":
		return pick(l, "Confirming results are database-backed", "Vérifier l'origine des annonces", "التأكد أن النتائج من قاعدة البيانات", "确认结果来自数据库")
	case "deliver_results":
		return pick(l, "Delivering property cards", "Présentation des annonces", "عرض بطاقات العقارات", "呈现房源卡片")
	case "market_understand":
		return pick(l, "Understanding market question", "Comprendre la question marché", "فهم سؤال السوق", "理解市场问题")
	case "market_gather":
		return pick(l, "Gathering market context", "Collecte du contexte marché", "جمع بيانات السوق", "收集市场数据")
	case "market_analyze":
		return pick(l, "Analyzing trends and comparables", "Analyse des tendances", "تحليل الاتجاهات", "分析趋势")
	case "conv_understand":
		return pick(l, "Understanding your question", "Comprendre votre question", "فهم سؤالك", "理解您的问题")
	case "conv_gather":
		return pick(l, "Loading Meskeny knowledge", "Chargement du contexte Meskeny", "تحميل معرفة Meskeny", "加载 Meskeny 知识")
	case "conv_compose":
		return pick(l, "Composing answer", "Rédaction de la réponse", "صياغة الإجابة", "撰写回答")
	case "verify":
		return pick(l, "Checking answer matches your question", "Vérifier la pertinence", "التحقق من ملاءمة الإجابة", "核对回答是否匹配")
	case "deliver":
		return pick(l, "Delivering answer", "Présentation de la réponse", "تقديم الإجابة", "呈现回答")
	default:
		return key
	}
}

func pick(l lang.Lang, en, fr, ar, zh string) string {
	switch l {
	case lang.LangAR:
		return ar
	case lang.LangEN:
		return en
	case lang.LangZH:
		return zh
	default:
		return fr
	}
}
