//go:build ignore
// +build ignore

//go:generate go run generate.go

package main

import (
	"fmt"
	"github.com/pyneda/sukyan/db"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"gopkg.in/yaml.v2"
)

func toCamelCase(input string) string {
	words := strings.Split(input, "_")
	for i := range words {
		words[i] = strings.Title(words[i])
	}
	return strings.Join(words, "")
}

type IssueTemplateWrapper struct {
	Original      db.IssueTemplate
	CamelCaseCode string
}

func main() {
	var issueTemplates []IssueTemplateWrapper

	err := filepath.Walk("./db/kb", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() && strings.HasSuffix(info.Name(), ".yaml") {
			data, err := ioutil.ReadFile(path)
			if err != nil {
				return err
			}

			var issue db.IssueTemplate
			if err := yaml.Unmarshal(data, &issue); err != nil {
				return err
			}

			wrapper := IssueTemplateWrapper{
				Original:      issue,
				CamelCaseCode: toCamelCase(string(issue.Code)),
			}

			issueTemplates = append(issueTemplates, wrapper)
		}

		return nil
	})

	if err != nil {
		fmt.Println("Error walking the path:", err)
		return
	}

	tmpl, err := template.ParseFiles("db/kb/kb_template.go.tmpl")
	if err != nil {
		fmt.Println("Error parsing template:", err)
		return
	}

	f, err := os.Create("db/kb_autogenerated.go")
	if err != nil {
		fmt.Println("Error creating file:", err)
		return
	}
	defer f.Close()

	err = tmpl.Execute(f, issueTemplates)
	if err != nil {
		fmt.Println("Error executing template:", err)
	}
}
