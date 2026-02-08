package main

import (
	"archive/zip"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"golang.org/x/mod/sumdb/dirhash"
)

func createTestProvider() Provider {
	return Provider{
		Hostname: "registry.terraform.io",
		Owner:    "hashicorp",
		Name:     "aws",
	}
}

func createTestZip(t *testing.T, path string) string {
	t.Helper()

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}

	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	w := zip.NewWriter(f)
	fw, err := w.Create("terraform-provider-aws")
	if err != nil {
		t.Fatal(err)
	}
	_, err = fw.Write([]byte("fake provider binary content"))
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	hash, err := dirhash.HashZip(path, dirhash.Hash1)
	if err != nil {
		t.Fatal(err)
	}
	return hash
}

func setupFSStorage(t *testing.T) FSProviderStorageConfiguration {
	t.Helper()
	dir := t.TempDir()
	provider := createTestProvider()

	return FSProviderStorageConfiguration{
		downloadRoot: dir,
		provider:     provider,
		sugar:        sugar,
	}
}

// --- LoadCatalog tests ---

func TestFSLoadCatalog_Success(t *testing.T) {
	s := setupFSStorage(t)
	providerDir := filepath.Join(s.downloadRoot, s.provider.String())
	if err := os.MkdirAll(providerDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create a zip file
	zipPath := filepath.Join(providerDir, "terraform-provider-aws_4.0.0_linux_amd64.zip")
	hash := createTestZip(t, zipPath)

	// Create version JSON
	archives := MirrorArchives{
		Archives: map[string]MirrorProviderPlatformArch{
			"linux_amd64": {
				Hashes: []string{hash},
				URL:    "terraform-provider-aws_4.0.0_linux_amd64.zip",
			},
		},
	}
	versionJSON, _ := json.MarshalIndent(archives, "", "  ")
	if err := os.WriteFile(filepath.Join(providerDir, "4.0.0.json"), versionJSON, 0644); err != nil {
		t.Fatal(err)
	}

	// Create index.json
	index := MirrorIndex{
		Versions: map[string]map[string]any{
			"4.0.0": {},
		},
	}
	indexJSON, _ := json.MarshalIndent(index, "", "  ")
	if err := os.WriteFile(filepath.Join(providerDir, MIRROR_INDEX_FILE), indexJSON, 0644); err != nil {
		t.Fatal(err)
	}

	psibs, err := s.LoadCatalog()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(psibs) != 1 {
		t.Fatalf("expected 1 PSIB, got %d", len(psibs))
	}
	if psibs[0].Version != "4.0.0" {
		t.Errorf("expected version 4.0.0, got %s", psibs[0].Version)
	}
	if psibs[0].OS != "linux" || psibs[0].Arch != "amd64" {
		t.Errorf("expected linux_amd64, got %s_%s", psibs[0].OS, psibs[0].Arch)
	}
	if psibs[0].H1Checksum != hash {
		t.Errorf("expected hash %s, got %s", hash, psibs[0].H1Checksum)
	}
}

func TestFSLoadCatalog_MissingIndexFile(t *testing.T) {
	s := setupFSStorage(t)
	_, err := s.LoadCatalog()
	if err == nil {
		t.Fatal("expected error for missing index file, got nil")
	}
}

func TestFSLoadCatalog_MissingVersionJSON(t *testing.T) {
	s := setupFSStorage(t)
	providerDir := filepath.Join(s.downloadRoot, s.provider.String())
	if err := os.MkdirAll(providerDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create index.json referencing a version
	index := MirrorIndex{
		Versions: map[string]map[string]any{
			"4.0.0": {},
		},
	}
	indexJSON, _ := json.MarshalIndent(index, "", "  ")
	if err := os.WriteFile(filepath.Join(providerDir, MIRROR_INDEX_FILE), indexJSON, 0644); err != nil {
		t.Fatal(err)
	}

	// Don't create the version JSON file — should skip, not error
	psibs, err := s.LoadCatalog()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(psibs) != 0 {
		t.Errorf("expected 0 PSIBs when version JSON is missing, got %d", len(psibs))
	}
}

// --- VerifyCatalogAgainstStorage tests ---

func TestFSVerifyCatalog_ValidZip(t *testing.T) {
	s := setupFSStorage(t)
	providerDir := filepath.Join(s.downloadRoot, s.provider.String())

	zipPath := filepath.Join(providerDir, "terraform-provider-aws_4.0.0_linux_amd64.zip")
	hash := createTestZip(t, zipPath)

	catalog := []ProviderSpecificInstanceBinary{
		{
			ProviderSpecificInstance: ProviderSpecificInstance{
				Provider: s.provider,
				Version:  "4.0.0",
				OS:       "linux",
				Arch:     "amd64",
			},
			H1Checksum: hash,
			FullPath:   zipPath,
		},
	}

	valid, invalid, err := s.VerifyCatalogAgainstStorage(catalog)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(valid) != 1 {
		t.Errorf("expected 1 valid, got %d", len(valid))
	}
	if len(invalid) != 0 {
		t.Errorf("expected 0 invalid, got %d", len(invalid))
	}
}

func TestFSVerifyCatalog_WrongHash(t *testing.T) {
	s := setupFSStorage(t)
	providerDir := filepath.Join(s.downloadRoot, s.provider.String())

	zipPath := filepath.Join(providerDir, "terraform-provider-aws_4.0.0_linux_amd64.zip")
	createTestZip(t, zipPath)

	catalog := []ProviderSpecificInstanceBinary{
		{
			ProviderSpecificInstance: ProviderSpecificInstance{
				Provider: s.provider,
				Version:  "4.0.0",
				OS:       "linux",
				Arch:     "amd64",
			},
			H1Checksum: "h1:wrong_hash",
			FullPath:   zipPath,
		},
	}

	valid, invalid, err := s.VerifyCatalogAgainstStorage(catalog)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(valid) != 0 {
		t.Errorf("expected 0 valid, got %d", len(valid))
	}
	if len(invalid) != 1 {
		t.Errorf("expected 1 invalid, got %d", len(invalid))
	}
}

func TestFSVerifyCatalog_MissingFile(t *testing.T) {
	s := setupFSStorage(t)

	catalog := []ProviderSpecificInstanceBinary{
		{
			ProviderSpecificInstance: ProviderSpecificInstance{
				Provider: s.provider,
				Version:  "4.0.0",
				OS:       "linux",
				Arch:     "amd64",
			},
			H1Checksum: "h1:abc",
			FullPath:   filepath.Join(s.downloadRoot, "nonexistent.zip"),
		},
	}

	valid, invalid, err := s.VerifyCatalogAgainstStorage(catalog)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(valid) != 0 {
		t.Errorf("expected 0 valid, got %d", len(valid))
	}
	if len(invalid) != 1 {
		t.Errorf("expected 1 invalid, got %d", len(invalid))
	}
}

// --- WriteProviderBinaryDataToStorage tests ---

func TestFSWriteProviderBinaryData_Success(t *testing.T) {
	s := setupFSStorage(t)

	// Create a valid zip in memory
	zipPath := filepath.Join(t.TempDir(), "temp.zip")
	createTestZip(t, zipPath)
	zipData, err := os.ReadFile(zipPath)
	if err != nil {
		t.Fatal(err)
	}

	pi := ProviderSpecificInstance{
		Provider: s.provider,
		Version:  "4.0.0",
		OS:       "linux",
		Arch:     "amd64",
	}

	psib, err := s.WriteProviderBinaryDataToStorage(zipData, pi)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if psib == nil {
		t.Fatal("expected non-nil PSIB")
	}

	// Verify the file was written
	expectedPath := filepath.Join(s.downloadRoot, pi.GetDownloadBase(), pi.GetDownloadedFileName())
	if psib.FullPath != expectedPath {
		t.Errorf("expected path %s, got %s", expectedPath, psib.FullPath)
	}
	if _, err := os.Stat(psib.FullPath); os.IsNotExist(err) {
		t.Error("expected file to exist on disk")
	}
	if psib.H1Checksum == "" {
		t.Error("expected non-empty H1Checksum")
	}
}

func TestFSWriteProviderBinaryData_InvalidZip(t *testing.T) {
	s := setupFSStorage(t)

	pi := ProviderSpecificInstance{
		Provider: s.provider,
		Version:  "4.0.0",
		OS:       "linux",
		Arch:     "amd64",
	}

	_, err := s.WriteProviderBinaryDataToStorage([]byte("not a zip"), pi)
	if err == nil {
		t.Fatal("expected error for invalid zip data, got nil")
	}
}

// --- StoreCatalog tests ---

func TestFSStoreCatalog_MultipleVersions(t *testing.T) {
	s := setupFSStorage(t)
	providerDir := filepath.Join(s.downloadRoot, s.provider.GetDownloadBase())
	if err := os.MkdirAll(providerDir, 0755); err != nil {
		t.Fatal(err)
	}

	psibs := []ProviderSpecificInstanceBinary{
		{
			ProviderSpecificInstance: ProviderSpecificInstance{
				Provider: s.provider,
				Version:  "4.0.0",
				OS:       "linux",
				Arch:     "amd64",
			},
			H1Checksum: "h1:abc123",
		},
		{
			ProviderSpecificInstance: ProviderSpecificInstance{
				Provider: s.provider,
				Version:  "4.0.0",
				OS:       "darwin",
				Arch:     "arm64",
			},
			H1Checksum: "h1:def456",
		},
		{
			ProviderSpecificInstance: ProviderSpecificInstance{
				Provider: s.provider,
				Version:  "4.1.0",
				OS:       "linux",
				Arch:     "amd64",
			},
			H1Checksum: "h1:ghi789",
		},
	}

	err := s.StoreCatalog(psibs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify index.json
	indexPath := filepath.Join(providerDir, MIRROR_INDEX_FILE)
	indexData, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatalf("failed to read index.json: %v", err)
	}
	var index MirrorIndex
	if err := json.Unmarshal(indexData, &index); err != nil {
		t.Fatalf("failed to unmarshal index.json: %v", err)
	}
	if len(index.Versions) != 2 {
		t.Errorf("expected 2 versions in index, got %d", len(index.Versions))
	}

	// Verify version 4.0.0 JSON
	v400Data, err := os.ReadFile(filepath.Join(providerDir, "4.0.0.json"))
	if err != nil {
		t.Fatalf("failed to read 4.0.0.json: %v", err)
	}
	var v400 MirrorArchives
	if err := json.Unmarshal(v400Data, &v400); err != nil {
		t.Fatalf("failed to unmarshal 4.0.0.json: %v", err)
	}
	if len(v400.Archives) != 2 {
		t.Errorf("expected 2 archives in 4.0.0, got %d", len(v400.Archives))
	}

	// Verify version 4.1.0 JSON
	v410Data, err := os.ReadFile(filepath.Join(providerDir, "4.1.0.json"))
	if err != nil {
		t.Fatalf("failed to read 4.1.0.json: %v", err)
	}
	var v410 MirrorArchives
	if err := json.Unmarshal(v410Data, &v410); err != nil {
		t.Fatalf("failed to unmarshal 4.1.0.json: %v", err)
	}
	if len(v410.Archives) != 1 {
		t.Errorf("expected 1 archive in 4.1.0, got %d", len(v410.Archives))
	}
}

func TestFSStoreCatalog_EmptyPSIBs(t *testing.T) {
	s := setupFSStorage(t)
	providerDir := filepath.Join(s.downloadRoot, s.provider.GetDownloadBase())
	if err := os.MkdirAll(providerDir, 0755); err != nil {
		t.Fatal(err)
	}

	err := s.StoreCatalog(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify index.json written with empty versions
	indexPath := filepath.Join(providerDir, MIRROR_INDEX_FILE)
	indexData, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatalf("failed to read index.json: %v", err)
	}
	var index MirrorIndex
	if err := json.Unmarshal(indexData, &index); err != nil {
		t.Fatalf("failed to unmarshal index.json: %v", err)
	}
	if len(index.Versions) != 0 {
		t.Errorf("expected 0 versions in index, got %d", len(index.Versions))
	}
}

// --- Round-trip integration test ---

func TestFSStoreCatalogThenLoadCatalog_RoundTrip(t *testing.T) {
	s := setupFSStorage(t)
	providerDir := filepath.Join(s.downloadRoot, s.provider.String())
	if err := os.MkdirAll(providerDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create actual zip files so LoadCatalog can resolve paths
	zipPath1 := filepath.Join(providerDir, "terraform-provider-aws_4.0.0_linux_amd64.zip")
	hash1 := createTestZip(t, zipPath1)

	zipPath2 := filepath.Join(providerDir, "terraform-provider-aws_4.0.0_darwin_arm64.zip")
	hash2 := createTestZip(t, zipPath2)

	originalPSIBs := []ProviderSpecificInstanceBinary{
		{
			ProviderSpecificInstance: ProviderSpecificInstance{
				Provider: s.provider,
				Version:  "4.0.0",
				OS:       "linux",
				Arch:     "amd64",
			},
			H1Checksum: hash1,
			FullPath:   zipPath1,
		},
		{
			ProviderSpecificInstance: ProviderSpecificInstance{
				Provider: s.provider,
				Version:  "4.0.0",
				OS:       "darwin",
				Arch:     "arm64",
			},
			H1Checksum: hash2,
			FullPath:   zipPath2,
		},
	}

	// Store
	if err := s.StoreCatalog(originalPSIBs); err != nil {
		t.Fatalf("StoreCatalog error: %v", err)
	}

	// Load
	loadedPSIBs, err := s.LoadCatalog()
	if err != nil {
		t.Fatalf("LoadCatalog error: %v", err)
	}

	if len(loadedPSIBs) != len(originalPSIBs) {
		t.Fatalf("expected %d PSIBs, got %d", len(originalPSIBs), len(loadedPSIBs))
	}

	// Sort both for comparison
	sortPSIBs := func(s []ProviderSpecificInstanceBinary) {
		sort.Slice(s, func(i, j int) bool {
			return s[i].ProviderSpecificInstance.String() < s[j].ProviderSpecificInstance.String()
		})
	}
	sortPSIBs(originalPSIBs)
	sortPSIBs(loadedPSIBs)

	for i := range originalPSIBs {
		if loadedPSIBs[i].Version != originalPSIBs[i].Version {
			t.Errorf("version mismatch at %d: got %s, want %s", i, loadedPSIBs[i].Version, originalPSIBs[i].Version)
		}
		if loadedPSIBs[i].OS != originalPSIBs[i].OS {
			t.Errorf("OS mismatch at %d: got %s, want %s", i, loadedPSIBs[i].OS, originalPSIBs[i].OS)
		}
		if loadedPSIBs[i].Arch != originalPSIBs[i].Arch {
			t.Errorf("Arch mismatch at %d: got %s, want %s", i, loadedPSIBs[i].Arch, originalPSIBs[i].Arch)
		}
		if loadedPSIBs[i].H1Checksum != originalPSIBs[i].H1Checksum {
			t.Errorf("H1Checksum mismatch at %d: got %s, want %s", i, loadedPSIBs[i].H1Checksum, originalPSIBs[i].H1Checksum)
		}
	}
}
