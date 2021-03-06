package main

import (
	"context"

	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"go.uber.org/zap"
)

type ProviderDownloader struct {
	Storage ProviderStorer
}

type FSProviderStorageConfiguration struct {
	downloadRoot            string
	provider                Provider
	sugar                   *zap.SugaredLogger
	wantedProviderInstances []ProviderSpecificInstance
}

type S3ProviderStorageConfiguration struct {
	bucket                  string
	context                 context.Context
	prefix                  string
	provider                Provider
	s3client                awss3.Client
	sugar                   *zap.SugaredLogger
	wantedProviderInstances []ProviderSpecificInstance
}

// we can't checksum files directly in S3
type S3ObjectChecksum struct {
	ETag       string
	H1Checksum string
}
