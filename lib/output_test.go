package lib

import (
	"fmt"
	"io/ioutil"
	"os"
	"testing"
)

type MockData struct {
	Name    string
	Content string
}

func (m MockData) String() string {
	return m.Name
}

func (m MockData) Pretty() string {
	return fmt.Sprintf("Name: %s | Content: %s", m.Name, m.Content)
}

func TestFormatOutput(t *testing.T) {
	data := MockData{
		Name:    "Test",
		Content: "Sample Content",
	}

	tests := []struct {
		format FormatType
		output string
		hasErr bool
	}{
		{Text, "Test", false},
		{Pretty, "Name: Test | Content: Sample Content", false},
		{JSON, `{
  "Name": "Test",
  "Content": "Sample Content"
}`, false},
		{YAML, "name: Test\ncontent: Sample Content\n", false},
		{FormatType("unknown"), "", true},
	}

	for _, tt := range tests {
		result, err := FormatOutput(data, tt.format)
		if (err != nil) != tt.hasErr {
			t.Errorf("expected error %v, got %v", tt.hasErr, err)
		}
		if result != tt.output {
			t.Errorf("expected output %q, got %q", tt.output, result)
		}
	}
}

func TestFormatOutputToFile(t *testing.T) {
	data := MockData{
		Name:    "Test",
		Content: "Sample Content",
	}

	filepath := "sukyan_testing_file_test_output.txt"

	// clean up after test
	defer os.Remove(filepath)

	err := FormatOutputToFile(data, Pretty, filepath)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	content, err := ioutil.ReadFile(filepath)
	if err != nil {
		t.Fatalf("could not read test file: %v", err)
	}

	expected := "Name: Test | Content: Sample Content"
	if string(content) != expected {
		t.Errorf("expected file content %q, got %q", expected, string(content))
	}
}
