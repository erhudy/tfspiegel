package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	tests := []struct {
		name        string
		yaml        string
		wantErr     bool
		checkConfig func(t *testing.T, c Configuration)
	}{
		{
			name: "valid FS config",
			yaml: `
storage_type: fs
fs_config:
  download_root: /tmp/mirror
providers:
  - reference: aws
    version_range: ">=5.0.0"
    os_archs:
      - os: linux
        arch: amd64
`,
			wantErr: false,
			checkConfig: func(t *testing.T, c Configuration) {
				if c.DownloadDestination.Type != STORAGE_TYPE_FS {
					t.Errorf("expected STORAGE_TYPE_FS, got %v", c.DownloadDestination.Type)
				}
				if c.DownloadDestination.FSConfig.DownloadRoot != "/tmp/mirror" {
					t.Errorf("unexpected download root: %s", c.DownloadDestination.FSConfig.DownloadRoot)
				}
				if len(c.Providers) != 1 {
					t.Fatalf("expected 1 provider, got %d", len(c.Providers))
				}
				if c.Providers[0].Reference != "aws" {
					t.Errorf("unexpected provider reference: %s", c.Providers[0].Reference)
				}
			},
		},
		{
			name: "valid S3 config",
			yaml: `
storage_type: s3
s3_config:
  bucket: my-bucket
  prefix: providers
providers:
  - reference: aws
    version_range: ">=5.0.0"
`,
			wantErr: false,
			checkConfig: func(t *testing.T, c Configuration) {
				if c.DownloadDestination.Type != STORAGE_TYPE_S3 {
					t.Errorf("expected STORAGE_TYPE_S3, got %v", c.DownloadDestination.Type)
				}
				if c.DownloadDestination.S3Config.Bucket != "my-bucket" {
					t.Errorf("unexpected bucket: %s", c.DownloadDestination.S3Config.Bucket)
				}
				if c.DownloadDestination.S3Config.Prefix != "providers" {
					t.Errorf("unexpected prefix: %s", c.DownloadDestination.S3Config.Prefix)
				}
			},
		},
		{
			name: "case insensitive storage type",
			yaml: `
storage_type: FS
providers: []
fs_config:
  download_root: /tmp
`,
			wantErr: false,
			checkConfig: func(t *testing.T, c Configuration) {
				if c.DownloadDestination.Type != STORAGE_TYPE_FS {
					t.Errorf("expected STORAGE_TYPE_FS, got %v", c.DownloadDestination.Type)
				}
			},
		},
		{
			name: "unknown storage type",
			yaml: `
storage_type: gcs
providers: []
`,
			wantErr: true,
		},
		{
			name:    "invalid YAML",
			yaml:    "{{invalid yaml content",
			wantErr: true,
		},
		{
			name: "multiple providers",
			yaml: `
storage_type: fs
fs_config:
  download_root: /tmp
providers:
  - reference: aws
    version_range: ">=5.0.0"
  - reference: gavinbunney/kubectl
    version_range: ">=1.0.0"
  - reference: custom.io/myorg/myprovider
    version_range: ">=2.0.0"
`,
			wantErr: false,
			checkConfig: func(t *testing.T, c Configuration) {
				if len(c.Providers) != 3 {
					t.Fatalf("expected 3 providers, got %d", len(c.Providers))
				}
				if c.Providers[0].Reference != "aws" {
					t.Errorf("provider 0: got %s, want aws", c.Providers[0].Reference)
				}
				if c.Providers[1].Reference != "gavinbunney/kubectl" {
					t.Errorf("provider 1: got %s, want gavinbunney/kubectl", c.Providers[1].Reference)
				}
				if c.Providers[2].Reference != "custom.io/myorg/myprovider" {
					t.Errorf("provider 2: got %s, want custom.io/myorg/myprovider", c.Providers[2].Reference)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			configPath := filepath.Join(dir, "config.yaml")
			err := os.WriteFile(configPath, []byte(tt.yaml), 0644)
			if err != nil {
				t.Fatalf("failed to write config file: %v", err)
			}

			config, err := LoadConfig(configPath)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.checkConfig != nil {
				tt.checkConfig(t, config)
			}
		})
	}
}

func TestLoadConfigMissingFile(t *testing.T) {
	_, err := LoadConfig("/nonexistent/path/config.yaml")
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}
