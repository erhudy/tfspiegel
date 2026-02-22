package main

// type used for the config
type ProviderMirrorConfiguration struct {
	Reference    string                 `json:"reference" yaml:"reference"`
	VersionRange string                 `json:"version_range" yaml:"version_range"`
	SkipVersions []string               `json:"skip_versions" yaml:"skip_versions"`
	OSArchs      []HCTFProviderPlatform `json:"os_archs" yaml:"os_archs"`
}

type configRaw struct {
	Providers   []ProviderMirrorConfiguration `json:"providers" yaml:"providers"`
	StorageType string                        `json:"storage_type" yaml:"storage_type"`
	FSConfig    fsConfig                      `json:"fs_config,omitempty" yaml:"fs_config,omitempty"`
	S3Config    s3Config                      `json:"s3_config,omitempty" yaml:"s3_config,omitempty"`
}

type fsConfig struct {
	DownloadRoot string `json:"download_root" yaml:"download_root"`
}

type s3Config struct {
	Bucket   string `json:"bucket" yaml:"bucket"`
	Endpoint string `json:"endpoint,omitempty" yaml:"endpoint,omitempty"`
	Prefix   string `json:"prefix,omitempty" yaml:"prefix,omitempty"`
}

type Configuration struct {
	Providers           []ProviderMirrorConfiguration
	DownloadDestination DownloadDestination
}
