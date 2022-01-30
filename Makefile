.DEFAULT_GOAL := default

.PHONY: default
default:
	go build -o tfspiegel *.go
