package http_utils

import "strings"

// https://developer.mozilla.org/en-US/docs/Web/HTTP/Basics_of_HTTP/MIME_types/Common_types
// https://developer.mozilla.org/en-US/docs/Web/HTTP/Basics_of_HTTP/MIME_types
// https://gist.github.com/AshHeskes/6038140#gistcomment-3434363

// var MimeTypes sync.Map
const (
	MimeTypeImgGroup         = "image"
	MimeTypeTextGroup        = "text"
	MimeTypeApplicationGroup = "application"
	MimeTypeModelGroup       = "model"
	MimeTypeVideoGroup       = "video"
	MimeTypeFontGroup        = "font"
	MimeTypeAudioGroup       = "audio"
)

// MimeType holds information about a specific mime type
type MimeType struct {
	Extensions             []string
	MimeTypes              []string
	Group                  string
	Name                   string
	Code                   string
	CreateIssueOnDetection bool
	PassiveScanCallbacks   []interface{}
}

// MimeTypeChecker exposes few functions to detect mime type
type MimeTypeChecker struct {
	Available []MimeType
}

func (c *MimeTypeChecker) init() {
	if len(c.Available) == 0 {
		c.Available = GetMimeTypes()
	}
}

func (c *MimeTypeChecker) GetMimeTypeFromContentTypeString(contentType string) MimeType {
	c.init()
	for _, mt := range c.Available {
		for _, mts := range mt.MimeTypes {
			if strings.Contains(contentType, mts) {
				return mt
			}
		}
	}
	return MimeType{
		Code:                   "unknown",
		Group:                  c.getMimeTypeGroupFromContentString(contentType),
		Name:                   "Unknown mime type",
		Extensions:             []string{},
		MimeTypes:              []string{},
		CreateIssueOnDetection: true,
	}
}

func (c *MimeTypeChecker) getMimeTypeGroupFromContentString(contentType string) string {
	switch {
	case strings.HasPrefix(contentType, MimeTypeApplicationGroup) == true:
		return MimeTypeApplicationGroup
	case strings.HasPrefix(contentType, MimeTypeAudioGroup) == true:
		return MimeTypeAudioGroup
	case strings.HasPrefix(contentType, MimeTypeFontGroup) == true:
		return MimeTypeFontGroup
	case strings.HasPrefix(contentType, MimeTypeImgGroup) == true:
		return MimeTypeImgGroup
	case strings.HasPrefix(contentType, MimeTypeTextGroup) == true:
		return MimeTypeTextGroup
	case strings.HasPrefix(contentType, MimeTypeVideoGroup) == true:
		return MimeTypeVideoGroup
	case strings.HasPrefix(contentType, MimeTypeModelGroup) == true:
		return MimeTypeModelGroup
	default:
		return "unknown"
	}
}

func (c *MimeTypeChecker) GetMimeTypeFromURL(url string) MimeType {
	c.init()
	for _, mt := range c.Available {
		for _, extension := range mt.Extensions {
			if strings.HasSuffix(url, extension) {
				return mt
			}
		}
	}
	return MimeType{
		Code:                   "unknown",
		Group:                  "unknown",
		Name:                   "Unknown mime type",
		Extensions:             []string{},
		MimeTypes:              []string{},
		CreateIssueOnDetection: true,
	}
}

