package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func TestNewProviderFromConfigProvider(t *testing.T) {
	tests := []struct {
		name             string
		input            string
		expectedHostname string
		expectedOwner    string
		expectedName     string
	}{
		{
			name:             "simple name defaults hostname and owner",
			input:            "aws",
			expectedHostname: DEFAULT_PROVIDER_HOSTNAME,
			expectedOwner:    DEFAULT_PROVIDER_OWNER,
			expectedName:     "aws",
		},
		{
			name:             "owner/name sets owner",
			input:            "gavinbunney/kubectl",
			expectedHostname: DEFAULT_PROVIDER_HOSTNAME,
			expectedOwner:    "gavinbunney",
			expectedName:     "kubectl",
		},
		{
			name:             "full hostname/owner/name",
			input:            "registry.terraform.io/hashicorp/aws",
			expectedHostname: "registry.terraform.io",
			expectedOwner:    "hashicorp",
			expectedName:     "aws",
		},
		{
			name:             "custom hostname",
			input:            "custom.registry.io/myorg/myprovider",
			expectedHostname: "custom.registry.io",
			expectedOwner:    "myorg",
			expectedName:     "myprovider",
		},
		{
			name:             "empty string",
			input:            "",
			expectedHostname: DEFAULT_PROVIDER_HOSTNAME,
			expectedOwner:    DEFAULT_PROVIDER_OWNER,
			expectedName:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider, err := NewProviderFromConfigProvider(tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if provider.Hostname != tt.expectedHostname {
				t.Errorf("Hostname = %q, want %q", provider.Hostname, tt.expectedHostname)
			}
			if provider.Owner != tt.expectedOwner {
				t.Errorf("Owner = %q, want %q", provider.Owner, tt.expectedOwner)
			}
			if provider.Name != tt.expectedName {
				t.Errorf("Name = %q, want %q", provider.Name, tt.expectedName)
			}
		})
	}
}

func TestProviderGetDownloadBase(t *testing.T) {
	p := Provider{Hostname: "registry.terraform.io", Owner: "hashicorp", Name: "aws"}
	expected := filepath.Join("registry.terraform.io", "hashicorp", "aws")
	if got := p.GetDownloadBase(); got != expected {
		t.Errorf("GetDownloadBase() = %q, want %q", got, expected)
	}
}

func TestProviderString(t *testing.T) {
	p := Provider{Hostname: "registry.terraform.io", Owner: "hashicorp", Name: "aws"}
	expected := filepath.Join("registry.terraform.io", "hashicorp", "aws")
	if got := p.String(); got != expected {
		t.Errorf("String() = %q, want %q", got, expected)
	}
}

func TestProviderSpecificInstanceString(t *testing.T) {
	psi := ProviderSpecificInstance{
		Provider: Provider{Hostname: "registry.terraform.io", Owner: "hashicorp", Name: "aws"},
		Version:  "4.0.0",
		OS:       "linux",
		Arch:     "amd64",
	}
	expected := "registry.terraform.io/hashicorp/aws 4.0.0 linux_amd64"
	if got := psi.String(); got != expected {
		t.Errorf("String() = %q, want %q", got, expected)
	}
}

func TestProviderSpecificInstanceGetDownloadedFileName(t *testing.T) {
	psi := ProviderSpecificInstance{
		Provider: Provider{Hostname: "registry.terraform.io", Owner: "hashicorp", Name: "aws"},
		Version:  "4.0.0",
		OS:       "linux",
		Arch:     "amd64",
	}
	expected := "terraform-provider-aws_4.0.0_linux_amd64.zip"
	if got := psi.GetDownloadedFileName(); got != expected {
		t.Errorf("GetDownloadedFileName() = %q, want %q", got, expected)
	}
}

func TestGetProviderMetadataFromRegistry_Success(t *testing.T) {
	metadata := struct {
		ID       string                 `json:"id"`
		Versions []HCTFProviderVersion  `json:"versions"`
	}{
		ID: "hashicorp/aws",
		Versions: []HCTFProviderVersion{
			{
				Version:   "4.0.0",
				Protocols: []string{"5.0"},
				Platforms: []HCTFProviderPlatform{
					{OS: "linux", Arch: "amd64"},
					{OS: "darwin", Arch: "arm64"},
				},
			},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(metadata)
	}))
	defer server.Close()

	// Extract host from test server URL (strip scheme)
	host := server.URL[len("http://"):]

	oldScheme := registryScheme
	registryScheme = "http"
	defer func() { registryScheme = oldScheme }()

	p := Provider{Hostname: host, Owner: "hashicorp", Name: "aws"}
	result, err := p.GetProviderMetadataFromRegistry()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Versions) != 1 {
		t.Fatalf("expected 1 version, got %d", len(result.Versions))
	}
	if result.Versions[0].Version != "4.0.0" {
		t.Errorf("expected version 4.0.0, got %s", result.Versions[0].Version)
	}
	if len(result.Versions[0].Platforms) != 2 {
		t.Errorf("expected 2 platforms, got %d", len(result.Versions[0].Platforms))
	}
}

