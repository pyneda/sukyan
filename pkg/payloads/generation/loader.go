package generation

import (
	"embed"
	"os"
	"path/filepath"
	"strings"

	"github.com/rs/zerolog/log"
	"gopkg.in/yaml.v3"
)

//go:embed templates/*
var localTemplates embed.FS

// loadGenerator reads an individual file and maps it into an instance of PayloadGenerator
func loadGenerator(filePath string) (*PayloadGenerator, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	var pg PayloadGenerator
	err = yaml.Unmarshal(data, &pg)
	if err != nil {
		return nil, err
	}

	return &pg, nil
}

// loadGeneratorFromFS reads an individual file from the specified FS and maps it into an instance of PayloadGenerator
func loadGeneratorFromFS(fs embed.FS, path string) (*PayloadGenerator, error) {
	data, err := fs.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var pg PayloadGenerator
	err = yaml.Unmarshal(data, &pg)
	if err != nil {
		return nil, err
	}
	return &pg, nil
}

// LoadLocalGenerators loads all generators from the local directory
func LoadLocalGenerators() ([]*PayloadGenerator, error) {
	return loadGeneratorsFromFS(localTemplates, "templates")
}

// loadGeneratorsFromFS loads all generators from the specified FS
func loadGeneratorsFromFS(efs embed.FS, root string) ([]*PayloadGenerator, error) {
	var generators []*PayloadGenerator
	err := loadGeneratorsRecursiveFromFS(efs, root, &generators)
	if err != nil {
		return nil, err
	}
	return generators, nil
}

func loadGeneratorsRecursiveFromFS(efs embed.FS, root string, generators *[]*PayloadGenerator) error {
	entries, err := efs.ReadDir(root)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		path := filepath.Join(root, entry.Name())
		if entry.IsDir() {
			err := loadGeneratorsRecursiveFromFS(efs, path, generators)
			if err != nil {
				return err
			}
		} else if strings.HasSuffix(entry.Name(), ".yaml") || strings.HasSuffix(entry.Name(), ".yml") {
			pg, err := loadGeneratorFromFS(efs, path)
			if err != nil {
				log.Error().Err(err).Msgf("Failed to load generator %s", entry.Name())
			} else {
				*generators = append(*generators, pg)
			}
		}
	}
	return nil
}

// LoadUserGenerators loads all generators from the user specified directory
func LoadUserGenerators(dir string) ([]*PayloadGenerator, error) {
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

// LoadGenerators loads all generators from the local and user directories
func LoadGenerators(dir string) ([]*PayloadGenerator, error) {
	localGenerators, err := LoadLocalGenerators()
	if err != nil {
		return nil, err
	}
	if dir == "" {
		return localGenerators, nil
	}
	userGenerators, err := LoadUserGenerators(dir)
	if err != nil {
		return nil, err
	}
	return mergeGenerators(localGenerators, userGenerators), nil
}

// mergeGenerators merges local and user generators, giving priority to user generators
func mergeGenerators(local, user []*PayloadGenerator) []*PayloadGenerator {
	mappedGenerators := make(map[string]*PayloadGenerator)
	for _, lg := range local {
		mappedGenerators[lg.ID] = lg
	}
	for _, ug := range user {
		mappedGenerators[ug.ID] = ug // this will replace the local one if IDs match
	}
	var combined []*PayloadGenerator
	for _, v := range mappedGenerators {
		combined = append(combined, v)
	}
	return combined
}
