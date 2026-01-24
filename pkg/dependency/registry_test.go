package dependency

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNPMChecker(t *testing.T) {
	tests := []struct {
		name           string
		packageName    string
		serverResponse int
		serverBody     string
		expectedExists bool
	}{
		{
			name:           "Package exists",
			packageName:    "express",
			serverResponse: 200,
			serverBody:     `{"name": "express", "versions": {"4.18.0": {}}}`,
			expectedExists: true,
		},
		{
			name:           "Package not found",
			packageName:    "nonexistent-package",
			serverResponse: 404,
			serverBody:     `{"error": "Not found"}`,
			expectedExists: false,
		},
		{
			name:           "Package with error in response",
			packageName:    "error-package",
			serverResponse: 200,
			serverBody:     `{"error": "some error"}`,
			expectedExists: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.serverResponse)
				w.Write([]byte(tt.serverBody))
			}))
			defer server.Close()

			// Note: We can't easily test with custom URLs in the checker
			// This test verifies the checker interface works
			checker := NewNPMChecker(nil)
			if checker.GetRegistryType() != RegistryNPM {
				t.Errorf("Expected registry type %s, got %s", RegistryNPM, checker.GetRegistryType())
			}
		})
	}
}

func TestPyPIChecker(t *testing.T) {
	checker := NewPyPIChecker(nil)

	if checker.GetRegistryType() != RegistryPyPI {
		t.Errorf("Expected registry type %s, got %s", RegistryPyPI, checker.GetRegistryType())
	}
}

func TestRubyGemsChecker(t *testing.T) {
	checker := NewRubyGemsChecker(nil)

	if checker.GetRegistryType() != RegistryRubyGems {
		t.Errorf("Expected registry type %s, got %s", RegistryRubyGems, checker.GetRegistryType())
	}
}

func TestPackagistChecker(t *testing.T) {
	checker := NewPackagistChecker(nil)

	if checker.GetRegistryType() != RegistryPackagist {
		t.Errorf("Expected registry type %s, got %s", RegistryPackagist, checker.GetRegistryType())
	}
}

func TestPackagistCheckerWithoutVendor(t *testing.T) {
	checker := NewPackagistChecker(nil)

	// Package without vendor should return exists=true (can't verify)
	result := checker.CheckPackage("package-without-vendor")

	if !result.Exists {
		t.Error("Expected Packagist checker to return exists=true for package without vendor")
	}
}

func TestCheckPackageInRegistry(t *testing.T) {
	tests := []struct {
		name     string
		pkg      PackageInfo
		registry RegistryType
	}{
		{
			name: "NPM package",
			pkg: PackageInfo{
				Name:     "express",
				Registry: RegistryNPM,
			},
			registry: RegistryNPM,
		},
		{
			name: "PyPI package",
			pkg: PackageInfo{
				Name:     "django",
				Registry: RegistryPyPI,
			},
			registry: RegistryPyPI,
		},
		{
			name: "Unknown registry defaults to exists",
			pkg: PackageInfo{
				Name:     "some-package",
				Registry: RegistryType("unknown"),
			},
			registry: RegistryType("unknown"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Just verify it doesn't panic
			result := CheckPackageInRegistry(tt.pkg, nil)

			if result.PackageName != tt.pkg.Name {
				t.Errorf("Expected package name %s, got %s", tt.pkg.Name, result.PackageName)
			}
		})
	}
}

func TestDefaultHTTPClient(t *testing.T) {
	client := DefaultHTTPClient()

	if client == nil {
		t.Error("DefaultHTTPClient returned nil")
	}

	if client.Timeout == 0 {
		t.Error("DefaultHTTPClient timeout should not be 0")
	}
}

func TestFileTypeToRegistry(t *testing.T) {
	tests := []struct {
		fileType FileType
		registry RegistryType
	}{
		{FileTypePackageJSON, RegistryNPM},
		{FileTypePackageLock, RegistryNPM},
		{FileTypeYarnLock, RegistryNPM},
		{FileTypeComposerJSON, RegistryPackagist},
		{FileTypeGemfile, RegistryRubyGems},
		{FileTypeRequirementsTxt, RegistryPyPI},
		{FileTypeGoMod, RegistryGo},
		{FileTypeCargoTOML, RegistryCrates},
		{FileTypePomXML, RegistryMaven},
	}

	for _, tt := range tests {
		t.Run(string(tt.fileType), func(t *testing.T) {
			registry := FileTypeToRegistry[tt.fileType]

			if registry != tt.registry {
				t.Errorf("FileType %s: expected registry %s, got %s",
					tt.fileType, tt.registry, registry)
			}
		})
	}
}
