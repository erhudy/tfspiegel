# tfspiegel

`tfspiegel` is a client that implements the Terraform network provider mirror protocol. I wrote it because I felt that `terraform providers mirror` was not flexible enough and I wanted to be able to mirror provider versions without needing to reference actual Terraform provider configurations.

## Current features

* mirror to S3 (either AWS or S3-like such as Minio/Ceph) or local filesystem
* mirror complex semantic version ranges (ranges are specified using [blang/semver](https://github.com/blang/semver#ranges) syntax)
* only mirror what needs to be mirrored, i.e. missing file or wrong checksum
* attempt to loop and re-mirror providers on a set interval without needing to be run from cron

## Upcoming features

* mirror to Artifactory
* proper tests
* proper release process
* clean up user UX
* fix some error handling issues
* example Kubernetes manifests

## Usage

* build with `make`
* use `config.yaml.example` as a model for how to set it up
* if using S3 storage, all authentication/region variables are expected to be provided via normal AWSCLI environment variables
* configure `.terraformrc` according to the [Hashicorp documentation](https://www.terraform.io/cli/config/config-file)

**IMPORTANT:** Terraform mandates the use of HTTPS for the network provider mirror.