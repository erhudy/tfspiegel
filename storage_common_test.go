package main

import (
	"sort"
	"testing"
)

func psiSetEqual(a, b []ProviderSpecificInstance) bool {
	if len(a) != len(b) {
		return false
	}
	sortPSIs := func(s []ProviderSpecificInstance) {
		sort.Slice(s, func(i, j int) bool {
			return s[i].String() < s[j].String()
		})
	}
	aCopy := make([]ProviderSpecificInstance, len(a))
	bCopy := make([]ProviderSpecificInstance, len(b))
	copy(aCopy, a)
	copy(bCopy, b)
	sortPSIs(aCopy)
	sortPSIs(bCopy)
	for i := range aCopy {
		if aCopy[i] != bCopy[i] {
			return false
		}
	}
	return true
}

func TestCommonReconcile_AllNew(t *testing.T) {
	wanted := []ProviderSpecificInstance{
		{Provider: Provider{Hostname: "r", Owner: "o", Name: "n"}, Version: "1.0.0", OS: "linux", Arch: "amd64"},
		{Provider: Provider{Hostname: "r", Owner: "o", Name: "n"}, Version: "2.0.0", OS: "linux", Arch: "amd64"},
	}

	result := commonReconcileWantedProviderInstances(nil, nil, wanted)
	if !psiSetEqual(result, wanted) {
		t.Errorf("expected all wanted returned, got %v", result)
	}
}

func TestCommonReconcile_AllValid(t *testing.T) {
	psi := ProviderSpecificInstance{
		Provider: Provider{Hostname: "r", Owner: "o", Name: "n"},
		Version:  "1.0.0", OS: "linux", Arch: "amd64",
	}
	valid := []ProviderSpecificInstanceBinary{
		{ProviderSpecificInstance: psi, H1Checksum: "h1:abc"},
	}
	wanted := []ProviderSpecificInstance{psi}

	result := commonReconcileWantedProviderInstances(valid, nil, wanted)
	if len(result) != 0 {
		t.Errorf("expected empty result, got %v", result)
	}
}

func TestCommonReconcile_InvalidCausesRedownload(t *testing.T) {
	psi := ProviderSpecificInstance{
		Provider: Provider{Hostname: "r", Owner: "o", Name: "n"},
		Version:  "1.0.0", OS: "linux", Arch: "amd64",
	}
	invalid := []ProviderSpecificInstanceBinary{
		{ProviderSpecificInstance: psi, H1Checksum: "h1:bad"},
	}

	result := commonReconcileWantedProviderInstances(nil, invalid, nil)
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	if result[0] != psi {
		t.Errorf("expected %v, got %v", psi, result[0])
	}
}

func TestCommonReconcile_MixedState(t *testing.T) {
	psiValid := ProviderSpecificInstance{
		Provider: Provider{Hostname: "r", Owner: "o", Name: "n"},
		Version:  "1.0.0", OS: "linux", Arch: "amd64",
	}
	psiInvalid := ProviderSpecificInstance{
		Provider: Provider{Hostname: "r", Owner: "o", Name: "n"},
		Version:  "2.0.0", OS: "linux", Arch: "amd64",
	}
	psiNew := ProviderSpecificInstance{
		Provider: Provider{Hostname: "r", Owner: "o", Name: "n"},
		Version:  "3.0.0", OS: "linux", Arch: "amd64",
	}

	valid := []ProviderSpecificInstanceBinary{
		{ProviderSpecificInstance: psiValid, H1Checksum: "h1:good"},
	}
	invalid := []ProviderSpecificInstanceBinary{
		{ProviderSpecificInstance: psiInvalid, H1Checksum: "h1:bad"},
	}
	wanted := []ProviderSpecificInstance{psiValid, psiInvalid, psiNew}

	result := commonReconcileWantedProviderInstances(valid, invalid, wanted)
	expected := []ProviderSpecificInstance{psiInvalid, psiNew}
	if !psiSetEqual(result, expected) {
		t.Errorf("expected %v, got %v", expected, result)
	}
}

func TestCommonReconcile_EmptyInputs(t *testing.T) {
	result := commonReconcileWantedProviderInstances(nil, nil, nil)
	if len(result) != 0 {
		t.Errorf("expected empty result, got %v", result)
	}
}
