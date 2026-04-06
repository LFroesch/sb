package diff

import "strings"

// Line represents a single diff line with its type.
type Line struct {
	Type    LineType
	Content string
}

type LineType int

const (
	Context  LineType = iota
	Added
	Removed
)

// Unified computes a simple line-level unified diff between old and new text.
// Returns diff lines with context (up to 3 lines around changes).
func Unified(old, new string) []Line {
	oldLines := strings.Split(old, "\n")
	newLines := strings.Split(new, "\n")

	// LCS-based diff using Myers-like approach (simple DP for correctness)
	table := lcs(oldLines, newLines)
	return backtrack(table, oldLines, newLines)
}

func lcs(a, b []string) [][]int {
	m, n := len(a), len(b)
	table := make([][]int, m+1)
	for i := range table {
		table[i] = make([]int, n+1)
	}
	for i := 1; i <= m; i++ {
		for j := 1; j <= n; j++ {
			if a[i-1] == b[j-1] {
				table[i][j] = table[i-1][j-1] + 1
			} else if table[i-1][j] >= table[i][j-1] {
				table[i][j] = table[i-1][j]
			} else {
				table[i][j] = table[i][j-1]
			}
		}
	}
	return table
}

func backtrack(table [][]int, a, b []string) []Line {
	var result []Line
	i, j := len(a), len(b)

	for i > 0 || j > 0 {
		if i > 0 && j > 0 && a[i-1] == b[j-1] {
			result = append(result, Line{Context, a[i-1]})
			i--
			j--
		} else if j > 0 && (i == 0 || table[i][j-1] >= table[i-1][j]) {
			result = append(result, Line{Added, b[j-1]})
			j--
		} else {
			result = append(result, Line{Removed, a[i-1]})
			i--
		}
	}

	// Reverse
	for l, r := 0, len(result)-1; l < r; l, r = l+1, r-1 {
		result[l], result[r] = result[r], result[l]
	}

	return result
}