func TestGetProviderMetadataFromRegistry_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, "not valid json{{{")
	}))
	defer server.Close()

	host := server.URL[len("http://"):]

	oldScheme := registryScheme
	registryScheme = "http"
	defer func() { registryScheme = oldScheme }()

	p := Provider{Hostname: host, Owner: "hashicorp", Name: "aws"}

	// Current code panics on invalid JSON, so we recover
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for invalid JSON, but did not panic")
		}
	}()

	p.GetProviderMetadataFromRegistry()
}

func TestFilterToWantedPVIs_SingleMatch(t *testing.T) {
	p := Provider{Hostname: "registry.terraform.io", Owner: "hashicorp", Name: "aws"}
	metadata := RemoteProviderMetadata{
		Provider: p,
		Versions: []HCTFProviderVersion{
			{
				Version: "4.0.0",
				Platforms: []HCTFProviderPlatform{
					{OS: "linux", Arch: "amd64"},
				},
			},
		},
	}

	result, err := p.FilterToWantedPVIs(metadata, ">=4.0.0", []HCTFProviderPlatform{{OS: "linux", Arch: "amd64"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	if result[0].Version != "4.0.0" {
		t.Errorf("expected version 4.0.0, got %s", result[0].Version)
	}
	if result[0].OS != "linux" || result[0].Arch != "amd64" {
		t.Errorf("expected linux_amd64, got %s_%s", result[0].OS, result[0].Arch)
	}
}

func TestFilterToWantedPVIs_MultipleVersions(t *testing.T) {
	p := Provider{Hostname: "registry.terraform.io", Owner: "hashicorp", Name: "aws"}
	metadata := RemoteProviderMetadata{
		Provider: p,
		Versions: []HCTFProviderVersion{
			{
				Version:   "3.9.0",
				Platforms: []HCTFProviderPlatform{{OS: "linux", Arch: "amd64"}},
			},
			{
				Version:   "4.0.0",
				Platforms: []HCTFProviderPlatform{{OS: "linux", Arch: "amd64"}},
			},
			{
				Version:   "4.1.0",
				Platforms: []HCTFProviderPlatform{{OS: "linux", Arch: "amd64"}},
			},
		},
	}

	result, err := p.FilterToWantedPVIs(metadata, ">=4.0.0", []HCTFProviderPlatform{{OS: "linux", Arch: "amd64"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 results, got %d", len(result))
	}
}

func TestFilterToWantedPVIs_NoMatchingVersion(t *testing.T) {
	p := Provider{Hostname: "registry.terraform.io", Owner: "hashicorp", Name: "aws"}
	metadata := RemoteProviderMetadata{
		Provider: p,
		Versions: []HCTFProviderVersion{
			{
				Version:   "3.0.0",
				Platforms: []HCTFProviderPlatform{{OS: "linux", Arch: "amd64"}},
			},
		},
	}

	result, err := p.FilterToWantedPVIs(metadata, ">=4.0.0", []HCTFProviderPlatform{{OS: "linux", Arch: "amd64"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected 0 results, got %d", len(result))
	}
}

func TestFilterToWantedPVIs_NoMatchingPlatform(t *testing.T) {
	p := Provider{Hostname: "registry.terraform.io", Owner: "hashicorp", Name: "aws"}
	metadata := RemoteProviderMetadata{
		Provider: p,
		Versions: []HCTFProviderVersion{
			{
				Version:   "4.0.0",
				Platforms: []HCTFProviderPlatform{{OS: "linux", Arch: "amd64"}},
			},
		},
	}

	result, err := p.FilterToWantedPVIs(metadata, ">=4.0.0", []HCTFProviderPlatform{{OS: "darwin", Arch: "arm64"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected 0 results, got %d", len(result))
	}
}

func TestFilterToWantedPVIs_InvalidVersionRange(t *testing.T) {
	p := Provider{Hostname: "registry.terraform.io", Owner: "hashicorp", Name: "aws"}
	metadata := RemoteProviderMetadata{
		Provider: p,
		Versions: []HCTFProviderVersion{
			{
				Version:   "4.0.0",
				Platforms: []HCTFProviderPlatform{{OS: "linux", Arch: "amd64"}},
			},
		},
	}

	_, err := p.FilterToWantedPVIs(metadata, "not_a_range!!!", []HCTFProviderPlatform{{OS: "linux", Arch: "amd64"}})
	if err == nil {
		t.Error("expected error for invalid version range, got nil")
	}
}

func TestFilterToWantedPVIs_MultipleOSArchs(t *testing.T) {
	p := Provider{Hostname: "registry.terraform.io", Owner: "hashicorp", Name: "aws"}
	metadata := RemoteProviderMetadata{
		Provider: p,
		Versions: []HCTFProviderVersion{
			{
				Version: "4.0.0",
				Platforms: []HCTFProviderPlatform{
					{OS: "linux", Arch: "amd64"},
					{OS: "darwin", Arch: "arm64"},
					{OS: "windows", Arch: "amd64"},
				},
			},
		},
	}

	result, err := p.FilterToWantedPVIs(metadata, ">=4.0.0", []HCTFProviderPlatform{
		{OS: "linux", Arch: "amd64"},
		{OS: "darwin", Arch: "arm64"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 results, got %d", len(result))
	}
}
