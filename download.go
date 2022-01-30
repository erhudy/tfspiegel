package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	semver "github.com/blang/semver/v4"
	"go.uber.org/zap"
	"golang.org/x/mod/sumdb/dirhash"
)

const (
	DEFAULT_PROVIDER_HOSTNAME = "registry.terraform.io"
	DEFAULT_PROVIDER_OWNER    = "hashicorp"
)

var httpClient http.Client
var sugar *zap.SugaredLogger

func (pi ProviderInstance) String() string {
	var osArchs []string

	for _, osArch := range pi.Platforms {
		osArchs = append(osArchs, fmt.Sprintf("%s_%s", osArch.OS, osArch.Arch))
	}

	return fmt.Sprintf("%s/%s/%s: %s (%s)", pi.Hostname, pi.Owner, pi.Name, pi.Version, strings.Join(osArchs, ","))
}

func (pi ProviderInstance) DownloadDir(destination DownloadDestination) string {
	return filepath.Join(string(destination), pi.Hostname, pi.Owner, pi.Name)
}

func (pp ProviderPlatform) String() string {
	return fmt.Sprintf("%s_%s", pp.OS, pp.Arch)
}

func NewProvider(providerURL string) (Provider, error) {
	provider := Provider{
		Hostname: DEFAULT_PROVIDER_HOSTNAME,
		Owner:    DEFAULT_PROVIDER_OWNER,
	}

	splitURL := strings.Split(providerURL, "/")
	switch len(splitURL) {
	case 0:
		return provider, fmt.Errorf("split resulted in 0 len slice somehow")
	case 1:
		provider.Name = splitURL[0]
	case 2:
		provider.Owner = splitURL[0]
		provider.Name = splitURL[1]
	default:
		provider.Hostname = splitURL[0]
		provider.Owner = splitURL[1]
		provider.Name = strings.Join(splitURL[2:(len(splitURL)-1)], "/")
	}

	return provider, nil
}

func GetProviderMetadata(provider Provider) (ProviderMetadata, error) {
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("https://%s/v1/providers/%s/%s/versions", provider.Hostname, provider.Owner, provider.Name), nil)
	if err != nil {
		panic(err)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		panic(err)
	}
	var providerMetadata ProviderMetadata

	responseJson, err := io.ReadAll(resp.Body)
	if err != nil {
		panic(err)
	}
	err = json.Unmarshal(responseJson, &providerMetadata)
	if err != nil {
		panic(err)
	}

	return providerMetadata, nil
}

func FilterProvidersByConstraints(provider Provider, providerMetadata ProviderMetadata, versionRange string, osArchs []ProviderPlatform) ([]ProviderInstance, error) {
	var filteredProviders []ProviderInstance

	parsedRange, err := semver.ParseRange(versionRange)
	if err != nil {
		return filteredProviders, err
	}

	for _, upstreamProvider := range providerMetadata.Versions {
		version, err := semver.Parse(upstreamProvider.Version)
		if err != nil {
			sugar.Errorf("error parsing %s as semver", version)
			continue
		}
		if !parsedRange(version) {
			continue
		}
		var validArchs []ProviderPlatform

		for _, requestedOSArch := range osArchs {
			foundOSArch := false
			for _, upstreamOSArch := range upstreamProvider.Platforms {
				if upstreamOSArch.Arch == requestedOSArch.Arch && upstreamOSArch.OS == requestedOSArch.OS {
					validArchs = append(validArchs, requestedOSArch)
					foundOSArch = true
				}
			}
			if !foundOSArch {
				sugar.Errorf("Requested OS/arch combination not found: %s", requestedOSArch.String())
			}
		}

		providerInstance := ProviderInstance{
			Provider: Provider{
				Hostname: provider.Hostname,
				Owner:    provider.Owner,
				Name:     provider.Name,
			},
			Version:   version.String(),
			Platforms: validArchs,
		}
		filteredProviders = append(filteredProviders, providerInstance)
	}

	return filteredProviders, nil
}

