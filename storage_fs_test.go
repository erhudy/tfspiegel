package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"go.uber.org/zap"
	"golang.org/x/mod/sumdb/dirhash"
)

func testSugar() *zap.SugaredLogger {
	logger, _ := zap.NewDevelopment()
	return logger.Sugar()
}

func testProvider() Provider {
	return Provider{Hostname: "registry.terraform.io", Owner: "hashicorp", Name: "aws"}
}

// createTestZip creates a minimal valid ZIP file and returns its bytes and h1 hash.
func createTestZip(t *testing.T, filename, content string) ([]byte, string) {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	f, err := w.Create(filename)
	if err != nil {
		t.Fatalf("failed to create zip entry: %v", err)
	}
	_, err = f.Write([]byte(content))
	if err != nil {
		t.Fatalf("failed to write zip entry: %v", err)
	}
	err = w.Close()
	if err != nil {
		t.Fatalf("failed to close zip writer: %v", err)
	}

	// Write to temp file to compute hash
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "test.zip")
	err = os.WriteFile(zipPath, buf.Bytes(), 0644)
	if err != nil {
		t.Fatalf("failed to write temp zip: %v", err)
	}

	hash, err := dirhash.HashZip(zipPath, dirhash.Hash1)
	if err != nil {
		t.Fatalf("failed to hash zip: %v", err)
	}

	return buf.Bytes(), hash
}

// writeTestZipToPath writes a valid zip at the given path and returns the h1 hash.
func writeTestZipToPath(t *testing.T, path, entryName, content string) string {
	t.Helper()
	zipBytes, hash := createTestZip(t, entryName, content)
	dir := filepath.Dir(path)
	err := os.MkdirAll(dir, 0755)
	if err != nil {
		t.Fatalf("failed to create dir: %v", err)
	}
	err = os.WriteFile(path, zipBytes, 0644)
	if err != nil {
		t.Fatalf("failed to write zip: %v", err)
	}
	return hash
}

