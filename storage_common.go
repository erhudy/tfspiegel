package main

func commonReconcileWantedProviderInstances(
	validPSIBs []ProviderSpecificInstanceBinary,
	invalidPSIBs []ProviderSpecificInstanceBinary,
	wantedProviderInstances []ProviderSpecificInstance,
) (reconciledPIs []ProviderSpecificInstance) {
	dedupe := make(map[ProviderSpecificInstance]string)

	for _, x := range invalidPSIBs {
		dedupe[x.ProviderSpecificInstance] = ""
	}
	for _, x := range wantedProviderInstances {
		dedupe[x] = ""
	}
	for _, x := range validPSIBs {
		delete(dedupe, x.ProviderSpecificInstance)
	}

	var retval []ProviderSpecificInstance
	for k := range dedupe {
		retval = append(retval, k)
	}
	return retval
}
