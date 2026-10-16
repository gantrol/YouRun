package engine

import (
	"strings"
	"unicode"
)

func isBoundaryRune(r rune) bool {
	return unicode.IsSpace(r) || strings.ContainsRune("-_./\\:·,;|()[]{}'\"`", r)
}

func isRuneWordStart(runes []rune, index int) bool {
	if index <= 0 {
		return true
	}
	prev := runes[index-1]
	curr := runes[index]
	if isBoundaryRune(prev) && !isBoundaryRune(curr) {
		return true
	}
	return (prev < 128) != (curr < 128)
}

func SearchTerms(query string) []string {
	return strings.Fields(strings.ToLower(strings.TrimSpace(query)))
}

func compactSubsequenceScore(textRunes, termRunes []rune) int {
	if len(termRunes) < 2 || len(termRunes) > len(textRunes) {
		return 0
	}
	var wordInitials []rune
	for i, r := range textRunes {
		if !isBoundaryRune(r) && isRuneWordStart(textRunes, i) {
			wordInitials = append(wordInitials, r)
		}
	}
	if len(wordInitials) >= len(termRunes) {
		initialStr := string(wordInitials)
		termStr := string(termRunes)
		if strings.Contains(initialStr, termStr) {
			return 52
		}
	}

	pos := 0
	first, last := -1, -1
	boundaryOrConsecutive := 0
	for _, tr := range termRunes {
		found := -1
		for j := pos; j < len(textRunes); j++ {
			if textRunes[j] == tr && (j == pos || isRuneWordStart(textRunes, j)) {
				found = j
				break
			}
		}
		if found < 0 {
			for j := pos; j < len(textRunes); j++ {
				if textRunes[j] == tr {
					found = j
					break
				}
			}
		}
		if found < 0 {
			return 0
		}
		if found == pos || isRuneWordStart(textRunes, found) {
			boundaryOrConsecutive++
		}
		if first < 0 {
			first = found
		}
		last = found
		pos = found + 1
	}
	span := last - first + 1
	if boundaryOrConsecutive == len(termRunes) {
		return 46
	}
	if len(termRunes) >= 3 && span <= len(termRunes)*2+1 {
		return 38
	}
	return 0
}

func ScoreField(value, term string, allowSubsequence bool) int {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || term == "" {
		return 0
	}
	lower := strings.ToLower(trimmed)
	if lower == term {
		return 120
	}
	if strings.HasPrefix(lower, term) {
		diff := len(lower) - len(term)
		if diff > 36 {
			diff = 36
		}
		return 102 - diff/2
	}
	idx := strings.Index(lower, term)
	if idx >= 0 {
		prefixRunes := []rune(lower[:idx])
		if len(prefixRunes) == 0 || isBoundaryRune(prefixRunes[len(prefixRunes)-1]) {
			return 86
		}
		penalty := idx / 4
		if penalty > 18 {
			penalty = 18
		}
		return 68 - penalty
	}
	if allowSubsequence {
		return compactSubsequenceScore([]rune(lower), []rune(term))
	}
	return 0
}

type weightedField struct {
	value       string
	weight      int
	subsequence bool
}

func scoreWeightedFields(terms []string, fields []weightedField) int {
	if len(terms) == 0 {
		return 1
	}
	total := 0
	primaryAll := len(fields) > 0
	maxWeight := 0
	for _, f := range fields {
		if f.weight > maxWeight {
			maxWeight = f.weight
		}
	}
	for _, term := range terms {
		best := 0
		matchedPrimary := false
		for _, f := range fields {
			if f.value == "" {
				continue
			}
			raw := ScoreField(f.value, term, f.subsequence)
			if raw <= 0 {
				continue
			}
			weighted := raw * f.weight
			if weighted > best {
				best = weighted
			}
			if f.weight*10 >= maxWeight*8 {
				matchedPrimary = true
			}
		}
		if best <= 0 {
			return 0
		}
		if !matchedPrimary {
			primaryAll = false
		}
		total += best
	}
	if primaryAll && len(terms) > 1 {
		total += 250
	}
	return total
}

func ScoreCommand(cmd Command, query, projectName string, collectionNames ...string) int {
	terms := SearchTerms(query)
	if len(terms) == 0 {
		return 1
	}
	source := ""
	if cmd.Source != nil {
		source = *cmd.Source
	}
	fields := make([]weightedField, 0, 6+len(cmd.Aliases)+len(collectionNames))
	fields = append(fields, weightedField{value: cmd.Title, weight: 40, subsequence: true})
	for _, alias := range cmd.Aliases {
		fields = append(fields, weightedField{value: alias, weight: 36, subsequence: true})
	}
	if projectName != "" {
		fields = append(fields, weightedField{value: projectName, weight: 24, subsequence: true})
	}
	for _, name := range collectionNames {
		fields = append(fields, weightedField{value: name, weight: 22, subsequence: true})
	}
	fields = append(fields,
		weightedField{value: cmd.Text, weight: 20, subsequence: true},
		weightedField{value: source, weight: 13, subsequence: false},
		weightedField{value: cmd.Directory, weight: 10, subsequence: false},
		weightedField{value: cmd.Shell, weight: 8, subsequence: false},
	)
	return scoreWeightedFields(terms, fields)
}

func ScoreProject(proj Project, query, directory string, bestCommandScore int) int {
	terms := SearchTerms(query)
	if len(terms) == 0 {
		return 1
	}
	direct := scoreWeightedFields(terms, []weightedField{
		{value: proj.Name, weight: 42, subsequence: true},
		{value: directory, weight: 14, subsequence: false},
	})
	inherited := (bestCommandScore * 85) / 100
	if direct <= 0 && inherited <= 0 {
		return 0
	}
	best := direct
	if inherited > best {
		best = inherited
	}
	if direct > 0 && inherited > 0 {
		best += 200
	}
	return best
}

func ScoreCollection(col Collection, path, query string, bestCommandScore int) int {
	terms := SearchTerms(query)
	if len(terms) == 0 {
		return 1
	}
	direct := scoreWeightedFields(terms, []weightedField{
		{value: col.Name, weight: 40, subsequence: true},
		{value: path, weight: 28, subsequence: true},
	})
	inherited := (bestCommandScore * 85) / 100
	if inherited > direct {
		return inherited
	}
	return direct
}

func ScoreRun(run Run, query, projectName string, aliases []string, source string) int {
	terms := SearchTerms(query)
	if len(terms) == 0 {
		return 1
	}
	fields := make([]weightedField, 0, 7+len(aliases))
	fields = append(fields,
		weightedField{value: run.Title, weight: 40, subsequence: true},
		weightedField{value: projectName, weight: 30, subsequence: true},
	)
	for _, alias := range aliases {
		fields = append(fields, weightedField{value: alias, weight: 32, subsequence: true})
	}
	fields = append(fields,
		weightedField{value: run.Text, weight: 20, subsequence: true},
		weightedField{value: source, weight: 14, subsequence: false},
		weightedField{value: run.Directory, weight: 12, subsequence: false},
		weightedField{value: run.Shell, weight: 10, subsequence: false},
		weightedField{value: run.Status, weight: 10, subsequence: false},
		weightedField{value: run.ID, weight: 10, subsequence: false},
	)
	return scoreWeightedFields(terms, fields)
}
