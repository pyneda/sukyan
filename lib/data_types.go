package lib

import (
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"net/mail"
	"net/url"

	"regexp"
	"strconv"
	"strings"
	"time"
)

type DataType string

const (
	TypeInt     DataType = "Integer"
	TypeFloat   DataType = "Float"
	TypeJSON    DataType = "JSON"
	TypeXML     DataType = "XML"
	TypeSVG     DataType = "SVG"
	TypeDate1   DataType = "Date (YYYY-MM-DD)"
	TypeDate2   DataType = "Date (MM/DD/YYYY)"
	TypeArray   DataType = "Array"
	TypeBoolean DataType = "Boolean"
	TypeEmail   DataType = "Email"
	TypeURL     DataType = "URL"
	TypeBase64  DataType = "Base64"
	TypeUUID    DataType = "UUID"
	TypeHex     DataType = "Hexadecimal"
	TypeHTML    DataType = "HTML"
	TypeJSCode  DataType = "JavaScript Code"
	TypeString  DataType = "String"
)

func isCommaSeparatedList(s string) bool {
	parts := strings.Split(s, ",")
	if len(parts) < 2 {
		return false
	}

	for _, part := range parts {
		trimmedPart := strings.TrimSpace(part)
		if trimmedPart == "" {
			return false
		}

		if strings.ContainsAny(trimmedPart, "{}[]\"'<>") {
			return false
		}
	}
	return true
}

func GuessDataType(s string) DataType {
	_, err := strconv.Atoi(s)
	if err == nil {
		return TypeInt
	}

	_, err = strconv.ParseFloat(s, 64)
	if err == nil {
		return TypeFloat
	}

	if strings.EqualFold(s, "true") || strings.EqualFold(s, "false") {
		return TypeBoolean
	}

	var js json.RawMessage
	err = json.Unmarshal([]byte(s), &js)
	if err == nil {
		return TypeJSON
	}

	var x xml.Name
	err = xml.Unmarshal([]byte(s), &x)
	if err == nil {
		commonHTMLTags := []string{"html", "head", "body", "p", "div", "span", "a", "img", "script", "link", "meta", "style"}
		isHTML := false
		for _, tag := range commonHTMLTags {
			if strings.Contains(strings.ToLower(s), "<"+tag) {
				isHTML = true
				break
			}
		}
		if isHTML {
			return TypeHTML
		}
		if x.Local == "svg" {
			return TypeSVG
		}
		return TypeXML
	}

	_, err = time.Parse("2006-01-02", s)
	if err == nil {
		return TypeDate1
	}

	_, err = time.Parse("01/02/2006", s)
	if err == nil {
		return TypeDate2
	}

	if isCommaSeparatedList(s) {
		return TypeArray
	}

	_, err = mail.ParseAddress(s)
	if err == nil {
		return TypeEmail
	}

	_, err = url.ParseRequestURI(s)
	if err == nil {
		return TypeURL
	}

	isUUID := regexp.MustCompile(`\b[0-9a-f]{8}\b-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-\b[0-9a-f]{12}\b`)
	if isUUID.MatchString(s) {
		return TypeUUID
	}

	jsKeywords := []string{"function", "var", "let", "const", "if", "else", "return"}
	isJS := false
	for _, keyword := range jsKeywords {
		if strings.Contains(s, keyword) {
			isJS = true
			break
		}
	}
	if isJS {
		return TypeJSCode
	}

	isHex := regexp.MustCompile(`\b[0-9a-fA-F]+\b`)
	if isHex.MatchString(s) {
		return TypeHex
	}

	_, err = base64.StdEncoding.DecodeString(s)
	if err == nil {
		return TypeBase64
	}

	return TypeString
}
