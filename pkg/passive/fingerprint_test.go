package passive

import (
	"reflect"
	"testing"
)

func TestParseFingerprints(t *testing.T) {
	tests := []struct {
		name string
		fps  []string
		want []Fingerprint
	}{
		{
			name: "Fingerprints with version",
			fps:  []string{"PHP:5.4.0", "Apache HTTP Server:2.4.2"},
			want: []Fingerprint{{Name: "PHP", Version: "5.4.0"}, {Name: "Apache HTTP Server", Version: "2.4.2"}},
		},
		{
			name: "Fingerprints without version",
			fps:  []string{"Akamai", "UNIX"},
			want: []Fingerprint{{Name: "Akamai", Version: ""}, {Name: "UNIX", Version: ""}},
		},
		{
			name: "Mixed fingerprints",
			fps:  []string{"Python", "Uvicorn:0.12.0"},
			want: []Fingerprint{{Name: "Python", Version: ""}, {Name: "Uvicorn", Version: "0.12.0"}},
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
		fp   Fingerprint
		want string
	}{
		{
			name: "Name with multiple words",
			fp:   Fingerprint{Name: "Apache HTTP Server", Version: "2.4.2"},
			want: "apache",
		},
		{
			name: "Name with one word",
			fp:   Fingerprint{Name: "Python", Version: "3.9.0"},
			want: "python",
		},
		{
			name: "Name with upper and lower case",
			fp:   Fingerprint{Name: "JavaScript", Version: "1.8.5"},
			want: "javascript",
		},
		{
			name: "Name with special characters",
			fp:   Fingerprint{Name: "Node.js", Version: "12.18.3"},
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
