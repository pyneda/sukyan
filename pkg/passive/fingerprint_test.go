package passive

import (
	"reflect"
	"testing"

	"github.com/pyneda/sukyan/lib"
)

func TestParseFingerprints(t *testing.T) {
	tests := []struct {
		name string
		fps  []string
		want []lib.Fingerprint
	}{
		{
			name: "Fingerprints with version",
			fps:  []string{"PHP:5.4.0", "Apache HTTP Server:2.4.2"},
			want: []lib.Fingerprint{{Name: "PHP", Version: "5.4.0"}, {Name: "Apache HTTP Server", Version: "2.4.2"}},
		},
		{
			name: "Fingerprints without version",
			fps:  []string{"Akamai", "UNIX"},
			want: []lib.Fingerprint{{Name: "Akamai", Version: ""}, {Name: "UNIX", Version: ""}},
		},
		{
			name: "Mixed fingerprints",
			fps:  []string{"Python", "Uvicorn:0.12.0"},
			want: []lib.Fingerprint{{Name: "Python", Version: ""}, {Name: "Uvicorn", Version: "0.12.0"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseFingerprints(tt.fps); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parseFingerprints() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetNucleiTags(t *testing.T) {
	tests := []struct {
		name string
		fp   lib.Fingerprint
		want string
	}{
		{
			name: "Name with multiple words",
			fp:   lib.Fingerprint{Name: "Apache HTTP Server", Version: "2.4.2"},
			want: "apache",
		},
		{
			name: "Name with one word",
			fp:   lib.Fingerprint{Name: "Python", Version: "3.9.0"},
			want: "python",
		},
		{
			name: "Name with upper and lower case",
			fp:   lib.Fingerprint{Name: "JavaScript", Version: "1.8.5"},
			want: "javascript",
		},
		{
			name: "Name with special characters",
			fp:   lib.Fingerprint{Name: "Node.js", Version: "12.18.3"},
			want: "node.js",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.fp.GetNucleiTags(); got != tt.want {
				t.Errorf("GetNucleiTags() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBuildCPE(t *testing.T) {
	tests := []struct {
		name     string
		version  string
		expected string
		err      bool
	}{
		{
			name:     "PHP",
			version:  "5.4.0",
			expected: "cpe:/a:php:php:5.4.0",
			err:      false,
		},
		{
			name:     "UNIX",
			version:  "",
			expected: "",
			err:      true,
		},
		{
			name:     "Python",
			version:  "3.9.7",
			expected: "cpe:/a:python:python:3.9.7",
			err:      false,
		},
		{
			name:     "Apache HTTP Server",
			version:  "2.4.2",
			expected: "cpe:/a:apache_http_server:apache_http_server:2.4.2",
			err:      false,
		},
		{
			name:     "Nginx",
			version:  "",
			expected: "",
			err:      true,
		},
		{
			name:     "Microsoft IIS",
			version:  "10.0",
			expected: "cpe:/a:microsoft_iis:microsoft_iis:10.0",
			err:      false,
		},
		{
			name:     "Uvicorn",
			version:  "0.13.4",
			expected: "cpe:/a:uvicorn:uvicorn:0.13.4",
			err:      false,
		},
	}

	for _, test := range tests {
		f := lib.Fingerprint{
			Name:    test.name,
			Version: test.version,
		}

		cpe, err := f.BuildCPE()
		if (err != nil) != test.err {
			t.Errorf("Expected error status: %v, got: %v", test.err, (err != nil))
		}

		if cpe != test.expected {
			t.Errorf("Expected CPE: %s, got: %s", test.expected, cpe)
		}
	}
}
