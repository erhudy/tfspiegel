package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"

	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

func main() {
	var configPath string
	var loggerType string
	var loop bool
	var waitBetweenLoops time.Duration

	flag.StringVar(&configPath, "config-path", "config.yaml", "Path to configuration file")
	flag.StringVar(&loggerType, "logger-type", "development", "Logger type (development or production)")
	flag.BoolVar(&loop, "loop", false, "Loop on mirroring providers after a wait period")
	flag.DurationVar(&waitBetweenLoops, "wait-between-loops", 6*time.Hour, "How long to wait between mirroring attempts when looping")
	flag.Parse()

	if !StringInSlice(loggerType, []string{"development", "production"}) {
		panic(fmt.Errorf("%s is not a valid logger type", loggerType))
	}

	config, err := LoadConfig(configPath)
	if err != nil {
		panic(err)
	}

	var logger *zap.Logger
	if loggerType == "development" {
		logger, _ = zap.NewDevelopment()
	} else if loggerType == "production" {
		logger, _ = zap.NewProduction()
	}
	defer logger.Sync()

	sugar = logger.Sugar()

	if loop {
		for {
			err = MirrorProvidersWithConfig(config, logger)
			if err != nil {
				panic(err)
			}
			sugar.Infof("sleeping %s until next loop", waitBetweenLoops)
			time.Sleep(waitBetweenLoops)
		}
	} else {
		err = MirrorProvidersWithConfig(config, logger)
		if err != nil {
			panic(err)
		}
	}
}

func LoadConfig(configPath string) (config Configuration, err error) {
	var configRaw configRaw
	configData, err := os.ReadFile(configPath)
	if err != nil {
		return config, err
	}
	err = yaml.Unmarshal(configData, &configRaw)
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
		DownloadDestination: DownloadDestination{
			Type:     storageType,
			FSConfig: configRaw.FSConfig,
			S3Config: configRaw.S3Config,
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

		osarchs := configProvider.OSArchs
		if len(osarchs) < 1 {
			osarchs = []HCTFProviderPlatform{{runtime.GOOS, runtime.GOARCH}}
			sugar.Warnf("provider %s does not have OS/archs set, using current platform (%s/%s) as defaults", provider, runtime.GOOS, runtime.GOARCH)
		}

		wantedProviderVersionedInstances, err := provider.FilterToWantedPVIs(providerMetadata, configProvider.VersionRange, osarchs)
		if err != nil {
			sugar.Errorf("error fetching wanted provider version instances for provider %s: %w", provider, err)
			continue
		}

		var pvisToDownload []ProviderSpecificInstance
		var d ProviderDownloader

		if config.DownloadDestination.Type == STORAGE_TYPE_FS {
			d = ProviderDownloader{
				Storage: FSProviderStorageConfiguration{
					downloadRoot:            config.DownloadDestination.FSConfig.DownloadRoot,
					provider:                provider,
					sugar:                   sugar,
					wantedProviderInstances: wantedProviderVersionedInstances,
				},
			}
		} else if config.DownloadDestination.Type == STORAGE_TYPE_S3 {
			ctx := context.Background()
			awscfg, err := awsconfig.LoadDefaultConfig(ctx)
			if err != nil {
				return err
			}
			if config.DownloadDestination.S3Config.Endpoint != "" {
				const defaultRegion = "us-east-1"
				staticResolver := aws.EndpointResolverWithOptionsFunc(func(service, region string, options ...interface{}) (aws.Endpoint, error) {
					return aws.Endpoint{
						PartitionID:       "aws",
						URL:               config.DownloadDestination.S3Config.Endpoint,
						SigningRegion:     defaultRegion,
						HostnameImmutable: true,
					}, nil
				})

				awscfg.Region = defaultRegion
				awscfg.EndpointResolverWithOptions = staticResolver
			}
			prefix := config.DownloadDestination.S3Config.Prefix

			s3client := awss3.NewFromConfig(awscfg)

			d = ProviderDownloader{
				Storage: S3ProviderStorageConfiguration{
					bucket:                  config.DownloadDestination.S3Config.Bucket,
					context:                 ctx,
					prefix:                  prefix,
					provider:                provider,
					s3client:                *s3client,
					sugar:                   sugar,
					wantedProviderInstances: wantedProviderVersionedInstances,
				},
			}
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
		if len(pvisToDownload) > 0 {
			sugar.Debugf("%s\n", marshalled)
		}

		var psibs []ProviderSpecificInstanceBinary
		psibs = append(psibs, valid...)

		// we need to record failed downloads as well so that we can exclude that entire version from the catalog,
		// in instances where some particular OS+arch combo of a provider fails to download for some reason
		failedPvis := []ProviderSpecificInstance{}

		for _, pvi := range pvisToDownload {
			psib, err := d.MirrorProviderInstanceToDest(pvi)
			if err != nil {
				sugar.Errorf("error mirroring provider instance %s: %w", pvi, err)
				failedPvis = append(failedPvis, pvi)
				continue
			}
			psibs = append(psibs, *psib)
		}

		finalPsibs := FilterVersionsWithFailedPSIBs(psibs, failedPvis)

		err = d.Storage.StoreCatalog(finalPsibs)
		if err != nil {
			sugar.Errorf("error writing catalog for provider %s: %w", provider, err)
			continue
		}
	}

	return nil
}