func MirrorProviderInstanceToDest(pi ProviderInstance, destination DownloadDestination) error {
	sugar.Infof("Downloading provider %s", pi)

	var mirrorProvider MirrorProvider
	mirrorProvider.Archives = make(map[string]MirrorProviderPlatformArch)
	workingDir := pi.DownloadDir(destination)

	for _, osArch := range pi.Platforms {
		downloadResponseUrl := fmt.Sprintf("https://%s/v1/providers/%s/%s/%s/download/%s/%s", pi.Hostname, pi.Owner, pi.Name, pi.Version, osArch.OS, osArch.Arch)

		req, err := http.NewRequest(http.MethodGet, downloadResponseUrl, nil)
		if err != nil {
			return err
		}
		resp, err := httpClient.Do(req)
		if err != nil {
			return err
		}

		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			return err
		}

		var registryDownloadResponse RegistryDownloadResponse
		err = json.Unmarshal(respBody, &registryDownloadResponse)
		if err != nil {
			return err
		}

		downloadReq, err := http.NewRequest(http.MethodGet, registryDownloadResponse.DownloadURL, nil)
		if err != nil {
			return err
		}
		downloadResp, err := httpClient.Do(downloadReq)
		if err != nil {
			return err
		}

		hasher := sha256.New()
		providerBinary, err := ioutil.ReadAll(downloadResp.Body)
		if err != nil {
			return err
		}

		_, err = hasher.Write(providerBinary)
		if err != nil {
			return err
		}

		checksum := fmt.Sprintf("%x", hasher.Sum(nil))
		if checksum != registryDownloadResponse.Shasum {
			return fmt.Errorf("got SHA %x, expected %s", checksum, registryDownloadResponse.Shasum)
		}

		destPath := filepath.Join(workingDir, registryDownloadResponse.Filename)
		err = os.MkdirAll(workingDir, os.FileMode(0755))
		if err != nil {
			return err
		}

		err = ioutil.WriteFile(destPath, providerBinary, os.FileMode(0644))
		if err != nil {
			return err
		}

		hash, err := dirhash.HashZip(destPath, dirhash.DefaultHash)
		if err != nil {
			return err
		}
		mppa := MirrorProviderPlatformArch{
			Hashes: []string{hash},
			URL:    registryDownloadResponse.Filename,
		}
		mirrorProvider.Archives[osArch.String()] = mppa
	}

	mirrorProviderJSONPath := filepath.Join(workingDir, fmt.Sprintf("%s.json", pi.Version))
	mirrorProviderMarshalled, err := json.Marshal(&mirrorProvider)
	if err != nil {
		return err
	}
	err = ioutil.WriteFile(mirrorProviderJSONPath, mirrorProviderMarshalled, os.FileMode(0644))
	if err != nil {
		return err
	}

	return nil
}

func MirrorProvidersWithConfig(config Config) error {
	logger, _ := zap.NewProduction()
	defer logger.Sync()
	sugar = logger.Sugar()

	var mirrorIndex MirrorIndex
	mirrorIndex.Versions = make(map[string]map[string]interface{})

	for _, configProvider := range config.Providers {
		provider, err := NewProvider(configProvider.Reference)
		if err != nil {
			return err
		}
		providerMetadata, err := GetProviderMetadata(provider)
		if err != nil {
			return err
		}

		filteredProviders, err := FilterProvidersByConstraints(provider, providerMetadata, configProvider.VersionRange, configProvider.OSArchs)
		if err != nil {
			return err
		}

		for _, filteredProvider := range filteredProviders {
			err = MirrorProviderInstanceToDest(filteredProvider, config.DownloadTo)
			if err != nil {
				sugar.Errorf("error mirroring provider: %s", err)
			}
			mirrorIndex.Versions[filteredProvider.Version] = make(map[string]interface{})
		}

		if len(filteredProviders) > 0 {
			indexJSONPath := filepath.Join(filteredProviders[0].DownloadDir(config.DownloadTo), "index.json")
			indexMarshalled, err := json.Marshal(&mirrorIndex)
			if err != nil {
				return err
			}
			err = ioutil.WriteFile(indexJSONPath, indexMarshalled, os.FileMode(0644))
			if err != nil {
				return err
			}
		}
	}

	return nil
}
