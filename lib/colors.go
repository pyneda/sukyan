package lib

// ANSI color codes
const (
	ResetColor = "\033[0m"

	Red    = "\033[31m"
	Green  = "\033[32m"
	Yellow = "\033[33m"
	Blue   = "\033[34m"
	Purple = "\033[35m"
	Cyan   = "\033[36m"
	White  = "\033[37m"
)

// Colorize wraps the text with the specified color and resets the color after.
func Colorize(text, color string) string {
	return color + text + ResetColor
}
