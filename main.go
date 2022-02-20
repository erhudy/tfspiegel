package main

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"

	"go.uber.org/zap"
)

func LoadConfig() (Config, error) {
	var config Config
	var configRaw configRaw
	configData, err := ioutil.ReadFile("config.json")
	if err != nil {
		return config, err
	}
	err = json.Unmarshal(configData, &configRaw)
	if err != nil {
		return config, err
	}

	config = Config{
		Providers: configRaw.Providers,
		DownloadTo: DownloadDestination{
			Type:     DOWNLOAD_DESTINATION_FS,
			Location: configRaw.DownloadTo,
		},
	}

	return config, nil
}

func main() {
	config, err := LoadConfig()
	if err != nil {
		panic(err)
	}

	err = MirrorProvidersWithConfig(config)
	if err != nil {
		panic(err)
	}
}

func MirrorProvidersWithConfig(config Config) error {
	logger, _ := zap.NewProduction()
	defer logger.Sync()
	sugar = logger.Sugar()

	var mirrorIndex MirrorIndex
	mirrorIndex.Versions = make(map[string]map[string]interface{})

	// loop through all requested provider mirror stanzas in the config and mirror each provider set one at a time
	for _, configProvider := range config.Providers {
		provider, err := NewProvider(configProvider.Reference)
		if err != nil {
			return err
		}
		providerMetadata, err := GetProviderMetadataFromRegistry(provider)
		if err != nil {
			return err
		}

		filteredProviders, err := FilterToWantedProviderInstances(provider, providerMetadata, configProvider.VersionRange, configProvider.OSArchs)
		if err != nil {
			return err
		}

		// TODO fix this for S3
		d := ProviderDownloader{
			storage: NewLocalProviderStorage(config.DownloadTo.Location),
		}
		for _, filteredProvider := range filteredProviders {
			err = d.mirrorProviderInstanceToDest(filteredProvider, config.DownloadTo)
			if err != nil {
				sugar.Errorf("error mirroring provider: %s", err)
			}
			mirrorIndex.Versions[filteredProvider.Version] = make(map[string]interface{})
		}

		if len(filteredProviders) > 0 {
			indexJSONPath := filepath.Join(config.DownloadTo.Location, filteredProviders[0].GetDownloadPath(), "index.json")
			indexMarshalled, err := json.Marshal(&mirrorIndex)
			if err != nil {
				return err
			}
			err = ioutil.WriteFile(indexJSONPath, indexMarshalled, os.FileMode(0644))
			if err != nil {
				return err
			}
		}
	}

	return nil
}
