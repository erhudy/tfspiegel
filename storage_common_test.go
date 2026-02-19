package main

import (
	"sort"
	"testing"
)

func sortPSIs(psis []ProviderSpecificInstance) {
	sort.Slice(psis, func(i, j int) bool {
		return psis[i].String() < psis[j].String()
	})
}

func psiSlicesEqual(a, b []ProviderSpecificInstance) bool {
	if len(a) != len(b) {
		return false
	}
	sortPSIs(a)
	sortPSIs(b)
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestCommonReconcileWantedProviderInstances(t *testing.T) {
	psi1 := ProviderSpecificInstance{
		Provider: Provider{Hostname: "registry.terraform.io", Owner: "hashicorp", Name: "aws"},
		Version:  "5.0.0", OS: "linux", Arch: "amd64",
	}
	psi2 := ProviderSpecificInstance{
		Provider: Provider{Hostname: "registry.terraform.io", Owner: "hashicorp", Name: "aws"},
		Version:  "5.1.0", OS: "linux", Arch: "amd64",
	}
	psi3 := ProviderSpecificInstance{
		Provider: Provider{Hostname: "registry.terraform.io", Owner: "hashicorp", Name: "aws"},
		Version:  "5.0.0", OS: "darwin", Arch: "arm64",
	}

	psib1 := ProviderSpecificInstanceBinary{ProviderSpecificInstance: psi1, H1Checksum: "h1:abc"}
	psib2 := ProviderSpecificInstanceBinary{ProviderSpecificInstance: psi2, H1Checksum: "h1:def"}
	psib3 := ProviderSpecificInstanceBinary{ProviderSpecificInstance: psi3, H1Checksum: "h1:ghi"}

	tests := []struct {
		name     string
		valid    []ProviderSpecificInstanceBinary
		invalid  []ProviderSpecificInstanceBinary
		wanted   []ProviderSpecificInstance
		expected []ProviderSpecificInstance
	}{
		{
			"all new",
			nil,
			nil,
			[]ProviderSpecificInstance{psi1, psi2},
			[]ProviderSpecificInstance{psi1, psi2},
		},
		{
			"all already valid",
			[]ProviderSpecificInstanceBinary{psib1, psib2},
			nil,
			[]ProviderSpecificInstance{psi1, psi2},
			nil,
		},
		{
			"invalid gets re-queued",
			nil,
			[]ProviderSpecificInstanceBinary{psib1},
			nil,
			[]ProviderSpecificInstance{psi1},
		},
		{
			"valid removes from invalid",
			[]ProviderSpecificInstanceBinary{psib1},
			[]ProviderSpecificInstanceBinary{psib1},
			nil,
			nil,
		},
		{
			"deduplication between invalid and wanted",
			nil,
			[]ProviderSpecificInstanceBinary{psib1},
			[]ProviderSpecificInstance{psi1},
			[]ProviderSpecificInstance{psi1},
		},
		{
			"empty inputs",
			nil,
			nil,
			nil,
			nil,
		},
		{
			"mixed scenario",
			[]ProviderSpecificInstanceBinary{psib1},
			[]ProviderSpecificInstanceBinary{psib2},
			[]ProviderSpecificInstance{psi3},
			[]ProviderSpecificInstance{psi2, psi3},
		},
		{
			"valid trumps invalid",
			[]ProviderSpecificInstanceBinary{psib3},
			[]ProviderSpecificInstanceBinary{psib3},
			[]ProviderSpecificInstance{psi1},
			[]ProviderSpecificInstance{psi1},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := commonReconcileWantedProviderInstances(tt.valid, tt.invalid, tt.wanted)
			if !psiSlicesEqual(got, tt.expected) {
				t.Errorf("got %v, want %v", got, tt.expected)
			}
		})
	}
}
