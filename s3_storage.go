package main

type s3ProviderStorage struct {
	config Configuration
}

func NewS3ProviderStorage(config Configuration) s3ProviderStorage {
	return s3ProviderStorage{
		config: config,
	}
}

func (this s3ProviderStorage) GetStoredProviderInstances() {

}