func TestFSLoadCatalog(t *testing.T) {
	provider := testProvider()

	t.Run("valid catalog", func(t *testing.T) {
		root := t.TempDir()
		providerDir := filepath.Join(root, provider.String())
		err := os.MkdirAll(providerDir, 0755)
		if err != nil {
			t.Fatal(err)
		}

		zipFile := "terraform-provider-aws_5.0.0_linux_amd64.zip"
		zipPath := filepath.Join(providerDir, zipFile)
		hash := writeTestZipToPath(t, zipPath, "provider.exe", "binary")

		index := MirrorIndex{Versions: map[string]map[string]any{"5.0.0": {}}}
		indexBytes, _ := json.Marshal(index)
		if err := os.WriteFile(filepath.Join(providerDir, "index.json"), indexBytes, 0644); err != nil {
			t.Fatalf("failed to write index.json: %v", err)
		}

		archives := MirrorArchives{
			Archives: map[string]MirrorProviderPlatformArch{
				"linux_amd64": {Hashes: []string{hash}, URL: zipFile},
			},
		}
		archivesBytes, _ := json.Marshal(archives)
		if err := os.WriteFile(filepath.Join(providerDir, "5.0.0.json"), archivesBytes, 0644); err != nil {
			t.Fatalf("failed to write 5.0.0.json: %v", err)
		}

		s := FSProviderStorageConfiguration{downloadRoot: root, provider: provider, sugar: testSugar()}
		psibs, err := s.LoadCatalog()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(psibs) != 1 {
			t.Fatalf("expected 1 psib, got %d", len(psibs))
		}
		if psibs[0].Version != "5.0.0" {
			t.Errorf("version = %s, want 5.0.0", psibs[0].Version)
		}
		if psibs[0].OS != "linux" || psibs[0].Arch != "amd64" {
			t.Errorf("os/arch = %s/%s, want linux/amd64", psibs[0].OS, psibs[0].Arch)
		}
	})

	t.Run("missing index.json", func(t *testing.T) {
		root := t.TempDir()
		s := FSProviderStorageConfiguration{downloadRoot: root, provider: provider, sugar: testSugar()}
		_, err := s.LoadCatalog()
		if err == nil {
			t.Fatal("expected error for missing index.json")
		}
	})

	t.Run("malformed index JSON", func(t *testing.T) {
		root := t.TempDir()
		providerDir := filepath.Join(root, provider.String())
		if err := os.MkdirAll(providerDir, 0755); err != nil {
			t.Fatalf("failed to create dir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(providerDir, "index.json"), []byte("{bad json"), 0644); err != nil {
			t.Fatalf("failed to write index.json: %v", err)
		}

		s := FSProviderStorageConfiguration{downloadRoot: root, provider: provider, sugar: testSugar()}
		_, err := s.LoadCatalog()
		if err == nil {
			t.Fatal("expected error for malformed JSON")
		}
	})

	t.Run("missing version JSON continues with partial results", func(t *testing.T) {
		root := t.TempDir()
		providerDir := filepath.Join(root, provider.String())
		if err := os.MkdirAll(providerDir, 0755); err != nil {
			t.Fatalf("failed to create dir: %v", err)
		}

		index := MirrorIndex{Versions: map[string]map[string]any{"5.0.0": {}, "5.1.0": {}}}
		indexBytes, _ := json.Marshal(index)
		if err := os.WriteFile(filepath.Join(providerDir, "index.json"), indexBytes, 0644); err != nil {
			t.Fatalf("failed to write index.json: %v", err)
		}

		// Only write version JSON for 5.1.0
		zipFile := "terraform-provider-aws_5.1.0_linux_amd64.zip"
		zipPath := filepath.Join(providerDir, zipFile)
		hash := writeTestZipToPath(t, zipPath, "provider.exe", "binary")

		archives := MirrorArchives{
			Archives: map[string]MirrorProviderPlatformArch{
				"linux_amd64": {Hashes: []string{hash}, URL: zipFile},
			},
		}
		archivesBytes, _ := json.Marshal(archives)
		if err := os.WriteFile(filepath.Join(providerDir, "5.1.0.json"), archivesBytes, 0644); err != nil {
			t.Fatalf("failed to write 5.1.0.json: %v", err)
		}

		s := FSProviderStorageConfiguration{downloadRoot: root, provider: provider, sugar: testSugar()}
		psibs, err := s.LoadCatalog()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(psibs) != 1 {
			t.Fatalf("expected 1 psib (partial), got %d", len(psibs))
		}
		if psibs[0].Version != "5.1.0" {
			t.Errorf("expected version 5.1.0, got %s", psibs[0].Version)
		}
	})

	t.Run("archive with multiple hashes is skipped", func(t *testing.T) {
		root := t.TempDir()
		providerDir := filepath.Join(root, provider.String())
		if err := os.MkdirAll(providerDir, 0755); err != nil {
			t.Fatalf("failed to create dir: %v", err)
		}

		index := MirrorIndex{Versions: map[string]map[string]any{"5.0.0": {}}}
		indexBytes, _ := json.Marshal(index)
		if err := os.WriteFile(filepath.Join(providerDir, "index.json"), indexBytes, 0644); err != nil {
			t.Fatalf("failed to write index.json: %v", err)
		}

		archives := MirrorArchives{
			Archives: map[string]MirrorProviderPlatformArch{
				"linux_amd64": {Hashes: []string{"h1:abc", "h1:def"}, URL: "test.zip"},
			},
		}
		archivesBytes, _ := json.Marshal(archives)
		if err := os.WriteFile(filepath.Join(providerDir, "5.0.0.json"), archivesBytes, 0644); err != nil {
			t.Fatalf("failed to write 5.0.0.json: %v", err)
		}

		s := FSProviderStorageConfiguration{downloadRoot: root, provider: provider, sugar: testSugar()}
		psibs, err := s.LoadCatalog()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(psibs) != 0 {
			t.Errorf("expected 0 psibs (skipped), got %d", len(psibs))
		}
	})

	t.Run("archive key without underscore delimiter is skipped", func(t *testing.T) {
		root := t.TempDir()
		providerDir := filepath.Join(root, provider.String())
		if err := os.MkdirAll(providerDir, 0755); err != nil {
			t.Fatalf("failed to create dir: %v", err)
		}

		index := MirrorIndex{Versions: map[string]map[string]any{"5.0.0": {}}}
		indexBytes, _ := json.Marshal(index)
		if err := os.WriteFile(filepath.Join(providerDir, "index.json"), indexBytes, 0644); err != nil {
			t.Fatalf("failed to write index.json: %v", err)
		}

		archives := MirrorArchives{
			Archives: map[string]MirrorProviderPlatformArch{
				"linuxamd64": {Hashes: []string{"h1:abc"}, URL: "test.zip"},
			},
		}
		archivesBytes, _ := json.Marshal(archives)
		if err := os.WriteFile(filepath.Join(providerDir, "5.0.0.json"), archivesBytes, 0644); err != nil {
			t.Fatalf("failed to write 5.0.0.json: %v", err)
		}

		s := FSProviderStorageConfiguration{downloadRoot: root, provider: provider, sugar: testSugar()}
		psibs, err := s.LoadCatalog()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(psibs) != 0 {
			t.Errorf("expected 0 psibs (skipped), got %d", len(psibs))
		}
	})
}

func TestFSVerifyCatalogAgainstStorage(t *testing.T) {
	provider := testProvider()

	t.Run("valid zip with matching hash", func(t *testing.T) {
		root := t.TempDir()
		providerDir := filepath.Join(root, provider.String())
		zipFile := "terraform-provider-aws_5.0.0_linux_amd64.zip"
		zipPath := filepath.Join(providerDir, zipFile)
		hash := writeTestZipToPath(t, zipPath, "provider.exe", "binary")

		catalog := []ProviderSpecificInstanceBinary{
			{
				ProviderSpecificInstance: ProviderSpecificInstance{
					Provider: provider, Version: "5.0.0", OS: "linux", Arch: "amd64",
				},
				H1Checksum: hash,
				FullPath:   zipPath,
			},
		}

		s := FSProviderStorageConfiguration{downloadRoot: root, provider: provider, sugar: testSugar()}
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
	})

	t.Run("missing file goes to invalid", func(t *testing.T) {
		root := t.TempDir()
		catalog := []ProviderSpecificInstanceBinary{
			{
				ProviderSpecificInstance: ProviderSpecificInstance{
					Provider: provider, Version: "5.0.0", OS: "linux", Arch: "amd64",
				},
				H1Checksum: "h1:doesntmatter",
				FullPath:   filepath.Join(root, "nonexistent.zip"),
			},
		}

		s := FSProviderStorageConfiguration{downloadRoot: root, provider: provider, sugar: testSugar()}
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
	})

	t.Run("checksum mismatch goes to invalid", func(t *testing.T) {
		root := t.TempDir()
		providerDir := filepath.Join(root, provider.String())
		zipFile := "terraform-provider-aws_5.0.0_linux_amd64.zip"
		zipPath := filepath.Join(providerDir, zipFile)
		writeTestZipToPath(t, zipPath, "provider.exe", "binary")

		catalog := []ProviderSpecificInstanceBinary{
			{
				ProviderSpecificInstance: ProviderSpecificInstance{
					Provider: provider, Version: "5.0.0", OS: "linux", Arch: "amd64",
				},
				H1Checksum: "h1:wrongchecksum",
				FullPath:   zipPath,
			},
		}

		s := FSProviderStorageConfiguration{downloadRoot: root, provider: provider, sugar: testSugar()}
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
	})

	t.Run("mixed valid and invalid", func(t *testing.T) {
		root := t.TempDir()
		providerDir := filepath.Join(root, provider.String())
		zipFile := "terraform-provider-aws_5.0.0_linux_amd64.zip"
		zipPath := filepath.Join(providerDir, zipFile)
		hash := writeTestZipToPath(t, zipPath, "provider.exe", "binary")

		catalog := []ProviderSpecificInstanceBinary{
			{
				ProviderSpecificInstance: ProviderSpecificInstance{
					Provider: provider, Version: "5.0.0", OS: "linux", Arch: "amd64",
				},
				H1Checksum: hash,
				FullPath:   zipPath,
			},
			{
				ProviderSpecificInstance: ProviderSpecificInstance{
					Provider: provider, Version: "5.1.0", OS: "linux", Arch: "amd64",
				},
				H1Checksum: "h1:doesntexist",
				FullPath:   filepath.Join(root, "nonexistent.zip"),
			},
		}

		s := FSProviderStorageConfiguration{downloadRoot: root, provider: provider, sugar: testSugar()}
		valid, invalid, err := s.VerifyCatalogAgainstStorage(catalog)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(valid) != 1 {
			t.Errorf("expected 1 valid, got %d", len(valid))
		}
		if len(invalid) != 1 {
			t.Errorf("expected 1 invalid, got %d", len(invalid))
		}
	})
}

func TestFSWriteProviderBinaryDataToStorage(t *testing.T) {
	provider := testProvider()

	t.Run("valid zip bytes written and hash returned", func(t *testing.T) {
		root := t.TempDir()
		zipBytes, expectedHash := createTestZip(t, "provider.exe", "binary content")

		pi := ProviderSpecificInstance{
			Provider: provider, Version: "5.0.0", OS: "linux", Arch: "amd64",
		}

		s := FSProviderStorageConfiguration{downloadRoot: root, provider: provider, sugar: testSugar()}
		psib, err := s.WriteProviderBinaryDataToStorage(zipBytes, pi)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if psib.H1Checksum != expectedHash {
			t.Errorf("hash = %s, want %s", psib.H1Checksum, expectedHash)
		}

		expectedPath := filepath.Join(root, pi.GetDownloadBase(), pi.GetDownloadedFileName())
		if psib.FullPath != expectedPath {
			t.Errorf("path = %s, want %s", psib.FullPath, expectedPath)
		}

		// Verify file actually exists
		if _, err := os.Stat(psib.FullPath); os.IsNotExist(err) {
			t.Error("written file does not exist")
		}
	})

	t.Run("creates nested directories", func(t *testing.T) {
		root := t.TempDir()
		zipBytes, _ := createTestZip(t, "provider.exe", "binary")

		pi := ProviderSpecificInstance{
			Provider: provider, Version: "5.0.0", OS: "linux", Arch: "amd64",
		}

		s := FSProviderStorageConfiguration{downloadRoot: root, provider: provider, sugar: testSugar()}
		_, err := s.WriteProviderBinaryDataToStorage(zipBytes, pi)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		dirPath := filepath.Join(root, pi.GetDownloadBase())
		info, err := os.Stat(dirPath)
		if err != nil {
			t.Fatalf("directory not created: %v", err)
		}
		if !info.IsDir() {
			t.Error("expected directory")
		}
	})

	t.Run("non-zip data returns error", func(t *testing.T) {
		root := t.TempDir()
		pi := ProviderSpecificInstance{
			Provider: provider, Version: "5.0.0", OS: "linux", Arch: "amd64",
		}

		s := FSProviderStorageConfiguration{downloadRoot: root, provider: provider, sugar: testSugar()}
		_, err := s.WriteProviderBinaryDataToStorage([]byte("not a zip"), pi)
		if err == nil {
			t.Fatal("expected error for non-zip data")
		}
	})
}

func TestFSStoreCatalog(t *testing.T) {
	provider := testProvider()

	t.Run("single version single arch", func(t *testing.T) {
		root := t.TempDir()
		providerDir := filepath.Join(root, provider.GetDownloadBase())
		if err := os.MkdirAll(providerDir, 0755); err != nil {
			t.Fatalf("failed to create dir: %v", err)
		}

		psibs := []ProviderSpecificInstanceBinary{
			{
				ProviderSpecificInstance: ProviderSpecificInstance{
					Provider: provider, Version: "5.0.0", OS: "linux", Arch: "amd64",
				},
				H1Checksum: "h1:abc123",
			},
		}

		s := FSProviderStorageConfiguration{downloadRoot: root, provider: provider, sugar: testSugar()}
		err := s.StoreCatalog(psibs)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Verify index.json
		indexPath := filepath.Join(providerDir, "index.json")
		indexBytes, err := os.ReadFile(indexPath)
		if err != nil {
			t.Fatalf("failed to read index.json: %v", err)
		}
		var index MirrorIndex
		if err := json.Unmarshal(indexBytes, &index); err != nil {
			t.Fatalf("failed to unmarshal index: %v", err)
		}
		if _, ok := index.Versions["5.0.0"]; !ok {
			t.Error("index.json missing version 5.0.0")
		}

		// Verify version JSON
		versionPath := filepath.Join(providerDir, "5.0.0.json")
		versionBytes, err := os.ReadFile(versionPath)
		if err != nil {
			t.Fatalf("failed to read 5.0.0.json: %v", err)
		}
		var archives MirrorArchives
		if err := json.Unmarshal(versionBytes, &archives); err != nil {
			t.Fatalf("failed to unmarshal archives: %v", err)
		}
		arch, ok := archives.Archives["linux_amd64"]
		if !ok {
			t.Fatal("missing linux_amd64 in archives")
		}
		if len(arch.Hashes) != 1 || arch.Hashes[0] != "h1:abc123" {
			t.Errorf("unexpected hashes: %v", arch.Hashes)
		}
	})

	t.Run("multiple versions", func(t *testing.T) {
		root := t.TempDir()
		providerDir := filepath.Join(root, provider.GetDownloadBase())
		if err := os.MkdirAll(providerDir, 0755); err != nil {
			t.Fatalf("failed to create dir: %v", err)
		}

		psibs := []ProviderSpecificInstanceBinary{
			{
				ProviderSpecificInstance: ProviderSpecificInstance{
					Provider: provider, Version: "5.0.0", OS: "linux", Arch: "amd64",
				},
				H1Checksum: "h1:abc",
			},
			{
				ProviderSpecificInstance: ProviderSpecificInstance{
					Provider: provider, Version: "5.1.0", OS: "linux", Arch: "amd64",
				},
				H1Checksum: "h1:def",
			},
		}

		s := FSProviderStorageConfiguration{downloadRoot: root, provider: provider, sugar: testSugar()}
		err := s.StoreCatalog(psibs)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Verify index has both versions
		indexBytes, _ := os.ReadFile(filepath.Join(providerDir, "index.json"))
		var index MirrorIndex
		if err := json.Unmarshal(indexBytes, &index); err != nil {
			t.Fatalf("failed to unmarshal index: %v", err)
		}
		if len(index.Versions) != 2 {
			t.Errorf("expected 2 versions in index, got %d", len(index.Versions))
		}

		// Verify both version JSONs exist
		for _, ver := range []string{"5.0.0", "5.1.0"} {
			if _, err := os.Stat(filepath.Join(providerDir, ver+".json")); os.IsNotExist(err) {
				t.Errorf("missing %s.json", ver)
			}
		}
	})

	t.Run("multiple archs same version", func(t *testing.T) {
		root := t.TempDir()
		providerDir := filepath.Join(root, provider.GetDownloadBase())
		if err := os.MkdirAll(providerDir, 0755); err != nil {
			t.Fatalf("failed to create dir: %v", err)
		}

		psibs := []ProviderSpecificInstanceBinary{
			{
				ProviderSpecificInstance: ProviderSpecificInstance{
					Provider: provider, Version: "5.0.0", OS: "linux", Arch: "amd64",
				},
				H1Checksum: "h1:abc",
			},
			{
				ProviderSpecificInstance: ProviderSpecificInstance{
					Provider: provider, Version: "5.0.0", OS: "darwin", Arch: "arm64",
				},
				H1Checksum: "h1:def",
			},
		}

		s := FSProviderStorageConfiguration{downloadRoot: root, provider: provider, sugar: testSugar()}
		err := s.StoreCatalog(psibs)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		versionBytes, _ := os.ReadFile(filepath.Join(providerDir, "5.0.0.json"))
		var archives MirrorArchives
		if err := json.Unmarshal(versionBytes, &archives); err != nil {
			t.Fatalf("failed to unmarshal archives: %v", err)
		}
		if len(archives.Archives) != 2 {
			t.Errorf("expected 2 archives, got %d", len(archives.Archives))
		}
		if _, ok := archives.Archives["linux_amd64"]; !ok {
			t.Error("missing linux_amd64")
		}
		if _, ok := archives.Archives["darwin_arm64"]; !ok {
			t.Error("missing darwin_arm64")
		}
	})
}

func TestFSReconcileWantedProviderInstances(t *testing.T) {
	provider := testProvider()
	psi := ProviderSpecificInstance{Provider: provider, Version: "5.0.0", OS: "linux", Arch: "amd64"}
	psib := ProviderSpecificInstanceBinary{ProviderSpecificInstance: psi, H1Checksum: "h1:abc"}

	s := FSProviderStorageConfiguration{downloadRoot: "/tmp", provider: provider, sugar: testSugar()}
	got := s.ReconcileWantedProviderInstances(
		[]ProviderSpecificInstanceBinary{psib},
		nil,
		[]ProviderSpecificInstance{psi},
	)
	if len(got) != 0 {
		t.Errorf("expected 0 (already valid), got %d", len(got))
	}
}