func GetApplicationMimeTypes() (mimeTypes []MimeType) {
	mimeTypes = append(mimeTypes, MimeType{
		Code:                   "azw",
		Group:                  MimeTypeApplicationGroup,
		Name:                   "AbiWord document",
		Extensions:             []string{".abw"},
		MimeTypes:              []string{"application/x-abiword"},
		CreateIssueOnDetection: true,
	})
	mimeTypes = append(mimeTypes, MimeType{
		Code:                   "arc",
		Group:                  MimeTypeApplicationGroup,
		Name:                   "Archive document (multiple files embedded)",
		Extensions:             []string{".arc"},
		MimeTypes:              []string{"application/x-freearc"},
		CreateIssueOnDetection: true,
	})
	mimeTypes = append(mimeTypes, MimeType{
		Code:                   "azw",
		Group:                  MimeTypeApplicationGroup,
		Name:                   "Amazon Kindle eBook format",
		Extensions:             []string{".azw"},
		MimeTypes:              []string{"application/vnd.amazon.ebook"},
		CreateIssueOnDetection: true,
	})
	mimeTypes = append(mimeTypes, MimeType{
		Code:       "binary",
		Group:      MimeTypeApplicationGroup,
		Name:       "Binary Data",
		Extensions: []string{".bin"},
		MimeTypes:  []string{"application/octet-stream"},
	})
	mimeTypes = append(mimeTypes, MimeType{
		Code:       "bzip",
		Group:      MimeTypeApplicationGroup,
		Name:       "BZip archive",
		Extensions: []string{".bz"},
		MimeTypes:  []string{"application/x-bzip"},
	})
	mimeTypes = append(mimeTypes, MimeType{
		Code:       "bzip2",
		Group:      MimeTypeApplicationGroup,
		Name:       "BZip2 archive",
		Extensions: []string{".bz2"},
		MimeTypes:  []string{"application/x-bzip2"},
	})
	mimeTypes = append(mimeTypes, MimeType{
		Code:                   "cshell",
		Group:                  MimeTypeApplicationGroup,
		Name:                   "C-Shell script",
		Extensions:             []string{".csh"},
		MimeTypes:              []string{"application/x-csh"},
		CreateIssueOnDetection: true,
	})
	mimeTypes = append(mimeTypes, MimeType{
		Code:                   "doc",
		Group:                  MimeTypeApplicationGroup,
		Name:                   "Microsoft Word",
		Extensions:             []string{".doc"},
		MimeTypes:              []string{"application/msword"},
		CreateIssueOnDetection: true,
	})
	mimeTypes = append(mimeTypes, MimeType{
		Code:                   "pdf",
		Group:                  MimeTypeApplicationGroup,
		Name:                   "Adobe Portable Document format (PDF)",
		Extensions:             []string{".pdf"},
		MimeTypes:              []string{"application/pdf"},
		CreateIssueOnDetection: true,
	})
	mimeTypes = append(mimeTypes, MimeType{
		Code:                   "php",
		Group:                  MimeTypeApplicationGroup,
		Name:                   "Hypertext Preprocessor (Personal Home Page)",
		Extensions:             []string{".php"},
		MimeTypes:              []string{"application/x-httpd-php"},
		CreateIssueOnDetection: false,
	})
	mimeTypes = append(mimeTypes, MimeType{
		Code:                   "ppt",
		Group:                  MimeTypeApplicationGroup,
		Name:                   "Microsoft PowerPoint",
		Extensions:             []string{".ppt"},
		MimeTypes:              []string{"application/vnd.ms-powerpoint"},
		CreateIssueOnDetection: true,
	})
	mimeTypes = append(mimeTypes, MimeType{
		Code:                   "pptx",
		Group:                  MimeTypeApplicationGroup,
		Name:                   "Microsoft PowerPoint (OpenXML)",
		Extensions:             []string{".pptx"},
		MimeTypes:              []string{"application/vnd.openxmlformats-officedocument.presentationml.presentation"},
		CreateIssueOnDetection: true,
	})
	mimeTypes = append(mimeTypes, MimeType{
		Code:                   "gzip",
		Group:                  MimeTypeApplicationGroup,
		Name:                   "GZip Compressed Archive",
		Extensions:             []string{".gz"},
		MimeTypes:              []string{"application/gzip"},
		CreateIssueOnDetection: false,
	})
	mimeTypes = append(mimeTypes, MimeType{
		Code:                   "jar",
		Group:                  MimeTypeApplicationGroup,
		Name:                   "Java Archive (JAR)",
		Extensions:             []string{".jar"},
		MimeTypes:              []string{"application/java-archive"},
		CreateIssueOnDetection: true,
	})
	mimeTypes = append(mimeTypes, MimeType{
		Code:                   "json",
		Group:                  MimeTypeApplicationGroup,
		Name:                   "JSON format",
		Extensions:             []string{".json"},
		MimeTypes:              []string{"application/json"},
		CreateIssueOnDetection: false,
	})
	mimeTypes = append(mimeTypes, MimeType{
		Code:                   "jsonld",
		Group:                  MimeTypeApplicationGroup,
		Name:                   "JSON-LD format",
		Extensions:             []string{".jsonld"},
		MimeTypes:              []string{"application/ld+json"},
		CreateIssueOnDetection: false,
	})
	mimeTypes = append(mimeTypes, MimeType{
		Code:                   "rar",
		Group:                  MimeTypeApplicationGroup,
		Name:                   "RAR archive",
		Extensions:             []string{".rar"},
		MimeTypes:              []string{"application/vnd.rar"},
		CreateIssueOnDetection: true,
	})
	mimeTypes = append(mimeTypes, MimeType{
		Code:                   "rtf",
		Group:                  MimeTypeApplicationGroup,
		Name:                   "Rich Text Format (RTF)",
		Extensions:             []string{".rtf"},
		MimeTypes:              []string{"application/rtf"},
		CreateIssueOnDetection: true,
	})
	mimeTypes = append(mimeTypes, MimeType{
		Code:                   "sh",
		Group:                  MimeTypeApplicationGroup,
		Name:                   "Bourne shell script",
		Extensions:             []string{".sh"},
		MimeTypes:              []string{"application/x-sh"},
		CreateIssueOnDetection: true,
	})
	mimeTypes = append(mimeTypes, MimeType{
		Code:                   "7z",
		Group:                  MimeTypeApplicationGroup,
		Name:                   "7-zip archive",
		Extensions:             []string{".7z"},
		MimeTypes:              []string{"application/x-7z-compressed"},
		CreateIssueOnDetection: true,
	})
	return mimeTypes
}

