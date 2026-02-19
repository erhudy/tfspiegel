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

func (p Provider) GetDownloadBase() string {
	return filepath.Join(p.Hostname, p.Owner, p.Name)
}

func (p Provider) String() string {
	return p.GetDownloadBase()
}

func (pi ProviderSpecificInstance) String() string {
	return fmt.Sprintf("%s/%s/%s %s %s_%s", pi.Hostname, pi.Owner, pi.Name, pi.Version, pi.OS, pi.Arch)
}

func (pi ProviderSpecificInstance) GetDownloadedFileName() string {
	return fmt.Sprintf("terraform-provider-%s_%s_%s_%s.zip", pi.Name, pi.Version, pi.OS, pi.Arch)
}

func NewProviderFromConfigProvider(providerURL string) (Provider, error) {
	provider := Provider{
		Hostname: defaultProviderHostname,
		Owner:    defaultProviderOwner,
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
		provider.Name = strings.Join(splitURL[2:], "/")
	}

	return provider, nil
}

// Fetches the JSON file from the registry for a given provider that lists the available versions and platforms.
func (p Provider) GetProviderMetadataFromRegistry() (RemoteProviderMetadata, error) {
	remoteProviderMetadata := RemoteProviderMetadata{
		Provider: p,
	}

	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("https://%s/v1/providers/%s/%s/versions", p.Hostname, p.Owner, p.Name), nil)
	if err != nil {
		return remoteProviderMetadata, fmt.Errorf("error creating HTTP request for provider metadata: %w", err)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return remoteProviderMetadata, fmt.Errorf("error fetching provider metadata from registry: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 400 {
		return remoteProviderMetadata, fmt.Errorf("HTTP %d fetching provider metadata for %s", resp.StatusCode, p)
	}

	responseJson, err := io.ReadAll(resp.Body)
	if err != nil {
		return remoteProviderMetadata, fmt.Errorf("error reading provider metadata response body: %w", err)
	}
	err = json.Unmarshal(responseJson, &remoteProviderMetadata)
	if err != nil {
		return remoteProviderMetadata, fmt.Errorf("error unmarshalling provider metadata: %w", err)
	}

	return remoteProviderMetadata, nil
}

// Filters the list of available version/platform combos in the remote registry for this provider down to the candidates to be mirrored.
func (p Provider) FilterToWantedPVIs(providerMetadata RemoteProviderMetadata, providerConfig ProviderMirrorConfiguration, osArchs []HCTFProviderPlatform) ([]ProviderSpecificInstance, error) {
	var filteredProviders []ProviderSpecificInstance

	sugar.Infof("%s", providerMetadata)

	parsedRange, err := semver.ParseRange(providerConfig.VersionRange)
	if err != nil {
		return filteredProviders, err
	}

	var versionsToSkip []semver.Version

	for _, upvts := range providerConfig.SkipVersions {
		parsed, err := semver.Parse(upvts)
		if err != nil {
			sugar.Errorf("error parsing version to skip '%s' as semver", upvts)
			continue
		}
		versionsToSkip = append(versionsToSkip, parsed)
	}

	for _, upstreamProvider := range providerMetadata.Versions {
		upstreamVersion, err := semver.Parse(upstreamProvider.Version)
		if err != nil {
			sugar.Errorf("error parsing upstream version '%s' as semver", upstreamProvider.Version)
			continue
		}
		if !parsedRange(upstreamVersion) {
			continue
		}

		skipThisVersion := false
		for _, versionToSkip := range versionsToSkip {
			if upstreamVersion.Equals(versionToSkip) {
				skipThisVersion = true
			}
			break
		}
		if skipThisVersion {
			continue
		}

		for _, requestedOSArch := range osArchs {
			foundOSArch := false
			for _, upstreamOSArch := range upstreamProvider.Platforms {
				if upstreamOSArch.Arch == requestedOSArch.Arch && upstreamOSArch.OS == requestedOSArch.OS {
					providerInstance := ProviderSpecificInstance{
						Provider: p,
						Version:  upstreamVersion.String(),
						OS:       upstreamOSArch.OS,
						Arch:     upstreamOSArch.Arch,
					}
					filteredProviders = append(filteredProviders, providerInstance)
					foundOSArch = true
				}
			}
			if !foundOSArch {
				sugar.Errorf("Requested OS/arch combination %s for %s %s not found", requestedOSArch.String(), providerMetadata.Provider, upstreamVersion)
			}
		}
	}

	return filteredProviders, nil
}
