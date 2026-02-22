package main

func StringInSlice(s string, slice []string) bool {
	for _, x := range slice {
		if x == s {
			return true
		}
	}
	return false
}