func GetTextMimeTypes() (mimeTypes []MimeType) {
	mimeTypes = append(mimeTypes, MimeType{
		Code:       "plain",
		Group:      MimeTypeTextGroup,
		Name:       "Plain Text",
		Extensions: []string{".txt"},
		MimeTypes:  []string{"text/plain"},
	})
	mimeTypes = append(mimeTypes, MimeType{
		Code:       "xml",
		Group:      MimeTypeTextGroup,
		Name:       "XML",
		Extensions: []string{".xml"},
		MimeTypes:  []string{"text/xml", "application/xml"},
	})
	mimeTypes = append(mimeTypes, MimeType{
		Code:       "css",
		Group:      MimeTypeTextGroup,
		Name:       "Cascading Style Sheets (CSS)",
		Extensions: []string{".css"},
		MimeTypes:  []string{"text/css"},
	})
	mimeTypes = append(mimeTypes, MimeType{
		Code:                   "csv",
		Group:                  MimeTypeTextGroup,
		Name:                   "Comma-separated values (CSV)",
		Extensions:             []string{".csv"},
		MimeTypes:              []string{"text/csv"},
		CreateIssueOnDetection: true,
	})
	mimeTypes = append(mimeTypes, MimeType{
		Code:       "html",
		Group:      MimeTypeTextGroup,
		Name:       "HyperText Markup Language (HTML)",
		Extensions: []string{".html", ".htm"},
		MimeTypes:  []string{"text/html"},
	})
	mimeTypes = append(mimeTypes, MimeType{
		Code:                   "icalendar",
		Group:                  MimeTypeTextGroup,
		Name:                   "iCalendar format",
		Extensions:             []string{".ics"},
		MimeTypes:              []string{"text/calendar"},
		CreateIssueOnDetection: true,
	})
	mimeTypes = append(mimeTypes, MimeType{
		Code:                   "javascript",
		Group:                  MimeTypeTextGroup,
		Name:                   "JavaScript",
		Extensions:             []string{".mjs", ".js"},
		MimeTypes:              []string{"text/javascript", "application/javascript", "application/ecmascript", "text/ecmascript"},
		CreateIssueOnDetection: false,
	})
	return mimeTypes
}

