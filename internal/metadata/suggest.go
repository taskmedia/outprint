package metadata

const maxSuggestDistance = 2

// suggest returns the name closest to typo within a small edit distance,
// or "" if none is close. The first of equally close names wins.
func suggest(typo string, names []string) string {
	best, bestDistance := "", maxSuggestDistance+1
	for _, name := range names {
		if d := editDistance(typo, name); d < bestDistance {
			best, bestDistance = name, d
		}
	}
	if bestDistance >= len([]rune(typo)) {
		return ""
	}
	return best
}

func editDistance(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	prev := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		cur := make([]int, len(rb)+1)
		cur[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev = cur
	}
	return prev[len(rb)]
}
