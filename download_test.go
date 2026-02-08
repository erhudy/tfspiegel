package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHCTFProviderPlatformString(t *testing.T) {
	pp := HCTFProviderPlatform{OS: "linux", Arch: "amd64"}
	expected := "linux_amd64"
	if got := pp.String(); got != expected {
		t.Errorf("String() = %q, want %q", got, expected)
	}
}

func TestFilterVersionsWithFailedPSIBs_NoFailures(t *testing.T) {
	psibs := []ProviderSpecificInstanceBinary{
		{
			ProviderSpecificInstance: ProviderSpecificInstance{
				Provider: Provider{Hostname: "r", Owner: "o", Name: "n"},
				Version:  "1.0.0", OS: "linux", Arch: "amd64",
			},
			H1Checksum: "h1:abc",
		},
	}

	result := FilterVersionsWithFailedPSIBs(psibs, nil)
	if len(result) != 1 {
		t.Errorf("expected 1 result, got %d", len(result))
	}
}

func TestFilterVersionsWithFailedPSIBs_OneVersionFailed(t *testing.T) {
	psibs := []ProviderSpecificInstanceBinary{
		{
			ProviderSpecificInstance: ProviderSpecificInstance{
				Provider: Provider{Hostname: "r", Owner: "o", Name: "n"},
				Version:  "1.0.0", OS: "linux", Arch: "amd64",
			},
		},
		{
			ProviderSpecificInstance: ProviderSpecificInstance{
				Provider: Provider{Hostname: "r", Owner: "o", Name: "n"},
				Version:  "2.0.0", OS: "linux", Arch: "amd64",
			},
		},
	}
	failed := []ProviderSpecificInstance{
		{Provider: Provider{Hostname: "r", Owner: "o", Name: "n"}, Version: "1.0.0", OS: "darwin", Arch: "arm64"},
	}

	result := FilterVersionsWithFailedPSIBs(psibs, failed)
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	if result[0].Version != "2.0.0" {
		t.Errorf("expected version 2.0.0 to remain, got %s", result[0].Version)
	}
}

func TestFilterVersionsWithFailedPSIBs_AllFailed(t *testing.T) {
	psibs := []ProviderSpecificInstanceBinary{
		{
			ProviderSpecificInstance: ProviderSpecificInstance{
				Provider: Provider{Hostname: "r", Owner: "o", Name: "n"},
				Version:  "1.0.0", OS: "linux", Arch: "amd64",
			},
		},
	}
	failed := []ProviderSpecificInstance{
		{Provider: Provider{Hostname: "r", Owner: "o", Name: "n"}, Version: "1.0.0", OS: "linux", Arch: "amd64"},
	}

	result := FilterVersionsWithFailedPSIBs(psibs, failed)
	if len(result) != 0 {
		t.Errorf("expected 0 results, got %d", len(result))
	}
}

func TestFilterVersionsWithFailedPSIBs_EmptyInputs(t *testing.T) {
	result := FilterVersionsWithFailedPSIBs(nil, nil)
	if len(result) != 0 {
		t.Errorf("expected 0 results, got %d", len(result))
	}
}

// mockProviderStorer implements the ProviderStorer interface for testing
type mockProviderStorer struct {
	writeFunc func(binaryData []byte, pi ProviderSpecificInstance) (*ProviderSpecificInstanceBinary, error)
}

func (m mockProviderStorer) LoadCatalog() ([]ProviderSpecificInstanceBinary, error) {
	panic("not implemented")
}

func (m mockProviderStorer) VerifyCatalogAgainstStorage(catalog []ProviderSpecificInstanceBinary) ([]ProviderSpecificInstanceBinary, []ProviderSpecificInstanceBinary, error) {
	panic("not implemented")
}

func (m mockProviderStorer) ReconcileWantedProviderInstances(validPSIBs []ProviderSpecificInstanceBinary, invalidPSIBs []ProviderSpecificInstanceBinary, wantedProviderInstances []ProviderSpecificInstance) []ProviderSpecificInstance {
	panic("not implemented")
}

func (m mockProviderStorer) WriteProviderBinaryDataToStorage(binaryData []byte, pi ProviderSpecificInstance) (*ProviderSpecificInstanceBinary, error) {
	return m.writeFunc(binaryData, pi)
}

func (m mockProviderStorer) StoreCatalog(psibs []ProviderSpecificInstanceBinary) error {
	panic("not implemented")
}

