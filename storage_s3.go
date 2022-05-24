package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	awss3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/xorcare/pointer"
	"golang.org/x/mod/sumdb/dirhash"
)

func (s S3ProviderStorageConfiguration) LoadCatalog() ([]ProviderSpecificInstanceBinary, error) {
	indexFullPath := filepath.Join(s.prefix, s.provider.String(), MIRROR_INDEX_FILE)
	indexObjectOutput, err := s.s3client.GetObject(s.context, &awss3.GetObjectInput{
		Bucket: &s.bucket,
		Key:    &indexFullPath,
	})
	if err != nil {
		s.sugar.Errorf("unable to get index file %s from S3: %w", indexFullPath, err)
		return nil, fmt.Errorf("error loading catalog: %w", err)
	}
	indexContents, err := io.ReadAll(indexObjectOutput.Body)
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

	// because we cannot get the h1 hash of an object in S3 directly,
	// we instead store another metadata file in S3 that has a list of object keys
	// with their expected etags and the H1 checksum that we recorded when we downloaded
	// the provider, and then we check the etag against the object's current etag -
	// if the etag matches what we recorded, then we assume the object is okay
	etagMapFullPath := filepath.Join(s.prefix, s.provider.String(), S3_ETAG_MAP_FILE)
	etagMapObjectOutput, err := s.s3client.GetObject(s.context, &awss3.GetObjectInput{
		Bucket: &s.bucket,
		Key:    &etagMapFullPath,
	})
	if err != nil {
		s.sugar.Errorf("unable to get etag map file %s from S3: %w", etagMapFullPath, err)
		return nil, fmt.Errorf("error loading catalog: %w", err)
	}
	etagMapContents, err := io.ReadAll(etagMapObjectOutput.Body)
	if err != nil {
		s.sugar.Errorf("unable to read etag map file %s: %w", etagMapFullPath, err)
		return nil, fmt.Errorf("error loading catalog: %w", err)
	}
	var etagMap map[string]S3ObjectChecksum
	err = json.Unmarshal(etagMapContents, &etagMap)
	if err != nil {
		return nil, err
	}
	sugar.Debugf("unmarshalled etag map: %v", etagMap)

	var psibs []ProviderSpecificInstanceBinary
	// per Hashicorp docs at https://www.terraform.io/internals/provider-network-mirror-protocol#sample-response
	// the value for each key is currently an empty object
	for versionNumber := range index.Versions {
		sugar.Debugf("examining version %s", versionNumber)
		versionJsonFullPath := filepath.Join(s.prefix, s.provider.String(), fmt.Sprintf("%s.json", versionNumber))
		versionJsonObjectOutput, err := s.s3client.GetObject(s.context, &awss3.GetObjectInput{
			Bucket: &s.bucket,
			Key:    &versionJsonFullPath,
		})
		if err != nil {
			s.sugar.Errorf("unable to get version JSON file %s from S3: %w", versionJsonFullPath, err)
			continue
		}
		versionJsonContents, err := io.ReadAll(versionJsonObjectOutput.Body)
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
			// it's okay if there's no etag entry for this object, we'll just redownload it
			etag, ok := etagMap[hashesAndUrl.URL]
			if !ok {
				s.sugar.Debugf("no etag map entry for provider version %s (%s), will redownload", versionNumber, osAndArch)
			}

			psib := ProviderSpecificInstanceBinary{
				FullPath:         filepath.Join(s.prefix, s.provider.String(), hashesAndUrl.URL),
				H1Checksum:       hashesAndUrl.Hashes[0], // right now there's only the h1 hash so we just pick the first one
				S3ObjectChecksum: etag,
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

func (s S3ProviderStorageConfiguration) VerifyCatalogAgainstStorage(
	catalog []ProviderSpecificInstanceBinary,
) (
	validLocalBinaries []ProviderSpecificInstanceBinary,
	invalidLocalBinaries []ProviderSpecificInstanceBinary,
	err error,
) {
	sugar.Debugf("verifying catalog data: %v", catalog)

	storagePath := filepath.Join(s.prefix, s.provider.String())
	var continuationToken *string
	doneListing := false
	objects := make(map[string]awss3types.Object)
	for {
		if doneListing {
			break
		}
		input := awss3.ListObjectsV2Input{
			Bucket:            &s.bucket,
			ContinuationToken: continuationToken,
			Prefix:            &storagePath,
		}
		objectListOutput, err := s.s3client.ListObjectsV2(s.context, &input)
		if err != nil {
			sugar.Errorf("error listing objects from S3: %w", err)
			return nil, nil, fmt.Errorf("unable to list objects from S3: %w", err)
		}
		for _, object := range objectListOutput.Contents {
			objects[*object.Key] = object
		}
		doneListing = !objectListOutput.IsTruncated
		continuationToken = objectListOutput.NextContinuationToken
	}

	sugar.Debugf("got objects from S3: %#v\n", objects)

	for _, pib := range catalog {
		sugar.Debugf("looking for %s in S3 object map", pib.FullPath)
		matchingObject, ok := objects[pib.FullPath]
		if !ok {
			sugar.Debugf("not found")
			invalidLocalBinaries = append(invalidLocalBinaries, pib)
			continue
		}

		// for an object to be considered valid the etag and H1 must both match
		s.sugar.Debugf("%s: comparing etags local '%s' : remote '%s'", pib.FullPath, pib.S3ObjectChecksum.ETag, *matchingObject.ETag)
		s.sugar.Debugf("%s: comparing H1 local '%s' : remote '%s'", pib.FullPath, pib.S3ObjectChecksum.H1Checksum, pib.H1Checksum)
		if pib.S3ObjectChecksum.ETag == *matchingObject.ETag && pib.S3ObjectChecksum.H1Checksum == pib.H1Checksum {
			validLocalBinaries = append(validLocalBinaries, pib)
		} else {
			invalidLocalBinaries = append(invalidLocalBinaries, pib)
		}
	}

	return validLocalBinaries, invalidLocalBinaries, nil
}

func (s S3ProviderStorageConfiguration) ReconcileWantedProviderInstances(
	validPSIBs []ProviderSpecificInstanceBinary,
	invalidPSIBs []ProviderSpecificInstanceBinary,
	wantedProviderInstances []ProviderSpecificInstance,
) (reconciledPIs []ProviderSpecificInstance) {
	return commonReconcileWantedProviderInstances(validPSIBs, invalidPSIBs, wantedProviderInstances)
}

func (s S3ProviderStorageConfiguration) WriteProviderBinaryDataToStorage(
	binaryData []byte,
	pi ProviderSpecificInstance,
) (psib *ProviderSpecificInstanceBinary, err error) {
	// in order to compute the H1 we will need to write the binary data to temporary local storage first
	file, err := os.CreateTemp("", "tfspiegel-*.zip")
	defer os.Remove(file.Name())
	defer file.Close()
	if err != nil {
		return nil, err
	}
	_, err = file.Write(binaryData)
	if err != nil {
		return nil, err
	}

	hash, err := dirhash.HashZip(file.Name(), dirhash.Hash1)
	if err != nil {
		return nil, err
	}

	// now upload to S3
	key := filepath.Join(s.prefix, pi.Provider.GetDownloadBase(), pi.GetDownloadedFileName())
	// TODO also calculate SHA256 b64 encoded and provide it here
	reader := bytes.NewReader(binaryData)
	putObjectOutput, err := s.s3client.PutObject(s.context, &awss3.PutObjectInput{
		Bucket: &s.bucket,
		Key:    &key,
		Body:   reader,
	})
	if err != nil {
		return nil, err
	}
	s.sugar.Debugf("Uploaded %s to S3", key)
	psib = &ProviderSpecificInstanceBinary{
		ProviderSpecificInstance: pi,
		H1Checksum:               hash,
		S3ObjectChecksum: S3ObjectChecksum{
			ETag:       *putObjectOutput.ETag,
			H1Checksum: hash,
		},
		FullPath: key,
	}
	return psib, nil
}

func (s S3ProviderStorageConfiguration) StoreCatalog(psibs []ProviderSpecificInstanceBinary) error {
	versionMap := make(map[string][]ProviderSpecificInstanceBinary)
	mirrorIndex := MirrorIndex{
		Versions: make(map[string]map[string]any),
	}

	etagMap := make(map[string]S3ObjectChecksum)

	for _, psib := range psibs {
		etagMap[psib.GetDownloadedFileName()] = psib.S3ObjectChecksum

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

		versionJsonPath := filepath.Join(s.prefix, s.provider.GetDownloadBase(), fmt.Sprintf("%s.json", version))

		reader := bytes.NewReader(versionJson)
		_, err = s.s3client.PutObject(s.context, &awss3.PutObjectInput{
			Body:   reader,
			Bucket: &s.bucket,
			Key:    &versionJsonPath,
		})

		if err != nil {
			return fmt.Errorf("error writing version JSON: %w", err)
		}

		mirrorIndex.Versions[version] = make(map[string]any)
	}

	mirrorIndexJson, err := json.MarshalIndent(mirrorIndex, "", "  ")
	if err != nil {
		return fmt.Errorf("error marshalling mirror index JSON: %w", err)
	}
	mirrorIndexJsonPath := filepath.Join(s.prefix, s.provider.GetDownloadBase(), MIRROR_INDEX_FILE)

	mirrorIndexReader := bytes.NewReader(mirrorIndexJson)
	_, err = s.s3client.PutObject(s.context, &awss3.PutObjectInput{
		Body:        mirrorIndexReader,
		Bucket:      &s.bucket,
		Key:         &mirrorIndexJsonPath,
		ContentType: pointer.String("application/json"),
	})

	if err != nil {
		return fmt.Errorf("error writing index JSON: %w", err)
	}

	// now write out our special etag map
	etagMapJson, err := json.MarshalIndent(etagMap, "", "  ")
	if err != nil {
		return fmt.Errorf("error marshalling etag map JSON: %w", err)
	}
	etagMapJsonPath := filepath.Join(s.prefix, s.provider.GetDownloadBase(), S3_ETAG_MAP_FILE)

	etagReader := bytes.NewReader(etagMapJson)
	_, err = s.s3client.PutObject(s.context, &awss3.PutObjectInput{
		Body:        etagReader,
		Bucket:      &s.bucket,
		Key:         &etagMapJsonPath,
		ContentType: pointer.String("application/json"),
	})
	if err != nil {
		return fmt.Errorf("error writing etag map JSON: %w", err)
	}

	return nil
}
