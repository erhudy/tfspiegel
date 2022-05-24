package main

type RemoteProviderMetadata struct {
	Provider
	ID       string                `json:"id"`
	Versions []HCTFProviderVersion `json:"versions"`
	Warnings *[]string             `json:"warnings"`
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
	H1Checksum       string
	S3ObjectChecksum S3ObjectChecksum // only relevant for S3 - probably a better way to organize this but this is fast
	FullPath         string
}

type ProviderStorageType int

const (
	STORAGE_TYPE_FS ProviderStorageType = iota
	STORAGE_TYPE_S3
)

type DownloadDestination struct {
	Type     ProviderStorageType
	FSConfig fsConfig
	S3Config s3Config
}

type ProviderStorer interface {
	LoadCatalog() ([]ProviderSpecificInstanceBinary, error)
	VerifyCatalogAgainstStorage(catalog []ProviderSpecificInstanceBinary) (validLocalBinaries []ProviderSpecificInstanceBinary, invalidLocalBinaries []ProviderSpecificInstanceBinary, err error)
	ReconcileWantedProviderInstances(validPSIBs []ProviderSpecificInstanceBinary, invalidPSIBs []ProviderSpecificInstanceBinary, wantedProviderInstances []ProviderSpecificInstance) []ProviderSpecificInstance
	WriteProviderBinaryDataToStorage(binaryData []byte, pi ProviderSpecificInstance) (psib *ProviderSpecificInstanceBinary, err error)
	StoreCatalog([]ProviderSpecificInstanceBinary) error
}
