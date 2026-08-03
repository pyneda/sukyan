package report

import (
	"embed"

	"codeberg.org/go-pdf/fpdf"
)

//go:embed fonts/*.ttf
var fontFiles embed.FS

const (
	fontSans = "Plex"
	fontMono = "PlexMono"
)

const (
	marginX       = 22.0
	marginTop     = 20.0
	marginBottom  = 18.0
	contentWidth  = 166.0
	evidenceInset = 5.0
	evidenceWidth = contentWidth - evidenceInset
)

const (
	sizeCoverTitle = 30.0
	sizeH1         = 20.0
	sizeH2         = 14.0
	sizeH3         = 10.5
	sizeBody       = 9.5
	sizeCaption    = 8.0
	sizeMono       = 7.5
)

const (
	leadBody = 4.8
	leadMono = 3.7
	leadH1   = 10.0
	leadH2   = 7.0
	leadH3   = 5.5
)

type rgb struct{ R, G, B int }

var (
	inkBody       = rgb{26, 29, 31}
	inkMuted      = rgb{92, 102, 107}
	inkRule       = rgb{213, 218, 220}
	inkEvidenceBG = rgb{245, 246, 247}
	inkWhite      = rgb{255, 255, 255}
)

// Descending lightness ramp, so severity stays distinguishable when the report
// is printed in greyscale.
var severityFills = map[string]rgb{
	"Critical": {110, 18, 20},
	"High":     {163, 58, 17},
	"Medium":   {196, 130, 10},
	"Low":      {127, 163, 181},
	"Info":     {176, 184, 188},
}

var severityUsesDarkInk = map[string]bool{
	"Medium": true,
	"Low":    true,
	"Info":   true,
}

var severityOrder = []string{"Critical", "High", "Medium", "Low", "Info"}

var severityDescriptions = map[string]string{
	"Critical": "Directly exploitable with severe impact: remote code execution, authentication bypass, or mass disclosure of sensitive data. Requires immediate remediation.",
	"High":     "Exploitable with significant impact on confidentiality, integrity or availability, though it may need specific conditions or an authenticated position.",
	"Medium":   "Meaningful weakness that is harder to exploit, needs user interaction, or has limited impact on its own. Often valuable to an attacker when chained.",
	"Low":      "Minor weakness or hardening gap with little direct impact. Worth fixing as part of routine maintenance.",
	"Info":     "Observation about the application's behaviour or configuration. Not a vulnerability by itself, but useful context for the findings above.",
}

func severityFill(sev string) rgb {
	if c, ok := severityFills[sev]; ok {
		return c
	}
	return severityFills["Info"]
}

func severityChipInk(sev string) rgb {
	if severityUsesDarkInk[sev] {
		return inkBody
	}
	return inkWhite
}

func registerFonts(pdf *fpdf.Fpdf) error {
	faces := []struct{ family, style, file string }{
		{fontSans, "", "fonts/IBMPlexSans-Regular.ttf"},
		{fontSans, "B", "fonts/IBMPlexSans-SemiBold.ttf"},
		{fontSans, "I", "fonts/IBMPlexSans-Italic.ttf"},
		{fontMono, "", "fonts/IBMPlexMono-Regular.ttf"},
		{fontMono, "B", "fonts/IBMPlexMono-SemiBold.ttf"},
	}

	for _, f := range faces {
		b, err := fontFiles.ReadFile(f.file)
		if err != nil {
			return err
		}
		pdf.AddUTF8FontFromBytes(f.family, f.style, b)
	}

	return pdf.Error()
}
