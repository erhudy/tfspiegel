package main

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/mod/sumdb/dirhash"
)

type localProviderStorage struct {
	downloadTo string
}

func NewLocalProviderStorage(downloadTo string) localProviderStorage {
	return localProviderStorage{
		downloadTo: downloadTo,
	}
}

// Examines local storage and determines what providers need to be downloaded/redownloaded (e.g. they are missing,
// checksum is not correct...)
func (this localProviderStorage) FilterAvailableProviderInstancesToDownload(pis []ProviderInstance) (filteredInstances []ProviderInstance, err error) {
	// first fetch

	return filteredInstances, nil
}

/* Examines all the ProviderInstances to ensure the download paths
 * are consistent, then
 */
func (this localProviderStorage) getLocallyStoredProviders(pis []ProviderInstance) (localBinaries []ProviderBinary, err error) {
	var downloadPaths map[string]string
	for _, pi := range pis {
		downloadPaths[pi.GetDownloadPath()] = ""
	}
	if len(downloadPaths) != 1 {
		var pathList []string
		for k, _ := range downloadPaths {
			pathList = append(pathList, k)
		}
		return nil, fmt.Errorf("got %d download paths instead of expected 1: %s", len(downloadPaths), strings.Join(pathList, ","))
	}
	var downloadPath string
	for k, _ := range downloadPaths {
		downloadPath = k
		break
	}

	err = filepath.WalkDir(downloadPath, func(path string, d fs.DirEntry, err error) error {
		if path == downloadPath {
			return nil
		}
		hash, err := dirhash.HashZip(path, dirhash.Hash1)
		if err != nil {
			return err
		}
		pb := 
		return nil
	})
	if err != nil {
		return nil, err
	}

	return localBinaries, nil
}

func (this localProviderStorage) NeedToDownloadProviderInstance(pi ProviderInstance, platform ProviderPlatform) bool {
	workingDir := filepath.Join(this.downloadTo, pi.GetDownloadPath())
	mirrorProviderJSONPath := filepath.Join(workingDir, fmt.Sprintf("%s.json", pi.Version))
	mirrorProviderJSONRaw, err := os.ReadFile(mirrorProviderJSONPath)
	if err != nil {
		return true
	}

	var mirrorProviderJSON MirrorProvider

	err = json.Unmarshal(mirrorProviderJSONRaw, &mirrorProviderJSON)
	if err != nil {
		return true
	}

	mirrorProviderPlatformArch, ok := mirrorProviderJSON.Archives[platform.String()]
	if !ok {
		return true
	}

	binaryPath := filepath.Join(workingDir, mirrorProviderPlatformArch.URL)
	hash, err := dirhash.HashZip(binaryPath, dirhash.DefaultHash)
	if err != nil {
		return true
	}

	var foundHash bool
	for _, h := range mirrorProviderPlatformArch.Hashes {
		if h == hash {
			foundHash = true
			break
		}
	}

	sugar.Infof("%s (%s): %s", pi.String(), pi.GetOsArchs(), foundHash)
	return !foundHash
}

func (this localProviderStorage) WriteProvider(pi ProviderInstance, rdr RegistryDownloadResponse, providerData []byte) (MirrorProviderPlatformArch, error) {
	var mppa MirrorProviderPlatformArch
	workingDir := filepath.Join(this.downloadTo, pi.GetDownloadPath())
	destPath := filepath.Join(workingDir, rdr.Filename)
	err := os.MkdirAll(workingDir, os.FileMode(0755))
	if err != nil {
		return mppa, err
	}

	err = ioutil.WriteFile(destPath, providerData, os.FileMode(0644))
	if err != nil {
		return mppa, err
	}

	hash, err := dirhash.HashZip(destPath, dirhash.DefaultHash)
	if err != nil {
		return mppa, err
	}
	mppa = MirrorProviderPlatformArch{
		Hashes: []string{hash},
		URL:    rdr.Filename,
	}
	return mppa, nil
}

func (l localProviderStorage) WriteProviderMetadataFile(pi ProviderInstance, mp MirrorProvider) error {
	mirrorProviderMarshalled, err := json.Marshal(&mp)
	if err != nil {
		return err
	}
	mirrorProviderJSONPath := filepath.Join(l.downloadTo, pi.GetDownloadPath(), fmt.Sprintf("%s.json", pi.Version))
	err = ioutil.WriteFile(mirrorProviderJSONPath, mirrorProviderMarshalled, os.FileMode(0644))
	if err != nil {
		return err
	}
	return nil
}
