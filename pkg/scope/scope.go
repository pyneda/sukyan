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
		log.Error().Err(err).Str("url", path).Msg("Url to check if is in scope seems not valid")
		return false
	}
	//host, _, _ := net.SplitHostPort(u.Host)
	tld := fmt.Sprintf("%s.%s", u.Domain, u.TLD)
	// wwwTld := fmt.Sprintf("%s.%s", "www", tld)
	host := u.Hostname()
	// fmt.Printf("Tld: %s  wwwTld: %s  Host: %s", tld, wwwTld, host)

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
