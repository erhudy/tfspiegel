package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/mod/sumdb/dirhash"
)

type fsProviderStorage ProviderStorageConfiguration

func NewFSProviderStorer(psc ProviderStorageConfiguration) ProviderStorer {
	return fsProviderStorage(psc)
}

// for filesystem mirroring we use the Terraform mirror index and the individual JSON files as the catalog
func (s fsProviderStorage) LoadCatalog() ([]ProviderSpecificInstanceBinary, error) {
	indexFullPath := filepath.Join(s.downloadRoot, s.provider.String(), MIRROR_INDEX_FILE)
	indexContents, err := os.ReadFile(indexFullPath)
	if err != nil {
		s.sugar.Errorf("unable to read index file %s: %w", indexFullPath, err)
		return nil, fmt.Errorf("error loading catalog: %w", err)
	}
	var index MirrorIndex
	err = json.Unmarshal(indexContents, &index)
	if err != nil {
		return nil, err
	}
	sugar.Debugf("unmarshalled index: %v", index)

	var psibs []ProviderSpecificInstanceBinary
	// per Hashicorp docs at https://www.terraform.io/internals/provider-network-mirror-protocol#sample-response
	// the value for each key is currently an empty object
	for versionNumber := range index.Versions {
		sugar.Debugf("examining version %s", versionNumber)
		versionJsonFullPath := filepath.Join(s.downloadRoot, s.provider.String(), fmt.Sprintf("%s.json", versionNumber))
		versionJsonContents, err := os.ReadFile(versionJsonFullPath)
		if err != nil {
			s.sugar.Errorf("unable to read version JSON file %s: %w", versionJsonFullPath, err)
			continue
		}
		var archives MirrorArchives
		err = json.Unmarshal(versionJsonContents, &archives)
		if err != nil {
			s.sugar.Errorf("unable to unmarshal version JSON file %s: %w")
		}

		for osAndArch, hashesAndUrl := range archives.Archives {
			if len(hashesAndUrl.Hashes) != 1 {
				s.sugar.Errorf("provider version %s (%s) has multiple available hashes", versionNumber, osAndArch)
				continue
			}
			os, arch, found := strings.Cut(osAndArch, "_")
			if !found {
				s.sugar.Errorf("provider version %s (%s) did not have expected split delimiter _", versionNumber, osAndArch)
				continue
			}
			psib := ProviderSpecificInstanceBinary{
				FullPath:   filepath.Join(s.downloadRoot, s.provider.String(), hashesAndUrl.URL),
				H1Checksum: hashesAndUrl.Hashes[0], // right now there's only the h1 hash so we just pick the first one
				ProviderSpecificInstance: ProviderSpecificInstance{
					Provider: s.provider,
					Version:  versionNumber,
					OS:       os,
					Arch:     arch,
				},
			}
			sugar.Debugf("generated PSIB %#v", psib)
			psibs = append(psibs, psib)
		}
	}

	return psibs, nil
}

func (s fsProviderStorage) VerifyCatalogAgainstStorage(catalog []ProviderSpecificInstanceBinary) (validLocalBinaries []ProviderSpecificInstanceBinary, invalidLocalBinaries []ProviderSpecificInstanceBinary, err error) {
	sugar.Debugf("verifying catalog data: %v", catalog)

	for _, pib := range catalog {
		hash, err := dirhash.HashZip(pib.FullPath, dirhash.Hash1)
		if err != nil {
			sugar.Debugf("err: %w", err)
			invalidLocalBinaries = append(invalidLocalBinaries, pib)
			continue
		}
		if hash != pib.H1Checksum {
			sugar.Errorf("checksum %s did not match expected %s", hash, pib.H1Checksum)
			invalidLocalBinaries = append(invalidLocalBinaries, pib)
			continue
		}

		validLocalBinaries = append(validLocalBinaries, pib)
	}

	sugar.Debugf("valid local binaries: %v", validLocalBinaries)
	sugar.Debugf("invalid local binaries: %v", invalidLocalBinaries)

	return validLocalBinaries, invalidLocalBinaries, nil
}

