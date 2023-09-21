package lib

import (
	"github.com/gosimple/slug"
)

func Slugify(text string) string {
	return slug.Make(text)
}
