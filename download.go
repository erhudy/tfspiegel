package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"path/filepath"
	"strings"

	"go.uber.org/zap"
)

const (
	DEFAULT_PROVIDER_HOSTNAME = "registry.terraform.io"
	DEFAULT_PROVIDER_OWNER    = "hashicorp"
)

var httpClient http.Client
var sugar *zap.SugaredLogger

func (p Provider) String() string {
	return fmt.Sprintf("%s/%s/%s", p.Hostname, p.Owner, p.Name)
}

func (pi ProviderInstance) String() string {
	return fmt.Sprintf("%s/%s/%s %s", pi.Hostname, pi.Owner, pi.Name, pi.Version)
}

func (pi ProviderInstance) GetOsArchs() string {
	var osArchs []string

	for _, osArch := range pi.Platforms {
		osArchs = append(osArchs, fmt.Sprintf("%s_%s", osArch.OS, osArch.Arch))
	}

	return strings.Join(osArchs, ",")
}

func (pi ProviderInstance) GetDownloadPath() string {
	return filepath.Join(pi.Hostname, pi.Owner, pi.Name)
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

func (d *ProviderDownloader) mirrorProviderInstanceToDest(pi ProviderInstance, destination DownloadDestination) error {
	sugar.Infof("Working on provider %s (%s)", pi, pi.GetOsArchs())

	var mirrorProvider MirrorProvider
	mirrorProvider.Archives = make(map[string]MirrorProviderPlatformArch)

	for _, osArch := range pi.Platforms {
		if !d.storage.NeedToDownloadProviderInstance(pi, osArch) {
			continue
		}
		sugar.Infof("Downloading provider %s (%s)", pi.String(), osArch.String())

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

		mppa, err := d.storage.WriteProvider(pi, registryDownloadResponse, providerBinary)
		if err != nil {
			return err
		}

		mirrorProvider.Archives[osArch.String()] = mppa
	}

	err := d.storage.WriteProviderMetadataFile(pi, mirrorProvider)
	if err != nil {
		return err
	}

	return nil
}
