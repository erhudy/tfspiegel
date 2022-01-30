package main

// types used in the registry response that tells you where to download a provider
type SigningKey struct {
	KeyID          string `json:"key_id"`
	AsciiArmor     string `json:"ascii_armor"`
	TrustSignature string `json:"trust_signature"`
	Source         string `json:"source"`
	SourceURL      string `json:"source_url"`
}

type RegistryDownloadResponse struct {
	Protocols           []string                `json:"protocols"`
	OS                  string                  `json:"os"`
	Arch                string                  `json:"arch"`
	Filename            string                  `json:"filename"`
	DownloadURL         string                  `json:"download_url"`
	ShasumsURL          string                  `json:"shasums_url"`
	ShasumsSignatureURL string                  `json:"shasums_signature_url"`
	Shasum              string                  `json:"shasum"`
	SigningKeys         map[string][]SigningKey `json:"signing_keys"`
}

// types used in the registry response that lists provider versions and platforms
type ProviderPlatform struct {
	OS   string `json:"os"`
	Arch string `json:"arch"`
}

type ProviderVersion struct {
	Version   string             `json:"version"`
	Protocols []string           `json:"protocols"`
	Platforms []ProviderPlatform `json:"platforms"`
}

type ProviderMetadata struct {
	ID       string            `json:"id"`
	Versions []ProviderVersion `json:"versions"`
	Warnings *[]string         `json:"warnings"`
}

// types used when writing out the JSON for the provider mirror protocol
type MirrorProviderPlatformArch struct {
	Hashes []string `json:"hashes"`
	URL    string   `json:"url"`
}

type MirrorProvider struct {
	Archives map[string]MirrorProviderPlatformArch `json:"archives"`
}

type MirrorIndex struct {
	Versions map[string]map[string]interface{} `json:"versions"`
}

// types used when downloading
type Provider struct {
	Hostname string
	Owner    string
	Name     string
}

type ProviderInstance struct {
	Provider
	Version   string
	Platforms []ProviderPlatform
}

// type used for the config
type ConfigProvider struct {
	Reference    string             `json:"reference"`
	VersionRange string             `json:"versionRange"`
	OSArchs      []ProviderPlatform `json:"os_archs"`
}

type DownloadDestination string

type Config struct {
	Providers  []ConfigProvider    `json:"providers"`
	DownloadTo DownloadDestination `json:"download_to"`
}
