package plugin

// PageLimit resolves a list endpoint's ?limit= query parameter into the number
// of rows to actually fetch: absent or non-positive means def, anything above
// maxLimit is CLAMPED DOWN to maxLimit, and anything in between is honoured.
//
// The clamp is the point. The obvious spelling —
//
//	limit := 50
//	if input.Limit > 0 && input.Limit <= 500 {
//		limit = input.Limit
//	}
//
// silently falls back to the DEFAULT when the caller asks for more than the
// maximum, so ?limit=1000 returns 50 rows rather than 500. To a paginating
// client that is indistinguishable from the server having only 50 rows: it
// reads the short page as the end of the collection and stops, and the
// remaining rows are simply never seen. Asking for too much must give the
// caller as much as the endpoint allows, never less than it would have
// returned had the parameter been omitted.
//
// A maxLimit below def is treated as the real ceiling, so a misconfigured pair
// can never hand out more rows than the endpoint permits.
func PageLimit(requested, def, maxLimit int) int {
	if maxLimit < def {
		def = maxLimit
	}
	if requested <= 0 {
		return def
	}
	if requested > maxLimit {
		return maxLimit
	}
	return requested
}

// PageOffset resolves a list endpoint's ?offset= query parameter: a negative or
// absent value means 0.
func PageOffset(requested int) int {
	if requested < 0 {
		return 0
	}
	return requested
}
