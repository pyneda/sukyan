package lib

import (
	"errors"
	"fmt"
	"strings"
)

type Fingerprint struct {
	Name    string
	Version string
}

func (f *Fingerprint) GetNucleiTags() string {
	splitName := strings.Split(f.Name, " ")
	firstWord := strings.ToLower(splitName[0])
	return firstWord
}

func (f *Fingerprint) BuildCPE() (string, error) {
	if f.Version == "" {
		return "", errors.New("version not available")
	}
	name := strings.ToLower(strings.ReplaceAll(f.Name, " ", "_"))
	return fmt.Sprintf("cpe:/a:%s:%s:%s", name, name, f.Version), nil
}
