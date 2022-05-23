package main

import "go.uber.org/zap"

type RemoteProviderMetadata struct {
	Provider
	ID       string                `json:"id"`
	Versions []HCTFProviderVersion `json:"versions"`
	Warnings *[]string             `json:"warnings"`
}

type ProviderDownloader struct {
	Storage ProviderStorer
}

type ProviderStorageConfiguration struct {
	downloadRoot            string
	provider                Provider
	sugar                   *zap.SugaredLogger
	wantedProviderInstances []ProviderSpecificInstance
}

// types used when downloading
type Provider struct {
	Hostname string
	Owner    string
	Name     string
}

type ProviderSpecificInstance struct {
	Provider
	Version string
	OS      string
	Arch    string
}

type ProviderSpecificInstanceBinary struct {
	ProviderSpecificInstance
	H1Checksum string
	FullPath   string
}

type ProviderStorageType int

const (
	STORAGE_TYPE_FS ProviderStorageType = iota
	STORAGE_TYPE_S3
)

type DownloadDestination struct {
	Type     ProviderStorageType
	Location string
}

type ProviderStorer interface {
	LoadCatalog() ([]ProviderSpecificInstanceBinary, error)
	VerifyCatalogAgainstStorage(catalog []ProviderSpecificInstanceBinary) (validLocalBinaries []ProviderSpecificInstanceBinary, invalidLocalBinaries []ProviderSpecificInstanceBinary, err error)
	ReconcileWantedProviderInstances(validPSIBs []ProviderSpecificInstanceBinary, invalidPSIBs []ProviderSpecificInstanceBinary, wantedProviderInstances []ProviderSpecificInstance) []ProviderSpecificInstance
	WriteProviderBinaryDataToStorage(binaryData []byte, pi ProviderSpecificInstance) (psib *ProviderSpecificInstanceBinary, err error)
	StoreCatalog([]ProviderSpecificInstanceBinary) error
}

// we can't checksum files directly in S3
type S3ObjectEntry struct {
	ETag           string
	Key            string
	SHA256Checksum string
}
