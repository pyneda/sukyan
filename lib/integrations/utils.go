package integrations

import (
	"strings"
)

func convertToTitle(s string) string {
	words := strings.FieldsFunc(s, func(r rune) bool {
		return r == '-'
	})

	for i, w := range words {
		words[i] = strings.Title(w)
	}

	return strings.Join(words, " ")
}
