package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"testing"
)

func TestNewProviderFromConfigProvider(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected Provider
	}{
		{
			"simple name defaults to hashicorp",
			"aws",
			Provider{Hostname: "registry.terraform.io", Owner: "hashicorp", Name: "aws"},
		},
		{
			"two parts sets owner and name",
			"gavinbunney/kubectl",
			Provider{Hostname: "registry.terraform.io", Owner: "gavinbunney", Name: "kubectl"},
		},
		{
			"three parts sets hostname owner name",
			"custom.io/myorg/myprovider",
			Provider{Hostname: "custom.io", Owner: "myorg", Name: "myprovider"},
		},
		{
			"four parts joins remainder with slash",
			"a/b/c/d",
			Provider{Hostname: "a", Owner: "b", Name: "c/d"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewProviderFromConfigProvider(tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.expected {
				t.Errorf("got %+v, want %+v", got, tt.expected)
			}
		})
	}
}

func TestProviderGetDownloadBase(t *testing.T) {
	p := Provider{Hostname: "registry.terraform.io", Owner: "hashicorp", Name: "aws"}
	expected := filepath.Join("registry.terraform.io", "hashicorp", "aws")
	got := p.GetDownloadBase()
	if got != expected {
		t.Errorf("GetDownloadBase() = %q, want %q", got, expected)
	}
}

func TestProviderString(t *testing.T) {
	p := Provider{Hostname: "registry.terraform.io", Owner: "hashicorp", Name: "aws"}
	expected := filepath.Join("registry.terraform.io", "hashicorp", "aws")
	got := p.String()
	if got != expected {
		t.Errorf("String() = %q, want %q", got, expected)
	}
}

func TestProviderSpecificInstanceString(t *testing.T) {
	psi := ProviderSpecificInstance{
		Provider: Provider{Hostname: "registry.terraform.io", Owner: "hashicorp", Name: "aws"},
		Version:  "5.0.0",
		OS:       "linux",
		Arch:     "amd64",
	}
	expected := "registry.terraform.io/hashicorp/aws 5.0.0 linux_amd64"
	got := psi.String()
	if got != expected {
		t.Errorf("String() = %q, want %q", got, expected)
	}
}

func TestProviderSpecificInstanceGetDownloadedFileName(t *testing.T) {
	psi := ProviderSpecificInstance{
		Provider: Provider{Name: "aws"},
		Version:  "5.0.0",
		OS:       "linux",
		Arch:     "amd64",
	}
	expected := "terraform-provider-aws_5.0.0_linux_amd64.zip"
	got := psi.GetDownloadedFileName()
	if got != expected {
		t.Errorf("GetDownloadedFileName() = %q, want %q", got, expected)
	}
}

func TestGetProviderMetadataFromRegistry(t *testing.T) {
	tests := []struct {
		name        string
		statusCode  int
		body        any
		wantErr     bool
		wantVerions int
	}{
		{
			name:       "successful fetch",
			statusCode: 200,
			body: RemoteProviderMetadata{
				Versions: []HCTFProviderVersion{
					{Version: "1.0.0", Platforms: []HCTFProviderPlatform{{OS: "linux", Arch: "amd64"}}},
					{Version: "2.0.0", Platforms: []HCTFProviderPlatform{{OS: "linux", Arch: "amd64"}}},
				},
			},
			wantErr:     false,
			wantVerions: 2,
		},
		{
			name:       "HTTP 404",
			statusCode: 404,
			body:       nil,
			wantErr:    true,
		},
		{
			name:       "HTTP 500",
			statusCode: 500,
			body:       nil,
			wantErr:    true,
		},
		{
			name:       "empty versions",
			statusCode: 200,
			body: RemoteProviderMetadata{
				Versions: []HCTFProviderVersion{},
			},
			wantErr:     false,
			wantVerions: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
				if tt.body != nil {
					b, _ := json.Marshal(tt.body)
					w.Write(b)
				}
			}))
			defer server.Close()

			oldClient := httpClient
			httpClient = server.Client()
			defer func() { httpClient = oldClient }()

			// Extract host:port from test server URL (strip https://)
			serverHost := server.URL[len("https://"):]

			p := Provider{Hostname: serverHost, Owner: "hashicorp", Name: "aws"}
			got, err := p.GetProviderMetadataFromRegistry()
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got.Versions) != tt.wantVerions {
				t.Errorf("got %d versions, want %d", len(got.Versions), tt.wantVerions)
			}
		})
	}
}

func TestGetProviderMetadataFromRegistryInvalidJSON(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		fmt.Fprint(w, "not json at all{{{")
	}))
	defer server.Close()

	oldClient := httpClient
	httpClient = server.Client()
	defer func() { httpClient = oldClient }()

	serverHost := server.URL[len("https://"):]
	p := Provider{Hostname: serverHost, Owner: "hashicorp", Name: "aws"}
	_, err := p.GetProviderMetadataFromRegistry()
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

