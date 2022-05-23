package main

// type used for the config
type ProviderMirrorConfiguration struct {
	Reference    string                 `json:"reference"`
	VersionRange string                 `json:"version_range"`
	OSArchs      []HCTFProviderPlatform `json:"os_archs"`
}

type configRaw struct {
	Providers    []ProviderMirrorConfiguration `json:"providers"`
	DownloadRoot string                        `json:"download_root"`
	StorageType  string                        `json:"storage_type"`
}

type Configuration struct {
	Providers    []ProviderMirrorConfiguration
	DownloadRoot DownloadDestination
}
