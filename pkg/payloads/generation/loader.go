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
	files, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	generators := make([]*PayloadGenerator, 0)
	for _, f := range files {
		if strings.HasSuffix(f.Name(), ".yaml") || strings.HasSuffix(f.Name(), ".yml") {
			pg, err := loadGenerator(filepath.Join(dir, f.Name()))
			if err != nil {
				log.Error().Err(err).Msgf("Failed to load generator %s", f.Name())
				continue
			}

			generators = append(generators, pg)
		}
	}

	return generators, nil
}
