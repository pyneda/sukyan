package crawl

import "path"

var ignoredExtensions = map[string]struct{}{
	".js":    {},
	".png":   {},
	".jpg":   {},
	".jpeg":  {},
	".gif":   {},
	".svg":   {},
	".webp":  {},
	".css":   {},
	".woff":  {},
	".woff2": {},
	".ttf":   {},
	".otf":   {},
	".eot":   {},
	".ico":   {},
	".pdf":   {},
	".mp3":   {},
	".mp4":   {},
	".ogg":   {},
	".webm":  {},
	".flv":   {},
	".avi":   {},
	".mov":   {},
	".doc":   {},
	".docx":  {},
	".xls":   {},
}

func isIgnoredExtension(url string) bool {
	ext := path.Ext(url)
	_, isIgnored := ignoredExtensions[ext]
	return isIgnored
}
