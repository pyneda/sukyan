package report

import (
	"encoding/base64"
	"fmt"
	"strings"
	"unicode/utf8"
)

type evidenceText struct {
	Lines        []string
	Truncated    bool
	OmittedBytes int
	NotUTF8      bool
}

func (e evidenceText) Empty() bool { return len(e.Lines) == 0 }

func decodeBase64Field(s string) []byte {
	if s == "" {
		return nil
	}
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return nil
	}
	return b
}

// sanitizeEvidence makes arbitrary captured bytes safe to hand to fpdf, which
// requires valid UTF-8 and would otherwise drop or corrupt them.
func sanitizeEvidence(raw []byte, maxBytes int) evidenceText {
	var out evidenceText

	if maxBytes > 0 && len(raw) > maxBytes {
		cut := maxBytes
		// Back off to a rune boundary so a clean truncation is not reported as corruption.
		for cut > 0 && !utf8.RuneStart(raw[cut]) {
			cut--
		}
		out.Truncated = true
		out.OmittedBytes = len(raw) - cut
		raw = raw[:cut]
	}

	var b strings.Builder
	b.Grow(len(raw))

	for i := 0; i < len(raw); {
		r, size := utf8.DecodeRune(raw[i:])
		if r == utf8.RuneError && size <= 1 {
			out.NotUTF8 = true
			b.WriteRune('·')
			i++
			continue
		}

		switch {
		case r == '\n' || r == '\t':
			b.WriteRune(r)
		case r == '\r':
		case r < 0x20 || r == 0x7f:
			b.WriteRune('·')
		default:
			b.WriteRune(r)
		}
		i += size
	}

	s := strings.TrimSuffix(b.String(), "\n")
	if s == "" {
		return out
	}
	out.Lines = strings.Split(s, "\n")
	return out
}

func hexDump(raw []byte, maxBytes int) []string {
	if maxBytes > 0 && len(raw) > maxBytes {
		raw = raw[:maxBytes]
	}

	var lines []string
	for off := 0; off < len(raw); off += 16 {
		end := min(off+16, len(raw))
		chunk := raw[off:end]

		var hex, ascii strings.Builder
		for i, c := range chunk {
			if i == 8 {
				hex.WriteByte(' ')
			}
			fmt.Fprintf(&hex, "%02x ", c)
			if c >= 0x20 && c < 0x7f {
				ascii.WriteByte(c)
			} else {
				ascii.WriteByte('.')
			}
		}

		lines = append(lines, fmt.Sprintf("%08x  %-49s |%s|", off, hex.String(), ascii.String()))
	}
	return lines
}
