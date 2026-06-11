package lang

// Lang represents the detected language of a user message.
type Lang int

// Intent represents the high-level intent of a message.
type Intent int

const (
	LangFR Lang = iota
	LangEN
	LangAR
	LangZH // Simplified/traditional Chinese script in message
)

const (
	IntentUnknown Intent = iota

	// Property search intents
	IntentSearchRent
	IntentSearchBuy
	IntentSearchAny
	IntentSearchLand
	IntentSearchCommercial
	IntentSearchByBudget
	IntentSearchByLocation
	IntentSearchByRooms
	IntentSearchByType

	// Conversational / info
	IntentGreeting
	IntentHelp
)

// MessageContext is the structured understanding of a single message.
type MessageContext struct {
	Lang   Lang
	Intent Intent

	City   string
	Zone   string
	Quartier string
	Type   string
	PlotNumber string

	// Budget: raw string for display; MRU values for DB search
	Budget    string // legacy / display
	BudgetMRU int64  // canonical MRU
	BudgetMin int64  // search range low
	BudgetMax int64  // search range high

	// RawText is the original user message (set by the AI service for RAG keyword matching).
	RawText string `json:"-"`

	// FromPicker is true when [MESKENY_PICKER] filters were submitted — do not re-ask city/zone.
	FromPicker bool `json:"-"`
}

