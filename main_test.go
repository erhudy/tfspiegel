package main

import (
	"os"
	"path/filepath"
	"testing"

	"go.uber.org/zap"
)

func TestMain(m *testing.M) {
	logger, _ := zap.NewDevelopment()
	sugar = logger.Sugar()
	os.Exit(m.Run())
}

func TestLoadConfig_ValidFS(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	configData := []byte(`
providers:
  - reference: aws
    version_range: '>=4.0.0'
    os_archs:
      - os: linux
        arch: amd64
storage_type: fs
fs_config:
  download_root: /tmp/providers
`)
	if err := os.WriteFile(configPath, configData, 0644); err != nil {
		t.Fatal(err)
	}

	config, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if config.DownloadDestination.Type != STORAGE_TYPE_FS {
		t.Errorf("expected STORAGE_TYPE_FS, got %v", config.DownloadDestination.Type)
	}
	if config.DownloadDestination.FSConfig.DownloadRoot != "/tmp/providers" {
		t.Errorf("expected download root /tmp/providers, got %s", config.DownloadDestination.FSConfig.DownloadRoot)
	}
	if len(config.Providers) != 1 {
		t.Fatalf("expected 1 provider, got %d", len(config.Providers))
	}
	if config.Providers[0].Reference != "aws" {
		t.Errorf("expected reference aws, got %s", config.Providers[0].Reference)
	}
}

func TestLoadConfig_ValidS3(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	configData := []byte(`
providers:
  - reference: aws
    version_range: '>=4.0.0'
storage_type: s3
s3_config:
  bucket: mybucket
  endpoint: https://127.0.0.1:9000
  prefix: providers
`)
	if err := os.WriteFile(configPath, configData, 0644); err != nil {
		t.Fatal(err)
	}

	config, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if config.DownloadDestination.Type != STORAGE_TYPE_S3 {
		t.Errorf("expected STORAGE_TYPE_S3, got %v", config.DownloadDestination.Type)
	}
	if config.DownloadDestination.S3Config.Bucket != "mybucket" {
		t.Errorf("expected bucket mybucket, got %s", config.DownloadDestination.S3Config.Bucket)
	}
	if config.DownloadDestination.S3Config.Endpoint != "https://127.0.0.1:9000" {
		t.Errorf("expected endpoint https://127.0.0.1:9000, got %s", config.DownloadDestination.S3Config.Endpoint)
	}
	if config.DownloadDestination.S3Config.Prefix != "providers" {
		t.Errorf("expected prefix providers, got %s", config.DownloadDestination.S3Config.Prefix)
	}
}

func TestLoadConfig_InvalidStorageType(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	configData := []byte(`
providers: []
storage_type: gcs
`)
	if err := os.WriteFile(configPath, configData, 0644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadConfig(configPath)
	if err == nil {
		t.Fatal("expected error for invalid storage type, got nil")
	}
}

func TestLoadConfig_MissingFile(t *testing.T) {
	_, err := LoadConfig("/nonexistent/path/config.yaml")
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

func TestLoadConfig_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	configData := []byte(`{{{not valid yaml`)
	if err := os.WriteFile(configPath, configData, 0644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadConfig(configPath)
	if err == nil {
		t.Fatal("expected error for invalid YAML, got nil")
	}
}
