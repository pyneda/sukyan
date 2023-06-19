package passive

import (
	"sort"
	"testing"
)

func TestExtractURLs(t *testing.T) {
	tests := []struct {
		name     string
		response string
		want     []string
	}{
		{
			name:     "HTML example with multiple URLs",
			response: `<a href="https://example.com">Link</a> <img src="http://example.com/image.png">`,
			want:     []string{`"https://example.com"`, `"http://example.com/image.png"`},
		},
		{
			name:     "HTML example with relative URL",
			response: `<a href="/relative/path">Link</a>`,
			want:     []string{`"/relative/path"`},
		},
		{
			name:     "CSS example with multiple URLs",
			response: `body { background-image: url('https://example.com/images/bg.png'); } .example { background: url('/images/example.jpg'); }`,
			want:     []string{`'https://example.com/images/bg.png'`, `'/images/example.jpg'`},
		},
		{
			name:     "JavaScript example with multiple URLs",
			response: `fetch("https://www.example.com/api/data").then(doSomething); loadScript("//example.com/script.js");`,
			want:     []string{`"https://www.example.com/api/data"`, `"//example.com/script.js"`},
		},
		{
			name:     "Complex HTML example",
			response: `<html><head><link rel="stylesheet" href="https://example.com/styles.css"></head><body><a href="https://example.com">Home</a><img src="/images/logo.png"></body></html>`,
			want:     []string{`"https://example.com/styles.css"`, `"https://example.com"`, `"/images/logo.png"`},
		},
		{
			name:     "Complex CSS example",
			response: `body { background: url("/images/bg.jpg"); } .logo { background: url("https://example.com/logo.png"); }`,
			want:     []string{`"/images/bg.jpg"`, `"https://example.com/logo.png"`},
		},
		{
			name:     "Complex JavaScript example",
			response: `fetch("/api/data").then(doSomething); loadScript("https://example.com/script.js"); importScripts('//www.example.com/imported.js');`,
			want:     []string{`"/api/data"`, `"https://example.com/script.js"`, `'//www.example.com/imported.js'`},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractURLs(tt.response)
			if len(got) != len(tt.want) {
				t.Errorf("ExtractURLs() got = %v, want %v", got, tt.want)
				return
			}
			for i, url := range got {
				if url != tt.want[i] {
					t.Errorf("ExtractURLs() got = %v, want %v", got, tt.want)
				}
			}
		})
	}
}

