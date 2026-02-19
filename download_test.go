package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHCTFProviderPlatformString(t *testing.T) {
	pp := HCTFProviderPlatform{OS: "linux", Arch: "amd64"}
	expected := "linux_amd64"
	got := pp.String()
	if got != expected {
		t.Errorf("String() = %q, want %q", got, expected)
	}
}

func TestFilterVersionsWithFailedPSIBs(t *testing.T) {
	awsProvider := Provider{Hostname: "registry.terraform.io", Owner: "hashicorp", Name: "aws"}
	gcpProvider := Provider{Hostname: "registry.terraform.io", Owner: "hashicorp", Name: "google"}

	makePSIB := func(p Provider, version, os, arch string) ProviderSpecificInstanceBinary {
		return ProviderSpecificInstanceBinary{
			ProviderSpecificInstance: ProviderSpecificInstance{
				Provider: p, Version: version, OS: os, Arch: arch,
			},
		}
	}
	makePSI := func(p Provider, version, os, arch string) ProviderSpecificInstance {
		return ProviderSpecificInstance{Provider: p, Version: version, OS: os, Arch: arch}
	}

	tests := []struct {
		name      string
		psibs     []ProviderSpecificInstanceBinary
		failed    []ProviderSpecificInstance
		wantCount int
	}{
		{
			"no failures returns all",
			[]ProviderSpecificInstanceBinary{
				makePSIB(awsProvider, "5.0.0", "linux", "amd64"),
				makePSIB(awsProvider, "5.0.0", "darwin", "arm64"),
			},
			nil,
			2,
		},
		{
			"one version fails removes all psibs for that version",
			[]ProviderSpecificInstanceBinary{
				makePSIB(awsProvider, "5.0.0", "linux", "amd64"),
				makePSIB(awsProvider, "5.0.0", "darwin", "arm64"),
				makePSIB(awsProvider, "5.1.0", "linux", "amd64"),
			},
			[]ProviderSpecificInstance{
				makePSI(awsProvider, "5.0.0", "linux", "arm64"),
			},
			1,
		},
		{
			"all fail returns empty",
			[]ProviderSpecificInstanceBinary{
				makePSIB(awsProvider, "5.0.0", "linux", "amd64"),
			},
			[]ProviderSpecificInstance{
				makePSI(awsProvider, "5.0.0", "darwin", "arm64"),
			},
			0,
		},
		{
			"different provider same version only matching provider filtered",
			[]ProviderSpecificInstanceBinary{
				makePSIB(awsProvider, "5.0.0", "linux", "amd64"),
				makePSIB(gcpProvider, "5.0.0", "linux", "amd64"),
			},
			[]ProviderSpecificInstance{
				makePSI(awsProvider, "5.0.0", "linux", "arm64"),
			},
			1,
		},
		{
			"empty inputs",
			nil,
			nil,
			0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FilterVersionsWithFailedPSIBs(tt.psibs, tt.failed)
			if len(got) != tt.wantCount {
				t.Errorf("got %d psibs, want %d", len(got), tt.wantCount)
			}
		})
	}
}

