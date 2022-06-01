package main

// types used in the registry response that tells you where to download a provider
type HCTFSigningKey struct {
	KeyID          string `json:"key_id"`
	AsciiArmor     string `json:"ascii_armor"`
	TrustSignature string `json:"trust_signature"`
	Source         string `json:"source"`
	SourceURL      string `json:"source_url"`
}

type HCTFRegistryDownloadResponse struct {
	Protocols           []string                    `json:"protocols"`
	OS                  string                      `json:"os"`
	Arch                string                      `json:"arch"`
	Filename            string                      `json:"filename"`
	DownloadURL         string                      `json:"download_url"`
	ShasumsURL          string                      `json:"shasums_url"`
	ShasumsSignatureURL string                      `json:"shasums_signature_url"`
	Shasum              string                      `json:"shasum"`
	SigningKeys         map[string][]HCTFSigningKey `json:"signing_keys"`
}

// types used in the registry response that lists provider versions and platforms
type HCTFProviderPlatform struct {
	OS   string `json:"os"`
	Arch string `json:"arch"`
}

type HCTFProviderVersion struct {
	Version   string                 `json:"version"`
	Protocols []string               `json:"protocols"`
	Platforms []HCTFProviderPlatform `json:"platforms"`
}