func (s fsProviderStorage) ReconcileWantedProviderInstances(
	validPSIBs []ProviderSpecificInstanceBinary,
	invalidPSIBs []ProviderSpecificInstanceBinary,
	wantedProviderInstances []ProviderSpecificInstance,
) (reconciledPIs []ProviderSpecificInstance) {
	dedupe := make(map[ProviderSpecificInstance]string)

	for _, x := range invalidPSIBs {
		dedupe[x.ProviderSpecificInstance] = ""
	}
	for _, x := range wantedProviderInstances {
		dedupe[x] = ""
	}
	for _, x := range validPSIBs {
		delete(dedupe, x.ProviderSpecificInstance)
	}

	var retval []ProviderSpecificInstance
	for k := range dedupe {
		retval = append(retval, k)
	}
	return retval
}

func (s fsProviderStorage) WriteProviderBinaryDataToStorage(binaryData []byte, pi ProviderSpecificInstance) (psib *ProviderSpecificInstanceBinary, err error) {
	dirPath := filepath.Join(s.downloadRoot, pi.GetDownloadBase())
	err = os.MkdirAll(dirPath, os.FileMode(0755))
	if err != nil {
		return nil, err
	}

	fullPath := filepath.Join(dirPath, pi.GetDownloadedFileName())
	err = os.WriteFile(fullPath, binaryData, os.FileMode(0644))
	if err != nil {
		return nil, err
	}

	hash, err := dirhash.HashZip(fullPath, dirhash.Hash1)
	if err != nil {
		return nil, err
	}

	return &ProviderSpecificInstanceBinary{
		ProviderSpecificInstance: pi,
		H1Checksum:               hash,
		FullPath:                 fullPath,
	}, nil
}

func (s fsProviderStorage) StoreCatalog(psibs []ProviderSpecificInstanceBinary) error {
	versionMap := make(map[string][]ProviderSpecificInstanceBinary)
	mirrorIndex := MirrorIndex{
		Versions: make(map[string]map[string]any),
	}

	for _, psib := range psibs {
		if _, ok := versionMap[psib.Version]; !ok {
			versionMap[psib.Version] = []ProviderSpecificInstanceBinary{}
		}
		versionMap[psib.Version] = append(versionMap[psib.Version], psib)
	}

	for version, binaries := range versionMap {
		mirrorArchives := MirrorArchives{
			Archives: make(map[string]MirrorProviderPlatformArch),
		}

		for _, binary := range binaries {
			osArch := fmt.Sprintf("%s_%s", binary.OS, binary.Arch)
			mirrorArchives.Archives[osArch] = MirrorProviderPlatformArch{
				Hashes: []string{binary.H1Checksum},
				URL:    binary.GetDownloadedFileName(),
			}
		}

		versionJson, err := json.MarshalIndent(mirrorArchives, "", "  ")
		if err != nil {
			return fmt.Errorf("error marshalling version JSON: %w", err)
		}

		versionJsonPath := filepath.Join(s.downloadRoot, s.provider.GetDownloadBase(), fmt.Sprintf("%s.json", version))
		err = os.WriteFile(versionJsonPath, versionJson, os.FileMode(0644))
		if err != nil {
			return fmt.Errorf("error writing version JSON: %w", err)
		}

		mirrorIndex.Versions[version] = make(map[string]any)
	}

	mirrorIndexJson, err := json.MarshalIndent(mirrorIndex, "", "  ")
	if err != nil {
		return fmt.Errorf("error marshalling mirror index JSON: %w", err)
	}
	mirrorIndexJsonPath := filepath.Join(s.downloadRoot, s.provider.GetDownloadBase(), MIRROR_INDEX_FILE)
	err = os.WriteFile(mirrorIndexJsonPath, mirrorIndexJson, os.FileMode(0644))
	if err != nil {
		return fmt.Errorf("error writing index JSON: %w", err)
	}

	return nil
}