func TestMirrorProviderInstanceToDest(t *testing.T) {
	origSleep := retrySleep
	retrySleep = func(int) {}
	defer func() { retrySleep = origSleep }()

	origClient := httpClient
	defer func() { httpClient = origClient }()

	testBinary := []byte("fake provider binary data for testing")
	hasher := sha256.New()
	hasher.Write(testBinary)
	testSHA := fmt.Sprintf("%x", hasher.Sum(nil))

	pi := ProviderSpecificInstance{
		Provider: Provider{Hostname: "placeholder", Owner: "hashicorp", Name: "aws"},
		Version:  "5.0.0",
		OS:       "linux",
		Arch:     "amd64",
	}

	t.Run("successful download on first attempt", func(t *testing.T) {
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.URL.Path == "/v1/providers/hashicorp/aws/5.0.0/download/linux/amd64":
				resp := HCTFRegistryDownloadResponse{
					DownloadURL: fmt.Sprintf("https://%s/download/aws.zip", r.Host),
					Shasum:      testSHA,
				}
				json.NewEncoder(w).Encode(resp)
			case r.URL.Path == "/download/aws.zip":
				w.Write(testBinary)
			default:
				w.WriteHeader(404)
			}
		}))
		defer server.Close()
		httpClient = server.Client()

		serverHost := server.URL[len("https://"):]
		localPI := pi
		localPI.Hostname = serverHost

		writeCalled := false
		mock := mockProviderStorer{
			writeProviderBinaryDataToStorageFunc: func(data []byte, p ProviderSpecificInstance) (*ProviderSpecificInstanceBinary, error) {
				writeCalled = true
				return &ProviderSpecificInstanceBinary{
					ProviderSpecificInstance: p,
					H1Checksum:              "h1:test",
					FullPath:                "/tmp/test.zip",
				}, nil
			},
		}

		d := ProviderDownloader{Storage: mock}
		psib, err := d.MirrorProviderInstanceToDest(localPI)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if psib == nil {
			t.Fatal("expected non-nil psib")
		}
		if !writeCalled {
			t.Error("expected WriteProviderBinaryDataToStorage to be called")
		}
	})

	t.Run("registry returns 404 errors after max retries", func(t *testing.T) {
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(404)
		}))
		defer server.Close()
		httpClient = server.Client()

		serverHost := server.URL[len("https://"):]
		localPI := pi
		localPI.Hostname = serverHost

		mock := mockProviderStorer{
			writeProviderBinaryDataToStorageFunc: func(data []byte, p ProviderSpecificInstance) (*ProviderSpecificInstanceBinary, error) {
				t.Fatal("should not be called")
				return nil, nil
			},
		}

		d := ProviderDownloader{Storage: mock}
		_, err := d.MirrorProviderInstanceToDest(localPI)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("binary download returns 404", func(t *testing.T) {
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/v1/providers/hashicorp/aws/5.0.0/download/linux/amd64" {
				resp := HCTFRegistryDownloadResponse{
					DownloadURL: fmt.Sprintf("https://%s/download/aws.zip", r.Host),
					Shasum:      testSHA,
				}
				json.NewEncoder(w).Encode(resp)
			} else {
				w.WriteHeader(404)
			}
		}))
		defer server.Close()
		httpClient = server.Client()

		serverHost := server.URL[len("https://"):]
		localPI := pi
		localPI.Hostname = serverHost

		mock := mockProviderStorer{
			writeProviderBinaryDataToStorageFunc: func(data []byte, p ProviderSpecificInstance) (*ProviderSpecificInstanceBinary, error) {
				t.Fatal("should not be called")
				return nil, nil
			},
		}

		d := ProviderDownloader{Storage: mock}
		_, err := d.MirrorProviderInstanceToDest(localPI)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("SHA256 checksum mismatch", func(t *testing.T) {
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.URL.Path == "/v1/providers/hashicorp/aws/5.0.0/download/linux/amd64":
				resp := HCTFRegistryDownloadResponse{
					DownloadURL: fmt.Sprintf("https://%s/download/aws.zip", r.Host),
					Shasum:      "0000000000000000000000000000000000000000000000000000000000000000",
				}
				json.NewEncoder(w).Encode(resp)
			case r.URL.Path == "/download/aws.zip":
				w.Write(testBinary)
			default:
				w.WriteHeader(404)
			}
		}))
		defer server.Close()
		httpClient = server.Client()

		serverHost := server.URL[len("https://"):]
		localPI := pi
		localPI.Hostname = serverHost

		mock := mockProviderStorer{
			writeProviderBinaryDataToStorageFunc: func(data []byte, p ProviderSpecificInstance) (*ProviderSpecificInstanceBinary, error) {
				t.Fatal("should not be called")
				return nil, nil
			},
		}

		d := ProviderDownloader{Storage: mock}
		_, err := d.MirrorProviderInstanceToDest(localPI)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("storage write failure", func(t *testing.T) {
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.URL.Path == "/v1/providers/hashicorp/aws/5.0.0/download/linux/amd64":
				resp := HCTFRegistryDownloadResponse{
					DownloadURL: fmt.Sprintf("https://%s/download/aws.zip", r.Host),
					Shasum:      testSHA,
				}
				json.NewEncoder(w).Encode(resp)
			case r.URL.Path == "/download/aws.zip":
				w.Write(testBinary)
			default:
				w.WriteHeader(404)
			}
		}))
		defer server.Close()
		httpClient = server.Client()

		serverHost := server.URL[len("https://"):]
		localPI := pi
		localPI.Hostname = serverHost

		mock := mockProviderStorer{
			writeProviderBinaryDataToStorageFunc: func(data []byte, p ProviderSpecificInstance) (*ProviderSpecificInstanceBinary, error) {
				return nil, fmt.Errorf("disk full")
			},
		}

		d := ProviderDownloader{Storage: mock}
		_, err := d.MirrorProviderInstanceToDest(localPI)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("retry succeeds on second attempt", func(t *testing.T) {
		callCount := 0
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/v1/providers/hashicorp/aws/5.0.0/download/linux/amd64" {
				callCount++
				if callCount == 1 {
					w.WriteHeader(500)
					return
				}
				resp := HCTFRegistryDownloadResponse{
					DownloadURL: fmt.Sprintf("https://%s/download/aws.zip", r.Host),
					Shasum:      testSHA,
				}
				json.NewEncoder(w).Encode(resp)
			} else if r.URL.Path == "/download/aws.zip" {
				w.Write(testBinary)
			} else {
				w.WriteHeader(404)
			}
		}))
		defer server.Close()
		httpClient = server.Client()

		serverHost := server.URL[len("https://"):]
		localPI := pi
		localPI.Hostname = serverHost

		mock := mockProviderStorer{
			writeProviderBinaryDataToStorageFunc: func(data []byte, p ProviderSpecificInstance) (*ProviderSpecificInstanceBinary, error) {
				return &ProviderSpecificInstanceBinary{
					ProviderSpecificInstance: p,
					H1Checksum:              "h1:test",
					FullPath:                "/tmp/test.zip",
				}, nil
			},
		}

		d := ProviderDownloader{Storage: mock}
		psib, err := d.MirrorProviderInstanceToDest(localPI)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if psib == nil {
			t.Fatal("expected non-nil psib")
		}
		if callCount < 2 {
			t.Errorf("expected at least 2 calls to registry, got %d", callCount)
		}
	})
}
