package passive

import (
	"testing"
)

func TestGetSensibleDataFromText(t *testing.T) {
	testString := "sometext test@gmail.com https://github.com/golang/go.git test fake @ email .com 3700 0000 0000 002"
	data := GetSensibleDataFromText(testString)
	if len(data) != 3 {
		t.Error()
	}
	htmlString := "<div><span class='font-bold'>test@test.com</span><ul><li>86:90:ba:d7:0f:4b</li><li>Random Text 1ajkdsfj</li><li>10.10.100.220</li></div>"
	htmlData := GetSensibleDataFromText(htmlString)
	if len(htmlData) != 3 {
		t.Error()
	}
}
