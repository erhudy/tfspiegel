package main

// types used when writing out the JSON for the provider mirror protocol
type MirrorProviderPlatformArch struct {
	Hashes []string `json:"hashes"` // this hash is the h1: type from dirhash
	URL    string   `json:"url"`
}

type MirrorArchives struct {
	Archives map[string]MirrorProviderPlatformArch `json:"archives"`
}

type MirrorIndex struct {
	Versions map[string]map[string]any `json:"versions"`
}