func GetImageMimeTypes() (mimeTypes []MimeType) {
	mimeTypes = append(mimeTypes, MimeType{
		Code:       "jpg",
		Group:      MimeTypeImgGroup,
		Name:       "JPG Image",
		Extensions: []string{".jpg", ".jpeg"},
		MimeTypes:  []string{"image/jpeg"},
	})
	mimeTypes = append(mimeTypes, MimeType{
		Code:       "png",
		Group:      MimeTypeImgGroup,
		Name:       "Portable Network Graphics (PNG)",
		Extensions: []string{".png"},
		MimeTypes:  []string{"image/png"},
	})
	mimeTypes = append(mimeTypes, MimeType{
		Code:       "bmp",
		Group:      MimeTypeImgGroup,
		Name:       "Windows OS/2 Bitmap Graphics",
		Extensions: []string{".bmp"},
		MimeTypes:  []string{"image/bmp"},
	})
	mimeTypes = append(mimeTypes, MimeType{
		Code:       "icon",
		Group:      MimeTypeImgGroup,
		Name:       "Icon format",
		Extensions: []string{".ico"},
		MimeTypes:  []string{"image/vnd.microsoft.icon"},
	})
	mimeTypes = append(mimeTypes, MimeType{
		Code:       "svg",
		Group:      MimeTypeImgGroup,
		Name:       "Scalable Vector Graphics (SVG)",
		Extensions: []string{".svg"},
		MimeTypes:  []string{"image/svg+xml"},
	})
	mimeTypes = append(mimeTypes, MimeType{
		Code:       "icon",
		Group:      MimeTypeImgGroup,
		Name:       "Icon format",
		Extensions: []string{".ico"},
		MimeTypes:  []string{"image/vnd.microsoft.icon"},
	})
	mimeTypes = append(mimeTypes, MimeType{
		Code:       "gif",
		Group:      MimeTypeImgGroup,
		Name:       "Graphics Interchange Format (GIF)",
		Extensions: []string{".gif"},
		MimeTypes:  []string{"image/gif"},
	})
	mimeTypes = append(mimeTypes, MimeType{
		Code:       "webp",
		Group:      MimeTypeImgGroup,
		Name:       "Web Picture format (WebP)",
		Extensions: []string{".webp"},
		MimeTypes:  []string{"image/webp"},
	})
	return mimeTypes
}

func GetVideoMimeTypes() (mimeTypes []MimeType) {
	mimeTypes = append(mimeTypes, MimeType{
		Code:       "avi",
		Group:      MimeTypeVideoGroup,
		Name:       "AVI: Audio Video Interleave",
		Extensions: []string{".avi"},
		MimeTypes:  []string{"video/x-msvideo"},
	})
	mimeTypes = append(mimeTypes, MimeType{
		Code:       "mpeg",
		Group:      MimeTypeVideoGroup,
		Name:       "MPEG Video",
		Extensions: []string{".mpeg"},
		MimeTypes:  []string{"video/mpeg"},
	})
	mimeTypes = append(mimeTypes, MimeType{
		Code:       "ogv",
		Group:      MimeTypeVideoGroup,
		Name:       "OGG Video",
		Extensions: []string{".ogv"},
		MimeTypes:  []string{"video/ogg"},
	})
	mimeTypes = append(mimeTypes, MimeType{
		Code:       "mpegts",
		Group:      MimeTypeVideoGroup,
		Name:       "MPEG transport stream",
		Extensions: []string{".ts"},
		MimeTypes:  []string{"video/mp2t"},
	})
	mimeTypes = append(mimeTypes, MimeType{
		Code:       "webm",
		Group:      MimeTypeVideoGroup,
		Name:       "WEBM video",
		Extensions: []string{".webm"},
		MimeTypes:  []string{"video/webm"},
	})
	mimeTypes = append(mimeTypes, MimeType{
		Code:       "3gp",
		Group:      MimeTypeVideoGroup,
		Name:       "3GPP audio/video container",
		Extensions: []string{".3gp"},
		MimeTypes:  []string{"video/3gpp"},
	})
	mimeTypes = append(mimeTypes, MimeType{
		Code:       "3g2",
		Group:      MimeTypeVideoGroup,
		Name:       "3GPP2 audio/video container",
		Extensions: []string{".3g2"},
		MimeTypes:  []string{"video/3gpp2"},
	})
	return mimeTypes
}

