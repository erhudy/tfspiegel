package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"go.uber.org/zap"
)

var httpClient http.Client
var sugar *zap.SugaredLogger

func (pp HCTFProviderPlatform) String() string {
	return fmt.Sprintf("%s_%s", pp.OS, pp.Arch)
}

func (d *ProviderDownloader) MirrorProviderInstanceToDest(pi ProviderSpecificInstance) (psib *ProviderSpecificInstanceBinary, err error) {
	sugar.Infof("mirroring PVI %s", pi)

	downloadResponseUrl := fmt.Sprintf("https://%s/v1/providers/%s/%s/%s/download/%s/%s", pi.Hostname, pi.Owner, pi.Name, pi.Version, pi.OS, pi.Arch)

	retries := 0
	maxRetries := 5
	completed := false

	var lastErr error

	for {
		if completed {
			break
		}

		sugar.Debugf("starting download loop for %s", pi)
		if retries > 0 {
			sleepFor := retries * retries
			sugar.Warnf("sleeping %d seconds", sleepFor)
			time.Sleep(time.Duration(sleepFor))
		}
		if retries >= maxRetries {
			sugar.Errorf("hit max retries of %d for provider instance %s", retries, pi)
			break
		}

		req, err := http.NewRequest(http.MethodGet, downloadResponseUrl, nil)
		if err != nil {
			lastErr = err
			sugar.Errorf("error making HTTP request for provider instance %s: %w", pi, err)
			retries += 1
			break
		}
		resp, err := httpClient.Do(req)
		if err != nil {
			lastErr = err
			sugar.Errorf("error getting HTTP response for provider instance %s: %w", pi, err)
			retries += 1
			break
		}

		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			lastErr = err
			sugar.Errorf("error reading HTTP response body for provider instance %s: %w", pi, err)
			retries += 1
			break
		}

		var registryDownloadResponse HCTFRegistryDownloadResponse
		err = json.Unmarshal(respBody, &registryDownloadResponse)
		if err != nil {
			lastErr = err
			sugar.Errorf("error unmarshalling response body for provider instance %s: %w", pi, err)
			retries += 1
			break
		}

		downloadReq, err := http.NewRequest(http.MethodGet, registryDownloadResponse.DownloadURL, nil)
		if err != nil {
			lastErr = err
			sugar.Errorf("error making HTTP download request for provider instance %s: %w", pi, err)
			retries += 1
			break
		}
		downloadResp, err := httpClient.Do(downloadReq)
		if err != nil {
			lastErr = err
			sugar.Errorf("error downloading binary for provider instance %s: %w", pi, err)
			retries += 1
			break
		}

		hasher := sha256.New()
		providerBinary, err := io.ReadAll(downloadResp.Body)
		if err != nil {
			lastErr = err
			sugar.Errorf("error reading downloaded binary for provider instance %s: %w", pi, err)
			retries += 1
			break
		}

		_, err = hasher.Write(providerBinary)
		if err != nil {
			lastErr = err
			sugar.Errorf("error checksumming provider for provider instance %s: %w", pi, err)
			retries += 1
			break
		}

		checksum := fmt.Sprintf("%x", hasher.Sum(nil))
		if checksum != registryDownloadResponse.Shasum {
			lastErr = fmt.Errorf("got SHA %x, expected %s", checksum, registryDownloadResponse.Shasum)
			sugar.Errorf("error making HTTP request for provider instance %s: %w", pi, lastErr)
			retries += 1
			break
		}

		psib, err = d.Storage.WriteProviderBinaryDataToStorage(providerBinary, pi)
		if err != nil {
			lastErr = err
			sugar.Errorf("error writing binary data to storage for provider instance %s: %w", pi, lastErr)
			retries += 1
			break
		}

		completed = true
	}

	if lastErr != nil && retries >= maxRetries {
		return nil, lastErr
	}

	return psib, nil
}
