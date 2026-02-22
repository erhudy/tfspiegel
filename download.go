package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

var httpClient = &http.Client{Timeout: 60 * time.Second}

var retrySleep = func(retries int) {
	sleepFor := retries * retries
	sugar.Warnf("sleeping %d seconds", sleepFor)
	time.Sleep(time.Duration(sleepFor) * time.Second)
}

func (pp HCTFProviderPlatform) String() string {
	return fmt.Sprintf("%s_%s", pp.OS, pp.Arch)
}

func (d *ProviderDownloader) MirrorProviderInstanceToDest(pi ProviderSpecificInstance) (psib *ProviderSpecificInstanceBinary, err error) {
	sugar.Infof("mirroring PVI %s", pi)

	downloadResponseUrl := fmt.Sprintf("https://%s/v1/providers/%s/%s/%s/download/%s/%s", pi.Hostname, pi.Owner, pi.Name, pi.Version, pi.OS, pi.Arch)

	retries := 0
	maxRetries := 5

	var lastErr error

	for retries < maxRetries {
		sugar.Debugf("starting download for PVI %s", pi)
		if retries > 0 {
			retrySleep(retries)
		}

		req, err := http.NewRequest(http.MethodGet, downloadResponseUrl, nil)
		if err != nil {
			lastErr = err
			sugar.Errorf("error making HTTP request for PVI %s: %v", pi, err)
			retries += 1
			continue
		}
		resp, err := httpClient.Do(req)
		if err != nil {
			lastErr = err
			sugar.Errorf("error getting HTTP response for PVI %s: %v", pi, err)
			retries += 1
			continue
		}
		if resp.StatusCode >= 400 {
			_ = resp.Body.Close()
			lastErr = fmt.Errorf("HTTP %d from registry for PVI %s", resp.StatusCode, pi)
			sugar.Errorf("error getting HTTP response for PVI %s: %v", pi, lastErr)
			retries += 1
			continue
		}

		respBody, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil {
			lastErr = err
			sugar.Errorf("error reading HTTP response body for PVI %s: %v", pi, err)
			retries += 1
			continue
		}

		var registryDownloadResponse HCTFRegistryDownloadResponse
		err = json.Unmarshal(respBody, &registryDownloadResponse)
		if err != nil {
			lastErr = err
			sugar.Errorf("error unmarshalling response body for PVI %s: %v", pi, err)
			retries += 1
			continue
		}

		downloadReq, err := http.NewRequest(http.MethodGet, registryDownloadResponse.DownloadURL, nil)
		if err != nil {
			lastErr = err
			sugar.Errorf("error making HTTP download request for PVI %s: %v", pi, err)
			retries += 1
			continue
		}
		downloadResp, err := httpClient.Do(downloadReq)
		if err != nil {
			lastErr = err
			sugar.Errorf("error downloading binary for PVI %s: %v", pi, err)
			retries += 1
			continue
		}
		if downloadResp.StatusCode >= 400 {
			_ = downloadResp.Body.Close()
			lastErr = fmt.Errorf("HTTP %d downloading binary for PVI %s", downloadResp.StatusCode, pi)
			sugar.Errorf("error downloading binary for PVI %s: %v", pi, lastErr)
			retries += 1
			continue
		}

		hasher := sha256.New()
		providerBinary, err := io.ReadAll(downloadResp.Body)
		_ = downloadResp.Body.Close()
		if err != nil {
			lastErr = err
			sugar.Errorf("error reading downloaded binary for PVI %s: %v", pi, err)
			retries += 1
			continue
		}

		_, err = hasher.Write(providerBinary)
		if err != nil {
			lastErr = err
			sugar.Errorf("error checksumming provider for PVI %s: %v", pi, err)
			retries += 1
			continue
		}

		checksum := fmt.Sprintf("%x", hasher.Sum(nil))
		if checksum != registryDownloadResponse.Shasum {
			lastErr = fmt.Errorf("got SHA %s, expected %s", checksum, registryDownloadResponse.Shasum)
			sugar.Errorf("checksum mismatch for PVI %s: %v", pi, lastErr)
			retries += 1
			continue
		}

		psib, err = d.Storage.WriteProviderBinaryDataToStorage(providerBinary, pi)
		if err != nil {
			lastErr = err
			sugar.Errorf("error writing binary data to storage for PVI %s: %v", pi, lastErr)
			retries += 1
			continue
		}

		return psib, nil
	}

	sugar.Errorf("hit max retries of %d for PVI %s", maxRetries, pi)
	return nil, lastErr
}

// removes versions from psibs where we don't have every OS+arch combination downloaded
// this is only to remove the version from index.json, we still keep the existing providers around so that we don't need to redownload everything later
func FilterVersionsWithFailedPSIBs(psibs []ProviderSpecificInstanceBinary, failedPvis []ProviderSpecificInstance) []ProviderSpecificInstanceBinary {
	filteredPsibs := []ProviderSpecificInstanceBinary{}

	for _, p := range psibs {
		foundInFailed := false
		for _, fp := range failedPvis {
			if p.Name == fp.Name && p.Owner == fp.Owner && p.Version == fp.Version {
				foundInFailed = true
				break
			}
		}
		if foundInFailed {
			sugar.Warnf("filtering %s out due to only having some of the providers for version %s", p.Provider.String(), p.Version)
		} else {
			filteredPsibs = append(filteredPsibs, p)
		}
	}

	return filteredPsibs
}
