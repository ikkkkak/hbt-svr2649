package services

import "testing"

func TestParsePastedItems(t *testing.T) {
	cases := []struct {
		name    string
		payload string
		minItem int
		wantURL string // expected URL in first item (if any)
	}{
		{"array of objects", `[{"title":"Transfer land","description":"Steps…","url":"https://x.mr/1"},{"title":"B","description":"y"}]`, 2, "https://x.mr/1"},
		{"wrapper data array", `{"data":[{"name":"A","content":"aaa"},{"name":"B","content":"bbb"}]}`, 2, ""},
		{"single object", `{"question":"How to register?","answer":"Bring documents…"}`, 1, ""},
		{"array of strings", `["first fact here","second fact here"]`, 2, ""},
		{"raw text", `this is not json at all, just notes`, 1, ""},
		{"object no desc field", `[{"foo":"bar","num":3}]`, 1, ""},
	}
	for _, c := range cases {
		items := parsePastedItems(c.payload)
		if len(items) < c.minItem {
			t.Errorf("%s: got %d items, want >= %d", c.name, len(items), c.minItem)
			continue
		}
		if items[0].Description == "" {
			t.Errorf("%s: first item has empty description", c.name)
		}
		if items[0].Title == "" {
			t.Errorf("%s: first item has empty title", c.name)
		}
		if c.wantURL != "" && items[0].URL != c.wantURL {
			t.Errorf("%s: url = %q, want %q", c.name, items[0].URL, c.wantURL)
		}
	}
}
