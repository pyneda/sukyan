package http_utils

import (
	"testing"

	"github.com/rs/zerolog/log"
)

func TestGetMimeTypeFromURL(t *testing.T) {
	checker := MimeTypeChecker{}
	mt1 := checker.GetMimeTypeFromURL("https:/test.com/script.js")
	if mt1.Code != "javascript" {
		log.Error().Str("url", "https:/test.com/script.js").Interface("ct", mt1).Msg("Error")
		t.Error()
	}
	mt2 := checker.GetMimeTypeFromURL("https:/test.com/test.htm")
	if mt2.Code != "html" {
		t.Error()
	}
	mt3 := checker.GetMimeTypeFromURL("https:/test.com/test.doc")
	if mt3.Code != "doc" {
		t.Error()
	}
}

func TestGetMimeTypeFromContentTypeString(t *testing.T) {
	checker := MimeTypeChecker{}
	mt1 := checker.GetMimeTypeFromContentTypeString("text/html;charset=utf-8")
	if mt1.Code != "html" {
		log.Error().Interface("ct", mt1).Msg("Error")
		t.Error()
	}
	mt2 := checker.GetMimeTypeFromContentTypeString("text/plain;charset=iso-8859-1")
	if mt2.Code != "plain" {
		log.Error().Interface("ct", mt2).Msg("Error")
		t.Error()
	}

	if "html" != checker.GetMimeTypeFromContentTypeString("text/html").Code {
		t.Error()
	}
	if "jpg" != checker.GetMimeTypeFromContentTypeString("image/jpeg").Code {
		t.Error()
	}
	if "json" != checker.GetMimeTypeFromContentTypeString("application/json; charset=utf-8").Code {
		t.Error()
	}
	if "css" != checker.GetMimeTypeFromContentTypeString("text/css; charset=utf-8").Code {
		t.Error()
	}
	if "webp" != checker.GetMimeTypeFromContentTypeString("image/webp").Code {
		t.Error()
	}

}

func TestGetMimeTypeFromContentTypeStringUnknown(t *testing.T) {
	checker := MimeTypeChecker{}
	if "image" != checker.GetMimeTypeFromContentTypeString("image/aksdjfaskdfj").Group {
		t.Error()
	}
	if "application" != checker.GetMimeTypeFromContentTypeString("application/does-not-exist").Group {
		t.Error()
	}
	if "text" != checker.GetMimeTypeFromContentTypeString("text/does-not-exist").Group {
		t.Error()
	}
}
