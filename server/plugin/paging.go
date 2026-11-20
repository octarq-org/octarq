package plugin

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

func PageOffset(requested int) int {
	if requested < 0 {
		return 0
	}
	return requested
}
