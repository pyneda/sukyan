package http_utils

import (
	"net/http"
	"strings"
)

// ParseCookies is a helper function to parse multiple cookies from a string
func ParseCookies(cookieStr string) []*http.Cookie {
	cookies := []*http.Cookie{}
	parts := strings.Split(cookieStr, ";")
	for _, part := range parts {
		pair := strings.SplitN(strings.TrimSpace(part), "=", 2)
		if len(pair) == 2 {
			cookie := &http.Cookie{
				Name:  pair[0],
				Value: pair[1],
			}
			cookies = append(cookies, cookie)
		}
	}
	return cookies
}

// JoinCookies is a helper function to join cookies into a string
func JoinCookies(cookies []*http.Cookie) string {
	cookieStrings := make([]string, 0, len(cookies))
	for _, cookie := range cookies {
		if cookie != nil {
			cookieStrings = append(cookieStrings, cookie.Name+"="+cookie.Value)
		}
	}
	return strings.Join(cookieStrings, "; ")
}