func TestMirrorProviderInstanceToDest_Success(t *testing.T) {
	// Disable retry sleep for tests
	oldSleep := retrySleepFunc
	retrySleepFunc = func(d time.Duration) {}
	defer func() { retrySleepFunc = oldSleep }()

	// Create test binary data and compute its SHA256
	binaryData := []byte("fake zip file content for testing")
	hasher := sha256.New()
	hasher.Write(binaryData)
	checksum := fmt.Sprintf("%x", hasher.Sum(nil))

	// Mock download server - serves both the registry response and the binary
	mux := http.NewServeMux()

	registryResponse := HCTFRegistryDownloadResponse{
		Shasum: checksum,
	}

	mux.HandleFunc("/v1/providers/hashicorp/testprov/1.0.0/download/linux/amd64", func(w http.ResponseWriter, r *http.Request) {
		// Set download_url to point to our own server
		registryResponse.DownloadURL = fmt.Sprintf("http://%s/download/testprov.zip", r.Host)
		json.NewEncoder(w).Encode(registryResponse)
	})
	mux.HandleFunc("/download/testprov.zip", func(w http.ResponseWriter, r *http.Request) {
		w.Write(binaryData)
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	host := server.URL[len("http://"):]

	oldScheme := registryScheme
	registryScheme = "http"
	defer func() { registryScheme = oldScheme }()

	pi := ProviderSpecificInstance{
		Provider: Provider{Hostname: host, Owner: "hashicorp", Name: "testprov"},
		Version:  "1.0.0",
		OS:       "linux",
		Arch:     "amd64",
	}

	mock := mockProviderStorer{
		writeFunc: func(data []byte, pi ProviderSpecificInstance) (*ProviderSpecificInstanceBinary, error) {
			return &ProviderSpecificInstanceBinary{
				ProviderSpecificInstance: pi,
				H1Checksum:              "h1:test",
				FullPath:                "/tmp/test.zip",
			}, nil
		},
	}

	d := ProviderDownloader{Storage: mock}
	result, err := d.MirrorProviderInstanceToDest(pi)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.H1Checksum != "h1:test" {
		t.Errorf("expected h1:test, got %s", result.H1Checksum)
	}
}

func TestMirrorProviderInstanceToDest_ChecksumMismatch(t *testing.T) {
	oldSleep := retrySleepFunc
	retrySleepFunc = func(d time.Duration) {}
	defer func() { retrySleepFunc = oldSleep }()

	binaryData := []byte("fake zip file content for testing")

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/providers/hashicorp/testprov/1.0.0/download/linux/amd64", func(w http.ResponseWriter, r *http.Request) {
		resp := HCTFRegistryDownloadResponse{
			Shasum:      "wrong_checksum_value",
			DownloadURL: fmt.Sprintf("http://%s/download/testprov.zip", r.Host),
		}
		json.NewEncoder(w).Encode(resp)
	})
	mux.HandleFunc("/download/testprov.zip", func(w http.ResponseWriter, r *http.Request) {
		w.Write(binaryData)
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	host := server.URL[len("http://"):]

	oldScheme := registryScheme
	registryScheme = "http"
	defer func() { registryScheme = oldScheme }()

	pi := ProviderSpecificInstance{
		Provider: Provider{Hostname: host, Owner: "hashicorp", Name: "testprov"},
		Version:  "1.0.0",
		OS:       "linux",
		Arch:     "amd64",
	}

	mock := mockProviderStorer{
		writeFunc: func(data []byte, pi ProviderSpecificInstance) (*ProviderSpecificInstanceBinary, error) {
			t.Fatal("WriteProviderBinaryDataToStorage should not be called on checksum mismatch")
			return nil, nil
		},
	}

	d := ProviderDownloader{Storage: mock}
	result, err := d.MirrorProviderInstanceToDest(pi)
	if err == nil {
		t.Fatal("expected error for checksum mismatch, got nil")
	}
	if result != nil {
		t.Error("expected nil result on error")
	}
}

func TestMirrorProviderInstanceToDest_StorageWriteError(t *testing.T) {
	oldSleep := retrySleepFunc
	retrySleepFunc = func(d time.Duration) {}
	defer func() { retrySleepFunc = oldSleep }()

	binaryData := []byte("fake zip file content for testing")
	hasher := sha256.New()
	hasher.Write(binaryData)
	checksum := fmt.Sprintf("%x", hasher.Sum(nil))

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/providers/hashicorp/testprov/1.0.0/download/linux/amd64", func(w http.ResponseWriter, r *http.Request) {
		resp := HCTFRegistryDownloadResponse{
			Shasum:      checksum,
			DownloadURL: fmt.Sprintf("http://%s/download/testprov.zip", r.Host),
		}
		json.NewEncoder(w).Encode(resp)
	})
	mux.HandleFunc("/download/testprov.zip", func(w http.ResponseWriter, r *http.Request) {
		w.Write(binaryData)
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	host := server.URL[len("http://"):]

	oldScheme := registryScheme
	registryScheme = "http"
	defer func() { registryScheme = oldScheme }()

	pi := ProviderSpecificInstance{
		Provider: Provider{Hostname: host, Owner: "hashicorp", Name: "testprov"},
		Version:  "1.0.0",
		OS:       "linux",
		Arch:     "amd64",
	}

	mock := mockProviderStorer{
		writeFunc: func(data []byte, pi ProviderSpecificInstance) (*ProviderSpecificInstanceBinary, error) {
			return nil, fmt.Errorf("storage write failed")
		},
	}

	d := ProviderDownloader{Storage: mock}
	result, err := d.MirrorProviderInstanceToDest(pi)
	if err == nil {
		t.Fatal("expected error for storage write failure, got nil")
	}
	if result != nil {
		t.Error("expected nil result on error")
	}
}
