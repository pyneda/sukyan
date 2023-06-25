package http_utils

import (
	"net/http"
	"reflect"
	"testing"
)

func TestParseCookies(t *testing.T) {
	cookieStr := "cookie1=value1; cookie2=value2; cookie3=value3"
	expected := []*http.Cookie{
		{Name: "cookie1", Value: "value1"},
		{Name: "cookie2", Value: "value2"},
		{Name: "cookie3", Value: "value3"},
	}

	result := ParseCookies(cookieStr)

	if !reflect.DeepEqual(result, expected) {
		t.Errorf("Expected cookies %+v, got %+v", expected, result)
	}
}

func TestJoinCookies(t *testing.T) {
	cookies := []*http.Cookie{
		{Name: "cookie1", Value: "value1"},
		{Name: "cookie2", Value: "value2"},
		{Name: "cookie3", Value: "value3"},
	}
	expected := "cookie1=value1; cookie2=value2; cookie3=value3"

	result := JoinCookies(cookies)

	if result != expected {
		t.Errorf("Expected cookie string '%s', got '%s'", expected, result)
	}
}

func TestParseCookies_EmptyString(t *testing.T) {
	cookieStr := ""
	expected := []*http.Cookie{}

	result := ParseCookies(cookieStr)

	if !reflect.DeepEqual(result, expected) {
		t.Errorf("Expected empty cookies, got %+v", result)
	}
}

func TestJoinCookies_EmptyList(t *testing.T) {
	cookies := []*http.Cookie{}
	expected := ""

	result := JoinCookies(cookies)

	if result != expected {
		t.Errorf("Expected empty cookie string, got '%s'", result)
	}
}

func TestParseCookies_InvalidFormat(t *testing.T) {
	cookieStr := "cookie1=value1; cookie2"
	expected := []*http.Cookie{
		{Name: "cookie1", Value: "value1"},
	}

	result := ParseCookies(cookieStr)

	if !reflect.DeepEqual(result, expected) {
		t.Errorf("Expected cookies %+v, got %+v", expected, result)
	}
}

func TestJoinCookies_NilCookie(t *testing.T) {
	cookies := []*http.Cookie{
		{Name: "cookie1", Value: "value1"},
		nil,
		{Name: "cookie3", Value: "value3"},
	}
	expected := "cookie1=value1; cookie3=value3"

	result := JoinCookies(cookies)

	if result != expected {
		t.Errorf("Expected cookie string '%s', got '%s'", expected, result)
	}
}