func TestFilterToWantedPVIs(t *testing.T) {
	p := Provider{Hostname: "registry.terraform.io", Owner: "hashicorp", Name: "aws"}
	metadata := RemoteProviderMetadata{
		Provider: p,
		Versions: []HCTFProviderVersion{
			{
				Version: "4.0.0",
				Platforms: []HCTFProviderPlatform{
					{OS: "linux", Arch: "amd64"},
					{OS: "darwin", Arch: "arm64"},
				},
			},
			{
				Version: "5.0.0",
				Platforms: []HCTFProviderPlatform{
					{OS: "linux", Arch: "amd64"},
					{OS: "darwin", Arch: "arm64"},
				},
			},
			{
				Version: "5.1.0",
				Platforms: []HCTFProviderPlatform{
					{OS: "linux", Arch: "amd64"},
				},
			},
		},
	}

	tests := []struct {
		name     string
		pmc      ProviderMirrorConfiguration
		osArchs  []HCTFProviderPlatform
		wantPVIs []ProviderSpecificInstance
		wantErr  bool
	}{
		{
			"single matching version and platform",
			ProviderMirrorConfiguration{
				Reference:    "merp",
				VersionRange: ">=5.0.0 <5.1.0",
				OSArchs:      []HCTFProviderPlatform{{OS: "linux", Arch: "amd64"}},
			},
			[]HCTFProviderPlatform{{OS: "linux", Arch: "amd64"}},
			[]ProviderSpecificInstance{
				{Provider: p, Version: "5.0.0", OS: "linux", Arch: "amd64"},
			},
			false,
		},
		{
			"no version match",
			ProviderMirrorConfiguration{
				Reference:    "merp",
				VersionRange: ">=6.0.0",
				OSArchs:      []HCTFProviderPlatform{{OS: "linux", Arch: "amd64"}},
			},
			[]HCTFProviderPlatform{{OS: "linux", Arch: "amd64"}},
			[]ProviderSpecificInstance{},
			false,
		},
		{
			"no platform match logs error but no error returned",
			ProviderMirrorConfiguration{
				Reference:    "merp",
				VersionRange: ">=5.1.0",
				OSArchs:      []HCTFProviderPlatform{{OS: "linux", Arch: "amd64"}},
			},
			[]HCTFProviderPlatform{{OS: "windows", Arch: "arm64"}},
			[]ProviderSpecificInstance{},
			false,
		},
		{
			"multiple versions partial range match",
			ProviderMirrorConfiguration{
				Reference:    "merp",
				VersionRange: ">=5.0.0",
				OSArchs:      []HCTFProviderPlatform{{OS: "linux", Arch: "amd64"}},
			},
			[]HCTFProviderPlatform{{OS: "linux", Arch: "amd64"}},
			[]ProviderSpecificInstance{
				{Provider: p, Version: "5.0.0", OS: "linux", Arch: "amd64"},
				{Provider: p, Version: "5.1.0", OS: "linux", Arch: "amd64"},
			},
			false,
		},
		{
			"multiple os archs",
			ProviderMirrorConfiguration{
				Reference:    "merp",
				VersionRange: ">=5.0.0 <5.1.0",
				OSArchs:      []HCTFProviderPlatform{{OS: "linux", Arch: "amd64"}, {OS: "darwin", Arch: "arm64"}},
			},
			[]HCTFProviderPlatform{{OS: "linux", Arch: "amd64"}, {OS: "darwin", Arch: "arm64"}},
			[]ProviderSpecificInstance{
				{Provider: p, Version: "5.0.0", OS: "linux", Arch: "amd64"},
				{Provider: p, Version: "5.0.0", OS: "darwin", Arch: "arm64"},
			},
			false,
		},
		{
			"invalid semver range",
			ProviderMirrorConfiguration{
				Reference:    "merp",
				VersionRange: "not-a-range!!!",
				OSArchs:      []HCTFProviderPlatform{{OS: "linux", Arch: "amd64"}, {OS: "darwin", Arch: "arm64"}},
			},
			[]HCTFProviderPlatform{{OS: "linux", Arch: "amd64"}},
			nil,
			true,
		},
		{
			"skip a particular version",
			ProviderMirrorConfiguration{
				Reference:    "merp",
				VersionRange: ">=4.0.0",
				OSArchs:      []HCTFProviderPlatform{{OS: "linux", Arch: "amd64"}},
				SkipVersions: []string{"5.0.0"},
			},
			[]HCTFProviderPlatform{{OS: "linux", Arch: "amd64"}},
			[]ProviderSpecificInstance{
				{Provider: p, Version: "4.0.0", OS: "linux", Arch: "amd64"},
				{Provider: p, Version: "5.1.0", OS: "linux", Arch: "amd64"},
			},
			false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := p.FilterToWantedPVIs(metadata, tt.pmc, tt.osArchs)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) == 0 && len(tt.wantPVIs) == 0 {
				return
			}
			if !reflect.DeepEqual(got, tt.wantPVIs) {
				t.Errorf("got %+v, want %+v", got, tt.wantPVIs)
			}
		})
	}
}
