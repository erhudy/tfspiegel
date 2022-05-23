package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"

	semver "github.com/blang/semver/v4"
)

func (p Provider) String() string {
	return fmt.Sprintf("%s/%s/%s", p.Hostname, p.Owner, p.Name)
}

func (p Provider) GetDownloadBase() string {
	return filepath.Join(p.Hostname, p.Owner, p.Name)
}

func (pi ProviderSpecificInstance) String() string {
	return fmt.Sprintf("%s/%s/%s %s %s_%s", pi.Hostname, pi.Owner, pi.Name, pi.Version, pi.OS, pi.Arch)
}

func (pi ProviderSpecificInstance) GetDownloadedFileName() string {
	return fmt.Sprintf("terraform-provider-%s_%s_%s_%s.zip", pi.Name, pi.Version, pi.OS, pi.Arch)
}

func NewProviderFromConfigProvider(providerURL string) (Provider, error) {
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

// Fetches the JSON file from the registry for a given provider that lists the available versions and platforms.
func (p Provider) GetProviderMetadataFromRegistry() (RemoteProviderMetadata, error) {
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("https://%s/v1/providers/%s/%s/versions", p.Hostname, p.Owner, p.Name), nil)
	if err != nil {
		panic(err)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		panic(err)
	}
	remoteProviderMetadata := RemoteProviderMetadata{
		Provider: p,
	}

	responseJson, err := io.ReadAll(resp.Body)
	if err != nil {
		panic(err)
	}
	err = json.Unmarshal(responseJson, &remoteProviderMetadata)
	if err != nil {
		panic(err)
	}

	return remoteProviderMetadata, nil
}

// Filters the list of available version/platform combos in the remote registry for this provider down to the candidates to be mirrored.
func (p Provider) FilterToWantedPVIs(providerMetadata RemoteProviderMetadata, versionRange string, osArchs []HCTFProviderPlatform) ([]ProviderSpecificInstance, error) {
	var filteredProviders []ProviderSpecificInstance

	sugar.Infof("%s", providerMetadata)

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

		for _, requestedOSArch := range osArchs {
			foundOSArch := false
			for _, upstreamOSArch := range upstreamProvider.Platforms {
				if upstreamOSArch.Arch == requestedOSArch.Arch && upstreamOSArch.OS == requestedOSArch.OS {
					providerInstance := ProviderSpecificInstance{
						Provider: p,
						Version:  version.String(),
						OS:       upstreamOSArch.OS,
						Arch:     upstreamOSArch.Arch,
					}
					filteredProviders = append(filteredProviders, providerInstance)
					foundOSArch = true
				}
			}
			if !foundOSArch {
				sugar.Errorf("Requested OS/arch combination %s for %s %s not found", requestedOSArch.String(), providerMetadata.Provider, version)
			}
		}
	}

	return filteredProviders, nil
}
