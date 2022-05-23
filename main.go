package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"strings"

	"go.uber.org/zap"
)

func main() {
	config, err := LoadConfig()
	if err != nil {
		panic(err)
	}

	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	err = MirrorProvidersWithConfig(config, logger)
	if err != nil {
		panic(err)
	}
}

func LoadConfig() (config Configuration, err error) {
	var configRaw configRaw
	configData, err := ioutil.ReadFile("config.json")
	if err != nil {
		return config, err
	}
	err = json.Unmarshal(configData, &configRaw)
	if err != nil {
		return config, err
	}

	var storageType ProviderStorageType
	switch x := strings.ToLower(configRaw.StorageType); x {
	case "s3":
		storageType = STORAGE_TYPE_S3
	case "fs":
		storageType = STORAGE_TYPE_FS
	default:
		return config, fmt.Errorf("%s is not a known storage type", x)
	}

	config = Configuration{
		Providers: configRaw.Providers,
		DownloadRoot: DownloadDestination{
			Type:     storageType,
			Location: configRaw.DownloadRoot,
		},
	}

	return config, nil
}

func MirrorProvidersWithConfig(config Configuration, logger *zap.Logger) error {
	sugar = logger.Sugar()

	var mirrorIndex MirrorIndex
	mirrorIndex.Versions = make(map[string]map[string]interface{})

	// loop through all requested provider mirror stanzas in the config and mirror each provider set one at a time
	for _, configProvider := range config.Providers {
		provider, err := NewProviderFromConfigProvider(configProvider.Reference)
		if err != nil {
			sugar.Errorf("error creating provider for %#v: %w", configProvider, err)
			continue
		}
		providerMetadata, err := provider.GetProviderMetadataFromRegistry()
		if err != nil {
			sugar.Errorf("error getting metadata from remote registry for provider %s: %w", provider, err)
			continue
		}

		wantedProviderVersionedInstances, err := provider.FilterToWantedPVIs(providerMetadata, configProvider.VersionRange, configProvider.OSArchs)
		if err != nil {
			sugar.Errorf("error fetching wanted provider version instances for provider %s: %w", provider, err)
			continue
		}

		var pvisToDownload []ProviderSpecificInstance
		d := ProviderDownloader{
			Storage: NewFSProviderStorer(ProviderStorageConfiguration{
				downloadRoot:            config.DownloadRoot.Location,
				provider:                provider,
				sugar:                   sugar,
				wantedProviderInstances: wantedProviderVersionedInstances,
			}),
		}
		catalogContents, err := d.Storage.LoadCatalog()
		var valid []ProviderSpecificInstanceBinary
		var invalid []ProviderSpecificInstanceBinary

		gotError := false
		if err == nil {
			valid, invalid, err = d.Storage.VerifyCatalogAgainstStorage(catalogContents)
			if err != nil {
				gotError = true
			}
		} else {
			gotError = true
		}

		if gotError {
			sugar.Errorf("error loading catalog for provider %s: %w", provider, err)
			sugar.Infof("initializing provider %s as fresh", provider)
			pvisToDownload = wantedProviderVersionedInstances
		} else {
			pvisToDownload = d.Storage.ReconcileWantedProviderInstances(valid, invalid, wantedProviderVersionedInstances)
		}

		marshalled, err := json.MarshalIndent(pvisToDownload, "", "  ")
		if err != nil {
			sugar.Errorf("error marshalling provider instances to download for provider %s: %w", provider, err)
			continue
		}
		sugar.Debugf("%s\n", marshalled)

		var psibs []ProviderSpecificInstanceBinary

		for _, pvi := range pvisToDownload {
			psib, err := d.MirrorProviderInstanceToDest(pvi)
			if err != nil {
				sugar.Errorf("error mirroring provider instance %s: %w", pvi, err)
				continue
			}
			psibs = append(psibs, *psib)
		}

		err = d.Storage.StoreCatalog(psibs)
		if err != nil {
			sugar.Errorf("error writing catalog for provider %s: %w", provider, err)
			continue
		}
	}

	return nil
}