func GetFontMimeTypes() (mimeTypes []MimeType) {
	mimeTypes = append(mimeTypes, MimeType{
		Code:       "otf",
		Group:      MimeTypeFontGroup,
		Name:       "OpenType font",
		Extensions: []string{".otf"},
		MimeTypes:  []string{"font/otf"},
	})
	mimeTypes = append(mimeTypes, MimeType{
		Code:       "ttf",
		Group:      MimeTypeFontGroup,
		Name:       "TrueType",
		Extensions: []string{".ttf"},
		MimeTypes:  []string{"font/ttf"},
	})
	mimeTypes = append(mimeTypes, MimeType{
		Code:       "woff",
		Group:      MimeTypeFontGroup,
		Name:       "Web Open Font Format (WOFF)",
		Extensions: []string{".woff"},
		MimeTypes:  []string{"font/woff"},
	})
	mimeTypes = append(mimeTypes, MimeType{
		Code:       "woff2",
		Group:      MimeTypeFontGroup,
		Name:       "Web Open Font Format 2 (WOFF2)",
		Extensions: []string{".woff2"},
		MimeTypes:  []string{"font/woff2"},
	})
	return mimeTypes
}

func GetAudioMimeTypes() (mimeTypes []MimeType) {
	mimeTypes = append(mimeTypes, MimeType{
		Code:       "aac",
		Group:      MimeTypeAudioGroup,
		Name:       "AAC audio",
		Extensions: []string{".aac"},
		MimeTypes:  []string{"audio/aac"},
	})
	mimeTypes = append(mimeTypes, MimeType{
		Code:       "midi",
		Group:      MimeTypeAudioGroup,
		Name:       "Musical Instrument Digital Interface (MIDI)",
		Extensions: []string{".mid", ".midi"},
		MimeTypes:  []string{"audio/midi", "audio/x-midi"},
	})
	mimeTypes = append(mimeTypes, MimeType{
		Code:       "mp3",
		Group:      MimeTypeAudioGroup,
		Name:       "MP3 audio",
		Extensions: []string{".mp3"},
		MimeTypes:  []string{"audio/mpeg"},
	})
	mimeTypes = append(mimeTypes, MimeType{
		Code:       "oga",
		Group:      MimeTypeAudioGroup,
		Name:       "OGG audio",
		Extensions: []string{".oga"},
		MimeTypes:  []string{"audio/ogg"},
	})
	mimeTypes = append(mimeTypes, MimeType{
		Code:       "opus",
		Group:      MimeTypeAudioGroup,
		Name:       "Opus audio",
		Extensions: []string{".opus"},
		MimeTypes:  []string{"audio/opus"},
	})
	mimeTypes = append(mimeTypes, MimeType{
		Code:       "wav",
		Group:      MimeTypeAudioGroup,
		Name:       "Waveform Audio Format",
		Extensions: []string{".wav"},
		MimeTypes:  []string{"audio/wav"},
	})
	mimeTypes = append(mimeTypes, MimeType{
		Code:       "webma",
		Group:      MimeTypeAudioGroup,
		Name:       "WEBM Audio",
		Extensions: []string{".weba"},
		MimeTypes:  []string{"audio/webm"},
	})
	return mimeTypes
}

func GetMimeTypes() (mimeTypes []MimeType) {
	mimeTypes = append(mimeTypes, GetImageMimeTypes()...)
	mimeTypes = append(mimeTypes, GetTextMimeTypes()...)
	mimeTypes = append(mimeTypes, GetApplicationMimeTypes()...)
	mimeTypes = append(mimeTypes, GetFontMimeTypes()...)
	mimeTypes = append(mimeTypes, GetVideoMimeTypes()...)
	mimeTypes = append(mimeTypes, GetAudioMimeTypes()...)
	return mimeTypes
}
