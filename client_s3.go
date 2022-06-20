package main

import (
	"bytes"
	"context"
	"fmt"
	"io"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	awss3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
)

type S3ClientInterface interface {
	GetObjectContents(bucket string, key string) ([]byte, error)
	ListPrefix(bucket string, prefix string) (map[string]awss3types.Object, error)
	PutObjectWithContentType(bucket string, key string, body []byte, contentType *string) (*string, error)
	PutObject(bucket string, key string, body []byte) (*string, error)
	DeleteObject(bucket string, key string) error
}

type TFSpiegelS3Client struct {
	awss3client *awss3.Client
	context     context.Context
}

func NewS3Client(ctx context.Context, endpoint string) (*TFSpiegelS3Client, error) {
	awscfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, err
	}
	if endpoint != "" {
		const defaultRegion = "us-east-1"
		staticResolver := aws.EndpointResolverWithOptionsFunc(func(service, region string, options ...interface{}) (aws.Endpoint, error) {
			return aws.Endpoint{
				PartitionID:       "aws",
				URL:               endpoint,
				SigningRegion:     defaultRegion,
				HostnameImmutable: true,
			}, nil
		})

		awscfg.Region = defaultRegion
		awscfg.EndpointResolverWithOptions = staticResolver
	}

	return &TFSpiegelS3Client{awss3client: awss3.NewFromConfig(awscfg), context: ctx}, nil
}

func (t TFSpiegelS3Client) GetObjectContents(bucket string, key string) ([]byte, error) {
	output, err := t.awss3client.GetObject(t.context, &awss3.GetObjectInput{
		Bucket: &bucket,
		Key:    &key,
	})
	if err != nil {
		return nil, fmt.Errorf("error loading object contents: %w", err)
	}
	contents, err := io.ReadAll(output.Body)
	if err != nil {
		return nil, fmt.Errorf("error loading object contents: %w", err)
	}
	return contents, nil
}

func (t TFSpiegelS3Client) ListPrefix(bucket string, prefix string) (map[string]awss3types.Object, error) {
	var continuationToken *string
	doneListing := false
	objects := make(map[string]awss3types.Object)
	for {
		if doneListing {
			break
		}
		input := awss3.ListObjectsV2Input{
			Bucket:            &bucket,
			ContinuationToken: continuationToken,
			Prefix:            &prefix,
		}
		objectListOutput, err := t.awss3client.ListObjectsV2(t.context, &input)
		if err != nil {
			return nil, fmt.Errorf("error listing objects from S3: %w", err)
		}
		for _, object := range objectListOutput.Contents {
			objects[*object.Key] = object
		}
		doneListing = !objectListOutput.IsTruncated
		continuationToken = objectListOutput.NextContinuationToken
	}

	return objects, nil
}

func (t TFSpiegelS3Client) PutObjectWithContentType(bucket string, key string, body []byte, contentType string) (*string, error) {
	reader := bytes.NewReader(body)

	var poi awss3.PutObjectInput
	if contentType == "" {
		poi = awss3.PutObjectInput{
			Bucket: &bucket,
			Key:    &key,
			Body:   reader,
		}
	} else {
		poi = awss3.PutObjectInput{
			Bucket:      &bucket,
			Key:         &key,
			Body:        reader,
			ContentType: &contentType,
		}
	}

	putObjectOutput, err := t.awss3client.PutObject(t.context, &poi)
	if err != nil {
		return nil, err
	}
	return putObjectOutput.ETag, nil
}

func (t TFSpiegelS3Client) PutObject(bucket string, key string, body []byte) (*string, error) {
	return t.PutObjectWithContentType(bucket, key, body, "")
}

func (t TFSpiegelS3Client) DeleteObject(bucket string, key string) error {
	_, err := t.awss3client.DeleteObject(t.context, &awss3.DeleteObjectInput{Bucket: &bucket, Key: &key})
	if err != nil {
		return err
	}
	return nil
}
