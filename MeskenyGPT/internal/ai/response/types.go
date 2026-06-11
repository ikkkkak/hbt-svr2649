package response

// Message is the minimal chat message shape shared with the frontend.
type Message struct {
	ID      string `json:"id"`
	Role    string `json:"role"` // "assistant" | "user"
	Content string `json:"content"`
}

// QuickReply represents a chip the user can tap.
type QuickReply struct {
	ID     string `json:"id"`
	Text   string `json:"text"`
	Action string `json:"action"`
}