func TestExtractAndAnalyzeURLS(t *testing.T) {
	tests := []struct {
		name             string
		response         string
		extractedFromURL string
		wantWeb          []string
		wantNonWeb       []string
	}{
		{
			name: "HTML example with absolute and relative URLs",
			response: `<a href="https://example.com">Link</a> 
			<a href="/relative/path">Link</a>`,
			extractedFromURL: "https://example.com/page",
			wantWeb:          []string{"https://example.com", "https://example.com/relative/path"},
			wantNonWeb:       []string{},
		},
		{
			name:             "HTML example with protocol-relative URL",
			response:         `<a href="//example.com">Link</a>`,
			extractedFromURL: "https://example.com/page",
			wantWeb:          []string{"https://example.com"},
			wantNonWeb:       []string{},
		},
		// {
		// 	name:             "JavaScript example with file URL (should be non-web)",
		// 	response:         `loadScript("file:///path/to/script.js");`,
		// 	extractedFromURL: "https://example.com/page",
		// 	wantWeb:          []string{},
		// 	wantNonWeb:       []string{"file:///path/to/script.js"},
		// },
		{
			name:             "HTML with absolute URL containing encoded characters",
			response:         `<a href="https://example.com/%C3%BC%C5%84%C3%AE%C3%A7%C3%B8d%C4%93">Link</a>`,
			extractedFromURL: "https://example.com/page",
			wantWeb:          []string{"https://example.com/%C3%BC%C5%84%C3%AE%C3%A7%C3%B8d%C4%93"},
			wantNonWeb:       []string{},
		},
		{
			name:             "Javascript using window.location with absolute URL",
			response:         `window.location = "https://example.com";`,
			extractedFromURL: "https://example.com/page",
			wantWeb:          []string{"https://example.com"},
			wantNonWeb:       []string{},
		},
		{
			name:             "Javascript using window.location with relative URL",
			response:         `window.location = "/relative/path";`,
			extractedFromURL: "https://example.com/page",
			wantWeb:          []string{"https://example.com/relative/path"},
			wantNonWeb:       []string{},
		},
		{
			name:             "CSS with font URL",
			response:         `@font-face { src: url('https://example.com/font.woff'); }`,
			extractedFromURL: "https://example.com/page",
			wantWeb:          []string{"https://example.com/font.woff"},
			wantNonWeb:       []string{},
		},
		{
			name:             "HTML using form action with absolute URL",
			response:         `<form action="https://example.com/submit">...</form>`,
			extractedFromURL: "https://example.com/page",
			wantWeb:          []string{"https://example.com/submit"},
			wantNonWeb:       []string{},
		},
		{
			name:             "HTML using form action with relative URL",
			response:         `<form action="/submit">...</form>`,
			extractedFromURL: "https://example.com/page",
			wantWeb:          []string{"https://example.com/submit"},
			wantNonWeb:       []string{},
		},
		{
			name:             "HTML using meta refresh with absolute URL",
			response:         `<meta http-equiv="refresh" content="0; url="https://example.com">`,
			extractedFromURL: "https://example.com/page",
			wantWeb:          []string{"https://example.com"},
			wantNonWeb:       []string{},
		},
		{
			name:             "HTML using meta refresh with relative URL",
			response:         `<meta http-equiv="refresh" content="0; url="/page">`,
			extractedFromURL: "https://example.com",
			wantWeb:          []string{"https://example.com/page"},
			wantNonWeb:       []string{},
		},
		{
			name:             "HTML using link tag with absolute URL",
			response:         `<link rel="stylesheet" href="https://example.com/style.css">`,
			extractedFromURL: "https://example.com/page",
			wantWeb:          []string{"https://example.com/style.css"},
			wantNonWeb:       []string{},
		},
		{
			name:             "HTML using link tag with relative URL",
			response:         `<link rel="stylesheet" href="/style.css">`,
			extractedFromURL: "https://example.com/page",
			wantWeb:          []string{"https://example.com/style.css"},
			wantNonWeb:       []string{},
		},
		{
			name:             "HTML using base tag",
			response:         `<base href="https://example.com/directory/">`,
			extractedFromURL: "https://example.com/page",
			wantWeb:          []string{"https://example.com/directory/"},
			wantNonWeb:       []string{},
		},
		{
			name:             "HTML using image source with absolute URL",
			response:         `<img src="https://example.com/image.png">`,
			extractedFromURL: "https://example.com/page",
			wantWeb:          []string{"https://example.com/image.png"},
			wantNonWeb:       []string{},
		},
		{
			name:             "HTML using image source with relative URL",
			response:         `<img src="/image.png">`,
			extractedFromURL: "https://example.com/page",
			wantWeb:          []string{"https://example.com/image.png"},
			wantNonWeb:       []string{},
		},
		{
			name:             "HTML using audio source with absolute URL",
			response:         `<audio src="https://example.com/audio.mp3">`,
			extractedFromURL: "https://example.com/page",
			wantWeb:          []string{"https://example.com/audio.mp3"},
			wantNonWeb:       []string{},
		},
		{
			name:             "HTML using audio source with relative URL",
			response:         `<audio src="/audio.mp3">`,
			extractedFromURL: "https://example.com/page",
			wantWeb:          []string{"https://example.com/audio.mp3"},
			wantNonWeb:       []string{},
		},
		{
			name:             "HTML using video source with absolute URL",
			response:         `<video src="https://example.com/video.mp4">`,
			extractedFromURL: "https://example.com/page",
			wantWeb:          []string{"https://example.com/video.mp4"},
			wantNonWeb:       []string{},
		},
		{
			name:             "HTML using video source with relative URL",
			response:         `<video src="/video.mp4">`,
			extractedFromURL: "https://example.com/page",
			wantWeb:          []string{"https://example.com/video.mp4"},
			wantNonWeb:       []string{},
		},
		{
			name:             "HTML using iframe source with absolute URL",
			response:         `<iframe src="https://example.com/iframe.html">`,
			extractedFromURL: "https://example.com/page",
			wantWeb:          []string{"https://example.com/iframe.html"},
			wantNonWeb:       []string{},
		},
		{
			name:             "HTML using iframe source with relative URL",
			response:         `<iframe src="/iframe.html">`,
			extractedFromURL: "https://example.com/page",
			wantWeb:          []string{"https://example.com/iframe.html"},
			wantNonWeb:       []string{},
		},
		{
			name: "Javascript function with multiple URLs",
			response: `
				function loadAssets() {
					loadScript("https://example.com/script1.js");
					loadScript("/script2.js");
					loadScript("script3.js");
				}`,
			extractedFromURL: "https://example.com/page",
			wantWeb: []string{
				"https://example.com/script1.js",
				"https://example.com/script2.js",
				"https://example.com/script3.js",
			},
			wantNonWeb: []string{},
		},
		{
			name: "Javascript function with absolute and relative URLs mixed with non-URLs",
			response: `
				function loadData() {
					var data1 = "https://example.com/data1";
					var data2 = "/data2";
					var data3 = "data3";
					var notAUrl = "hello world";
					return [data1, data2, data3, notAUrl];
				}`,
			extractedFromURL: "https://example.com/page",
			wantWeb: []string{
				"https://example.com/data1",
				"https://example.com/data2",
			},
			wantNonWeb: []string{},
		},
		{
			name:             "Javascript single line with multiple URLs",
			response:         `var urls = ["https://example.com/url1", "/url2", "url3", "not a url"];`,
			extractedFromURL: "https://example.com/page",
			wantWeb: []string{
				"https://example.com/url1",
				"https://example.com/url2",
			},
			wantNonWeb: []string{},
		},
		{
			name:             "Javascript with URL in string concatenation",
			response:         `var url = "https://" + "example.com" + "/path";`,
			extractedFromURL: "https://example.com/page",
			wantWeb:          []string{"https://example.com/path"},
			wantNonWeb:       []string{},
		},
		{
			name:             "Javascript with URL in template literal",
			response:         "var url = `https://${domain}/path`;",
			extractedFromURL: "https://example.com/page",
			wantWeb:          []string{},
			wantNonWeb:       []string{},
		},
		{
			name: "JavaScript function with absolute and relative URLs in different syntax",
			response: `
			function myFunc() {
				var url1 = "https://example.com/url1";
				var url2 = "/url2";
				var url3 = "./url3";
				var url4 = "../url4";
				var url5 = "url5";
				doSomething(url1, url2, url3, url4, url5);
			}`,
			extractedFromURL: "https://example.com/page",
			wantWeb: []string{
				"https://example.com/url1",
				"https://example.com/url2",
				"https://example.com/page/url3",
				"https://example.com/url4",
			},
			wantNonWeb: []string{},
		},
		{
			name: "JavaScript object with URLs as values",
			response: `var myObj = {
			url1: "https://example.com/url1",
			url2: "/url2",
			url3: "./url3",
			url4: "../url4",
			url5: "url5"
		};`,
			extractedFromURL: "https://example.com/page",
			wantWeb: []string{
				"https://example.com/url1",
				"https://example.com/url2",
				"https://example.com/page/url3",
				"https://example.com/url4",
			},
			wantNonWeb: []string{},
		},
		{
			name:             "JavaScript with fetch API",
			response:         `fetch("https://example.com/api/data")`,
			extractedFromURL: "https://example.com/page",
			wantWeb:          []string{"https://example.com/api/data"},
			wantNonWeb:       []string{},
		},
		{
			name: "JavaScript with async function and await fetch",
			response: `
			async function fetchData() {
				const response = await fetch("/api/data");
			}`,
			extractedFromURL: "https://example.com/page",
			wantWeb:          []string{"https://example.com/api/data"},
			wantNonWeb:       []string{},
		},
		{
			name: "JavaScript with multiple fetch calls",
			response: `
			fetch("https://example.com/api/data1");
			fetch("/api/data2");
			fetch("./api/data3");
		`,
			extractedFromURL: "https://example.com/page",
			wantWeb: []string{
				"https://example.com/api/data1",
				"https://example.com/api/data2",
				"https://example.com/page/api/data3",
			},
			wantNonWeb: []string{},
		},
		{
			name:             "JavaScript with URLs in a string array",
			response:         `var urls = ["https://example.com/url1", "/url2", "./url3", "../url4"];`,
			extractedFromURL: "https://example.com/page",
			wantWeb: []string{
				"https://example.com/url1",
				"https://example.com/url2",
				"https://example.com/page/url3",
				"https://example.com/url4",
			},
			wantNonWeb: []string{},
		},
		// {
		// 	name:             "JavaScript with URLs in comment",
		// 	response:         `var url = "https://example.com"; // This is a comment http://example.com/test/javascript/misc/comment.found`,
		// 	extractedFromURL: "https://example.com/page",
		// 	wantWeb:          []string{"https://example.com", "http://example.com/test/javascript/misc/comment.found"},
		// 	wantNonWeb:       []string{},
		// },
		{
			name:             "JavaScript string variable",
			response:         `var url = "https://example.com/aa/bb/cc/dd";`,
			extractedFromURL: "https://example.com/page",
			wantWeb:          []string{"https://example.com/aa/bb/cc/dd"},
			wantNonWeb:       []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractAndAnalyzeURLS(tt.response, tt.extractedFromURL)
			// Compare web URLs
			if !compareSlices(got.Web, tt.wantWeb) {
				t.Errorf("ExtractAndAnalyzeURLS().Web - = %v, want %v", got.Web, tt.wantWeb)
			}

			if !compareSlices(got.NonWeb, tt.wantNonWeb) {
				t.Errorf("ExtractAndAnalyzeURLS().NonWeb = %v, want %v", got.NonWeb, tt.wantNonWeb)
			}

		})
	}
}

func compareSlices(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	sort.Strings(got)
	sort.Strings(want)
	for i, v := range got {
		if v != want[i] {
			return false
		}
	}
	return true
}
