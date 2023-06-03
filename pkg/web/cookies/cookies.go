package cookies

import (
	"github.com/go-rod/rod"
	"github.com/rs/zerolog/log"
)

// NetworkCookieSameSite Represents the cookie's 'SameSite' status:
// https://tools.ietf.org/html/draft-west-first-party-cookies
type NetworkCookieSameSite string

const (
	// NetworkCookieSameSiteStrict enum const
	NetworkCookieSameSiteStrict NetworkCookieSameSite = "Strict"

	// NetworkCookieSameSiteLax enum const
	NetworkCookieSameSiteLax NetworkCookieSameSite = "Lax"

	// NetworkCookieSameSiteNone enum const
	NetworkCookieSameSiteNone NetworkCookieSameSite = "None"
)

// NetworkCookiePriority (experimental) Represents the cookie's 'Priority' status:
// https://tools.ietf.org/html/draft-west-cookie-priority-00
type NetworkCookiePriority string

const (
	// NetworkCookiePriorityLow enum const
	NetworkCookiePriorityLow NetworkCookiePriority = "Low"

	// NetworkCookiePriorityMedium enum const
	NetworkCookiePriorityMedium NetworkCookiePriority = "Medium"

	// NetworkCookiePriorityHigh enum const
	NetworkCookiePriorityHigh NetworkCookiePriority = "High"
)

// Cookie Represents a cookie
type Cookie struct {
	// https://github.com/go-rod/rod/blob/622cad45df06723449f16c97d7782e239b2c3445/lib/proto/definitions.go#L9576
	Name      string
	Value     string
	Domain    string
	Path      string
	Expires   string
	Size      int
	HttpOnly  bool
	Secure    bool
	Session   bool
	SameSite  NetworkCookieSameSite
	Priority  NetworkCookiePriority
	SameParty bool
}

// GetPageCookies Gets current page cookies and return them
func GetPageCookies(p *rod.Page) (cookies []Cookie) {

	var cookieUrls []string
	receivedCookies, err := p.Cookies(cookieUrls)
	if err != nil {
		log.Error().Err(err).Msg("Error getting page cookies")
	} else {
		for _, c := range receivedCookies {
			cookie := Cookie{
				Name:      c.Name,
				Value:     c.Value,
				Domain:    c.Domain,
				Path:      c.Path,
				Expires:   c.Expires.String(),
				Size:      c.Size,
				HttpOnly:  c.HTTPOnly,
				Secure:    c.Secure,
				Session:   c.Session,
				SameSite:  NetworkCookieSameSite(c.SameSite),
				Priority:  NetworkCookiePriority(c.Priority),
				SameParty: c.SameParty,
			}
			cookies = append(cookies, cookie)

		}
		log.Info().Int("count", len(cookies)).Msg("Page cookies extracted")
	}
	return cookies
}
