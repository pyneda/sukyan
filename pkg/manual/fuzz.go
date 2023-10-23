package manual

import (
	"github.com/pyneda/sukyan/db"
)

type RequestFuzzOptions struct {
	URL             string                 `json:"url" validate:"required"`
	Raw             string                 `json:"raw" validate:"required"`
	InsertionPoints []FuzzerInsertionPoint `json:"insertion_points" validate:"required"`
	Session         db.PlaygroundSession   `json:"session" validate:"required"`
	Options         RequestOptions         `json:"options"`
}

type FuzzerPayloadsGroup struct {
	Payloads   []string `json:"payloads"`
	Type       string   `json:"type"`
	Processors []string `json:"processors,omitempty" validate:"omitempty,dive,oneof=base64encode base64decode urlencode urldecode sha1hash sha256hash md5hash" example:"base64encode"`
	Wordlist   string   `json:"wordlist,omitempty"`
}

type FuzzerInsertionPoint struct {
	Start         int                   `json:"start"`
	End           int                   `json:"end"`
	OriginalValue string                `json:"originalValue"`
	PayloadGroups []FuzzerPayloadsGroup `json:"payloadGroups"`
}

func Fuzz(input RequestFuzzOptions, taskID uint) error {
	return nil
}
