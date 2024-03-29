package scope

import (
	"fmt"
	"net/url"

	"github.com/jpillora/go-tld"
	"github.com/rs/zerolog/log"
)

// Scope groups different scope items
type Scope struct {
	ScopeItems []DomainScope
}

type DomainScope struct {
	domain string
	scope  string // strict,www or subdomains
}

// AddScopeItem allows to add a new item to the scope
func (s *Scope) AddScopeItem(domain string, scope string) []DomainScope {
	s.ScopeItems = append(s.ScopeItems, DomainScope{
		domain: domain,
		scope:  scope,
	})
	return s.ScopeItems
}

// CreateScopeItemsFromUrls allow to create multiple scope items from multiple urls
func (s *Scope) CreateScopeItemsFromUrls(paths []string, scope string) {
	for _, startURL := range paths {
		u, err := url.Parse(startURL)
		if err == nil {
			s.AddScopeItem(u.Hostname(), scope)
		} else {
			log.Error().Err(err).Str("url", startURL).Msg("Error creating scope")
		}
	}
}

// IsInScope checks if a domain is in the current scope
func (s *Scope) IsInScope(path string) bool {
	u, err := tld.Parse(path)
	if err != nil {
		// tld.Parse failed; falling back to url.Parse for localhost check
		parsedURL, err := url.Parse(path)
		if err == nil && parsedURL.Hostname() == "localhost" {
			// Handle localhost special case
			for _, scopeItem := range s.ScopeItems {
				if scopeItem.domain == "localhost" {
					return true
				}
			}
			return false
		}
		log.Error().Err(err).Str("url", path).Msg("Url to check if is in scope seems not valid. Assuming it is not in scope, this should be reviewed.")
		return false
	}

	tld := fmt.Sprintf("%s.%s", u.Domain, u.TLD)
	host := u.Hostname()

	for _, scopeItem := range s.ScopeItems {
		switch scopeItem.scope {
		case "subdomains":
			if scopeItem.domain == host || scopeItem.domain == tld {
				return true
			}
		case "www":
			if scopeItem.domain == host || scopeItem.domain == u.Host || "www."+scopeItem.domain == host {
				return true
			}
		default: // Assumes strict
			if scopeItem.domain == host {
				return true
			}
		}
	}
	return false
}
