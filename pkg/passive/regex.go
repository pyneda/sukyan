package passive

import "regexp"

var privateIPRegex = regexp.MustCompile(`\b((10\.(25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.(25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.(25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?))|(172\.(1[6-9]|2\d|3[01])\.(25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.(25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?))|(192\.168\.(25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.(25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?))|(127\.0\.0\.1))\b`)
var fileUploadRegex = regexp.MustCompile(`(?i)<input[^>]*type\s*=\s*["']?file["']?[^>]*>`)
var emailRegex = regexp.MustCompile(`\b[a-zA-Z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}\b`)
var sessionTokenRegex = regexp.MustCompile(`(?i)[?&](auth|token|session(?:[_-])?id|jwt|access[_-]token|refresh[_-]token|apikey|api[_-]key|auth[_-]token|login[_-]token|auth[_-]code|client[_-]token|id[_-]token|session[_-]token|security[_-]token|session[_-]id|session[_-]key|sso[_-]token|oauth[_-]token|bearer[_-]token|account[_-]token|session[_-]auth|signature[_-]token|nonce|ticket|code|saml[_-]token|samltoken|jwt[_-]token|verification[_-]token|session[_-]cookie|access[_-]token|session[_-]id[_-]token|tokenid|sso[_-]auth[_-]token|authorization[_-]token|access[_-]key|session[_-]tokenid|authentication[_-]token|auth[_-]key|auth[_-]code|session[_-]key|authcode|sessionkey)=[-\w]*\b`)
