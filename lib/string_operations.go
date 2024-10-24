package lib

import (
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/url"
)

// StringOperation defines the type of operation to be performed
type StringOperation string

const (
	Base64EncodeOperation StringOperation = "base64encode"
	Base64DecodeOperation StringOperation = "base64decode"
	URLEncodeOperation    StringOperation = "urlencode"
	URLDecodeOperation    StringOperation = "urldecode"
	SHA1HashOperation     StringOperation = "sha1hash"
	SHA256HashOperation   StringOperation = "sha256hash"
	SHA512HashOperation   StringOperation = "sha512hash"
	MD5HashOperation      StringOperation = "md5hash"
)

// StringProcessor represents a single processing operation
type StringProcessor struct {
	Type        StringOperation `json:"type"`
	Description string          `json:"description"`
}

// ProcessError wraps errors that occur during processing
type ProcessError struct {
	Operation StringOperation
	Message   string
	Err       error
}

func (e *ProcessError) Error() string {
	return fmt.Sprintf("processing error in %s: %s - %v", e.Operation, e.Message, e.Err)
}

// ProcessString applies a chain of processors to an input string
func ProcessString(input string, processors []StringProcessor) (string, error) {
	result := input

	for _, proc := range processors {
		var err error
		result, err = applyOperation(result, proc.Type)
		if err != nil {
			return "", &ProcessError{
				Operation: proc.Type,
				Message:   "failed to apply operation",
				Err:       err,
			}
		}
	}

	return result, nil
}

// applyOperation applies a single processing operation
func applyOperation(input string, operation StringOperation) (string, error) {
	switch operation {
	case Base64EncodeOperation:
		return base64.StdEncoding.EncodeToString([]byte(input)), nil

	case Base64DecodeOperation:
		decoded, err := base64.StdEncoding.DecodeString(input)
		if err != nil {
			return "", err
		}
		return string(decoded), nil

	case URLEncodeOperation:
		return url.QueryEscape(input), nil

	case URLDecodeOperation:
		decoded, err := url.QueryUnescape(input)
		if err != nil {
			return "", err
		}
		return decoded, nil

	case SHA1HashOperation:
		hasher := sha1.New()
		hasher.Write([]byte(input))
		return hex.EncodeToString(hasher.Sum(nil)), nil

	case SHA256HashOperation:
		hasher := sha256.New()
		hasher.Write([]byte(input))
		return hex.EncodeToString(hasher.Sum(nil)), nil

	case SHA512HashOperation:
		hasher := sha512.New()
		hasher.Write([]byte(input))
		return hex.EncodeToString(hasher.Sum(nil)), nil

	case MD5HashOperation:
		hasher := md5.New()
		hasher.Write([]byte(input))
		return hex.EncodeToString(hasher.Sum(nil)), nil

	default:
		return "", fmt.Errorf("unknown operation type: %s", operation)
	}
}
