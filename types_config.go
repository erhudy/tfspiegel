package main

// type used for the config
type ProviderMirrorConfiguration struct {
	Reference    string                 `json:"reference"`
	VersionRange string                 `json:"version_range"`
	OSArchs      []HCTFProviderPlatform `json:"os_archs"`
}

type configRaw struct {
	Providers   []ProviderMirrorConfiguration `json:"providers"`
	StorageType string                        `json:"storage_type"`
	FSConfig    fsConfig                      `json:"fs_config,omitempty"`
	S3Config    s3Config                      `json:"s3_config,omitempty"`
}

type fsConfig struct {
	DownloadRoot string `json:"download_root"`
}

type s3Config struct {
	Bucket   string `json:"bucket"`
	Endpoint string `json:"endpoint,omitempty"`
	Prefix   string `json:"prefix,omitempty"`
}

type Configuration struct {
	Providers           []ProviderMirrorConfiguration
	DownloadDestination DownloadDestination
}
