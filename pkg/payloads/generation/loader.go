package generation

import (
	"github.com/rs/zerolog/log"
	"gopkg.in/yaml.v3"
	"os"
	"path/filepath"
	"strings"
)

// loadGenerator reads an individual file and map it into an instance of PayloadGenerator
func loadGenerator(filePath string) (*PayloadGenerator, error) {
	var pg PayloadGenerator
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	err = yaml.Unmarshal(data, &pg)
	if err != nil {
		return nil, err
	}

	return &pg, nil
}

// LoadGenerators handle reading all YAML files in a directory and parsing them into PayloadGenerator instances
func LoadGenerators(dir string) ([]*PayloadGenerator, error) {
	var generators []*PayloadGenerator
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && (strings.HasSuffix(info.Name(), ".yaml") || strings.HasSuffix(info.Name(), ".yml")) {
			pg, err := loadGenerator(path)
			if err != nil {
				log.Error().Err(err).Msgf("Failed to load generator %s", info.Name())
			} else {
				generators = append(generators, pg)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return generators, nil
}
