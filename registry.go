package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	semver "github.com/blang/semver/v4"
)

// Fetches the JSON file from the registry for a given provider that lists the available versions and platforms.
func GetProviderMetadataFromRegistry(provider Provider) (ProviderMetadata, error) {
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

// Filters the list of available version/platform combos in the remote registry down to the candidates to be mirrored.
func FilterToWantedProviderInstances(providerMetadata ProviderMetadata, versionRange string, osArchs []ProviderPlatform) ([]ProviderInstance, error) {
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
				sugar.Errorf("Requested OS/arch combination for %s %s not found: %s", providerMetadata.Provider, version, requestedOSArch.String())
			}
		}

		providerInstance := ProviderInstance{
			Provider: Provider{
				Hostname: providerMetadata.Hostname,
				Owner:    providerMetadata.Owner,
				Name:     providerMetadata.Name,
			},
			Version:   version.String(),
			Platforms: validArchs,
		}
		filteredProviders = append(filteredProviders, providerInstance)
	}

	return filteredProviders, nil
}
