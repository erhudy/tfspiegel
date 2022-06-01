package main

func StringInSlice(s string, slice []string) bool {
	found := false
	for _, x := range slice {
		if x == s {
			found = true
			break
		}
	}
	return found
}
