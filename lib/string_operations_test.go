package lib

import (
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"fmt"
	"strings"
	"testing"
)

func TestProcessString(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		processors []StringProcessor
		want       string
		wantErr    bool
		errMsg     string
	}{
		{
			name:  "single base64 encode",
			input: "Hello, World!",
			processors: []StringProcessor{
				{Type: Base64EncodeOperation, Description: "Base64 Encode"},
			},
			want:    "SGVsbG8sIFdvcmxkIQ==",
			wantErr: false,
		},
		{
			name:  "single base64 decode",
			input: "SGVsbG8sIFdvcmxkIQ==",
			processors: []StringProcessor{
				{Type: Base64DecodeOperation, Description: "Base64 Decode"},
			},
			want:    "Hello, World!",
			wantErr: false,
		},
		{
			name:  "invalid base64 decode",
			input: "invalid base64!@#$",
			processors: []StringProcessor{
				{Type: Base64DecodeOperation, Description: "Base64 Decode"},
			},
			want:    "",
			wantErr: true,
			errMsg:  "failed to apply operation",
		},
		{
			name:  "single url encode",
			input: "Hello World!@#$%",
			processors: []StringProcessor{
				{Type: URLEncodeOperation, Description: "URL Encode"},
			},
			want:    "Hello+World%21%40%23%24%25",
			wantErr: false,
		},
		{
			name:  "single url decode",
			input: "Hello+World%21%40%23%24%25",
			processors: []StringProcessor{
				{Type: URLDecodeOperation, Description: "URL Decode"},
			},
			want:    "Hello World!@#$%",
			wantErr: false,
		},
		{
			name:  "invalid url decode",
			input: "%invalid",
			processors: []StringProcessor{
				{Type: URLDecodeOperation, Description: "URL Decode"},
			},
			want:    "",
			wantErr: true,
			errMsg:  "failed to apply operation",
		},
		{
			name:  "single sha1 hash",
			input: "Hello, World!",
			processors: []StringProcessor{
				{Type: SHA1HashOperation, Description: "SHA1 Hash"},
			},
			want:    "0a0a9f2a6772942557ab5355d76af442f8f65e01",
			wantErr: false,
		},
		{
			name:  "single sha256 hash",
			input: "Hello, World!",
			processors: []StringProcessor{
				{Type: SHA256HashOperation, Description: "SHA256 Hash"},
			},
			want:    "dffd6021bb2bd5b0af676290809ec3a53191dd81c7f70a4b28688a362182986f",
			wantErr: false,
		},
		{
			name:  "single sha512 hash",
			input: "Hello, World!",
			processors: []StringProcessor{
				{Type: SHA512HashOperation, Description: "SHA512 Hash"},
			},
			want:    "374d794a95cdcfd8b35993185fef9ba368f160d8daf432d08ba9f1ed1e5abe6cc69291e0fa2fe0006a52570ef18c19def4e617c33ce52ef0a6e5fbe318cb0387",
			wantErr: false,
		},
		{
			name:  "single md5 hash",
			input: "Hello, World!",
			processors: []StringProcessor{
				{Type: MD5HashOperation, Description: "MD5 Hash"},
			},
			want:    "65a8e27d8879283831b664bd8b7f0ad4",
			wantErr: false,
		},
		{
			name:  "multiple operations chain",
			input: "Hello, World!",
			processors: []StringProcessor{
				{Type: Base64EncodeOperation, Description: "Base64 Encode"},
				{Type: URLEncodeOperation, Description: "URL Encode"},
				{Type: SHA256HashOperation, Description: "SHA256 Hash"},
			},
			want:    "0e5da538eb1062e1a10d403a4a030e0b16151619e9c45b363d6e4b64ded0ba50",
			wantErr: false,
		},
		{
			name:  "multiple operations to same result",
			input: "Hello, World!!!!!",
			processors: []StringProcessor{
				{Type: Base64EncodeOperation, Description: "Base64 Encode"},
				{Type: Base64DecodeOperation, Description: "Base64 Decode"},
				{Type: URLEncodeOperation, Description: "URL Encode"},
				{Type: URLDecodeOperation, Description: "URL Decode"},
				{Type: Base64EncodeOperation, Description: "Base64 Encode"},
				{Type: Base64DecodeOperation, Description: "Base64 Decode"},
			},
			want:    "Hello, World!!!!!",
			wantErr: false,
		},
		{
			name:  "unknown operation",
			input: "test",
			processors: []StringProcessor{
				{Type: "invalid", Description: "Invalid Operation"},
			},
			want:    "",
			wantErr: true,
			errMsg:  "failed to apply operation",
		},
		{
			name:  "empty input",
			input: "",
			processors: []StringProcessor{
				{Type: Base64EncodeOperation, Description: "Base64 Encode"},
			},
			want:    "",
			wantErr: false,
		},
		{
			name:       "empty processors list",
			input:      "test",
			processors: []StringProcessor{},
			want:       "test",
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ProcessString(tt.input, tt.processors)
			if (err != nil) != tt.wantErr {
				t.Errorf("ProcessString() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && !strings.Contains(err.Error(), tt.errMsg) {
				t.Errorf("ProcessString() error message = %v, want to contain %v", err, tt.errMsg)
				return
			}
			if got != tt.want {
				t.Errorf("ProcessString() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestProcessError_Error(t *testing.T) {
	err := &ProcessError{
		Operation: Base64EncodeOperation,
		Message:   "test message",
		Err:       fmt.Errorf("test error"),
	}

	expected := "processing error in base64encode: test message - test error"
	if got := err.Error(); got != expected {
		t.Errorf("ProcessError.Error() = %v, want %v", got, expected)
	}
}

// Helper function to verify hash results independently
func TestHashResults(t *testing.T) {
	input := "Hello, World!"

	// Test SHA1
	hasher := sha1.New()
	hasher.Write([]byte(input))
	sha1Want := hex.EncodeToString(hasher.Sum(nil))
	sha1Got, _ := applyOperation(input, SHA1HashOperation)
	if sha1Got != sha1Want {
		t.Errorf("SHA1 hash mismatch, got = %v, want %v", sha1Got, sha1Want)
	}

	// Test SHA256
	hasher = sha256.New()
	hasher.Write([]byte(input))
	sha256Want := hex.EncodeToString(hasher.Sum(nil))
	sha256Got, _ := applyOperation(input, SHA256HashOperation)
	if sha256Got != sha256Want {
		t.Errorf("SHA256 hash mismatch, got = %v, want %v", sha256Got, sha256Want)
	}

	// Test SHA512
	hasher = sha512.New()
	hasher.Write([]byte(input))
	sha512Want := hex.EncodeToString(hasher.Sum(nil))
	sha512Got, _ := applyOperation(input, SHA512HashOperation)
	if sha512Got != sha512Want {
		t.Errorf("SHA512 hash mismatch, got = %v, want %v", sha512Got, sha512Want)
	}

	// Test MD5
	hasher = md5.New()
	hasher.Write([]byte(input))
	md5Want := hex.EncodeToString(hasher.Sum(nil))
	md5Got, _ := applyOperation(input, MD5HashOperation)
	if md5Got != md5Want {
		t.Errorf("MD5 hash mismatch, got = %v, want %v", md5Got, md5Want)
	}
}
