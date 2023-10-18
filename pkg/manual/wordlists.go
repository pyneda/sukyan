package manual

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"github.com/pyneda/sukyan/lib"
	"github.com/spf13/viper"
	"io/ioutil"
	"path/filepath"
)

type Wordlist struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	SizeBytes int64  `json:"size_bytes"`
	SizeHuman string `json:"size_human"`
}

type WordlistStorage interface {
	GetWordlists() ([]Wordlist, error)
}

type FilesystemWordlistStorage struct {
	basePath          string
	allowedExtensions []string
}

func NewFilesystemWordlistStorage() *FilesystemWordlistStorage {
	basePath := viper.GetString("wordlists.directory")
	extensions := viper.GetStringSlice("wordlists.extensions")

	return &FilesystemWordlistStorage{
		basePath:          basePath,
		allowedExtensions: extensions,
	}
}

func (s *FilesystemWordlistStorage) GetWordlists() ([]Wordlist, error) {
	files, err := ioutil.ReadDir(s.basePath)
	if err != nil {
		return nil, err
	}

	var wordlists []Wordlist
	for _, file := range files {
		if file.IsDir() {
			continue
		}

		// Checking if the file extension is allowed
		ext := filepath.Ext(file.Name())
		if !lib.SliceContains(s.allowedExtensions, ext) {
			continue
		}

		sizeBytes := file.Size()
		hashData := fmt.Sprintf("%s%d", file.Name(), sizeBytes)
		hash := sha256.Sum256([]byte(hashData))
		hashStr := hex.EncodeToString(hash[:])
		wordlists = append(wordlists, Wordlist{
			ID:        hashStr,
			Name:      file.Name(),
			SizeBytes: sizeBytes,
			SizeHuman: lib.BytesCountToHumanReadable(sizeBytes),
		})
	}

	return wordlists, nil
}
