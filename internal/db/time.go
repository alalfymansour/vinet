package db

// TimeFilters maps user-facing ranges to SQLite datetime modifiers, used in
// queries like datetime('now', ?).
var TimeFilters = map[string]string{
	"today": "start of day",
	"24h":   "-1 day",
	"week":  "-7 days",
	"7d":    "-7 days",
	"month": "start of month",
	"30d":   "-30 days",
}

// TimeLabels maps the same ranges to human-facing descriptions.
var TimeLabels = map[string]string{
	"today": "today",
	"24h":   "last 24 hours",
	"week":  "this week",
	"7d":    "last 7 days",
	"month": "this month",
	"30d":   "last 30 days",
}
