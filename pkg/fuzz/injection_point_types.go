package fuzz

import (
	"github.com/pyneda/sukyan/pkg/payloads"
	"github.com/pyneda/sukyan/pkg/web/cookies"
	"strings"
)

// Interface

type InjectionPoint interface {
	GetTitle() string
	GetWithPayload(payloads.PayloadInterface) string
}

// Definitions

type URLInjectionPoint struct {
	Code          string
	Title         string
	URL           string
	ReplaceValue  string
	OriginalValue string
}

func (i URLInjectionPoint) GetWithPayload(payload payloads.PayloadInterface) string {
	return strings.Replace(i.URL, i.ReplaceValue, payload.GetValue(), 1)
}

func (i URLInjectionPoint) GetTitle() string {
	return i.Title
}

type BodyInjectionPoint struct {
}

type HeaderInjectionPoint struct {
}

type CookieInjectionPoint struct {
	Cookie cookies.Cookie
}
