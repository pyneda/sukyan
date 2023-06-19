package web

import (
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
	"github.com/rs/zerolog/log"
)

// IgnoreCertificateErrors tells the browser to ignore certificate errors
func IgnoreCertificateErrors(p *rod.Page) {
	ignoreCertsError := proto.SecuritySetIgnoreCertificateErrors{Ignore: true}.Call(p)
	if ignoreCertsError != nil {
		log.Error().Err(ignoreCertsError).Msg("Could not handle SecuritySetIgnoreCertificateErrors")
	} else {
		log.Debug().Msg("Handled SecuritySetIgnoreCertificateErrors")
	}
}
