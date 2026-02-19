package main

import (
	"os"
	"testing"

	"go.uber.org/zap"
)

func TestMain(m *testing.M) {
	logger, _ := zap.NewDevelopment()
	sugar = logger.Sugar()
	os.Exit(m.Run())
}

type mockProviderStorer struct {
	loadCatalogFunc                      func() ([]ProviderSpecificInstanceBinary, error)
	verifyCatalogAgainstStorageFunc      func(catalog []ProviderSpecificInstanceBinary) ([]ProviderSpecificInstanceBinary, []ProviderSpecificInstanceBinary, error)
	reconcileWantedProviderInstancesFunc func(validPSIBs []ProviderSpecificInstanceBinary, invalidPSIBs []ProviderSpecificInstanceBinary, wantedProviderInstances []ProviderSpecificInstance) []ProviderSpecificInstance
	writeProviderBinaryDataToStorageFunc func(binaryData []byte, pi ProviderSpecificInstance) (*ProviderSpecificInstanceBinary, error)
	storeCatalogFunc                     func([]ProviderSpecificInstanceBinary) error
}

func (m mockProviderStorer) LoadCatalog() ([]ProviderSpecificInstanceBinary, error) {
	return m.loadCatalogFunc()
}

func (m mockProviderStorer) VerifyCatalogAgainstStorage(catalog []ProviderSpecificInstanceBinary) ([]ProviderSpecificInstanceBinary, []ProviderSpecificInstanceBinary, error) {
	return m.verifyCatalogAgainstStorageFunc(catalog)
}

func (m mockProviderStorer) ReconcileWantedProviderInstances(validPSIBs []ProviderSpecificInstanceBinary, invalidPSIBs []ProviderSpecificInstanceBinary, wantedProviderInstances []ProviderSpecificInstance) []ProviderSpecificInstance {
	return m.reconcileWantedProviderInstancesFunc(validPSIBs, invalidPSIBs, wantedProviderInstances)
}

func (m mockProviderStorer) WriteProviderBinaryDataToStorage(binaryData []byte, pi ProviderSpecificInstance) (*ProviderSpecificInstanceBinary, error) {
	return m.writeProviderBinaryDataToStorageFunc(binaryData, pi)
}

func (m mockProviderStorer) StoreCatalog(psibs []ProviderSpecificInstanceBinary) error {
	return m.storeCatalogFunc(psibs)
}
