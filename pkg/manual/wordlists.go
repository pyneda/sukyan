package manual

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/pyneda/sukyan/lib"
	"github.com/spf13/viper"
)

type Wordlist struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	SizeBytes int64  `json:"size_bytes"`
	SizeHuman string `json:"size_human"`
}

func (w Wordlist) String() string {
	return fmt.Sprintf("ID: %s, Name: %s, Size: %s", w.ID, w.Name, w.SizeHuman)
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

func (s *FilesystemWordlistStorage) GetWordlistByID(id string) (Wordlist, error) {
	wordlists, err := s.GetWordlists()
	if err != nil {
		return Wordlist{}, err
	}

	for _, wordlist := range wordlists {
		if wordlist.ID == id {
			return wordlist, nil
		}
	}

	return Wordlist{}, fmt.Errorf("wordlist not found: %s", id)

}

func (s *FilesystemWordlistStorage) ReadWordlist(name string, maxLines int) ([]string, error) {
	path := filepath.Join(s.basePath, name)

	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, fmt.Errorf("file does not exist: %s", path)
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
		if maxLines > 0 && len(lines) >= maxLines {
			break
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return lines, nil
}
