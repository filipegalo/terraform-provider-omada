# VERSION comes from the git tag, which is what a release is cut from and what
# the registries publish. Between tags `git describe` appends the commit count
# and hash, which is still valid semver, so local installs never collide with a
# released version. Override for a one-off (make install VERSION=x.y.z).
GIT_VERSION := $(patsubst v%,%,$(shell git describe --tags --dirty 2>/dev/null))
VERSION ?= $(if $(GIT_VERSION),$(GIT_VERSION),0.0.0-dev)
OS_ARCH := $(shell go env GOOS)_$(shell go env GOARCH)
BINARY  := terraform-provider-omada_v$(VERSION)
PLUGIN_ROOT := $(HOME)/.terraform.d/plugins/local/filipegalo/omada
PLUGIN_DIR := $(PLUGIN_ROOT)/$(VERSION)/$(OS_ARCH)

# Platforms install-all populates the mirror for. The host platform is the one
# actually run; the others exist so a consumer's .terraform.lock.hcl can record
# a checksum for them, which `tofu providers lock` can only do for a package
# that is already present. Override to add one (make install-all
# PLATFORMS="darwin_arm64 linux_arm64").
PLATFORMS ?= $(OS_ARCH) linux_amd64

.PHONY: build install install-all install-platform test testacc generate lint

build:
	go build -ldflags "-X main.version=$(VERSION)" -o $(BINARY)

# install builds the provider for this machine and drops it into Terraform's
# implied local mirror directory, so any `terraform init` on this machine that
# requires local/filipegalo/omada at this version finds it with no registry and
# no network access.
install:
	@$(MAKE) --no-print-directory install-platform PLATFORM=$(OS_ARCH)

# install-all additionally cross-compiles for every other platform in
# PLATFORMS. Use it when a consumer needs a lock file valid on more than this
# machine; from that repo, record the checksums with:
#
#   tofu providers lock -fs-mirror="$$HOME/.terraform.d/plugins" \
#     -platform=darwin_arm64 -platform=linux_amd64
install-all:
	@for platform in $(PLATFORMS); do \
		$(MAKE) --no-print-directory install-platform PLATFORM=$$platform; \
	done

# install-platform builds one GOOS_GOARCH into its own mirror directory. Go
# cross-compiles this provider with no cgo, so every platform builds from any
# host. Call it directly for a one-off (make install-platform PLATFORM=linux_arm64).
install-platform:
	@mkdir -p $(PLUGIN_ROOT)/$(VERSION)/$(PLATFORM)
	GOOS=$(word 1,$(subst _, ,$(PLATFORM))) GOARCH=$(word 2,$(subst _, ,$(PLATFORM))) \
		go build -ldflags "-X main.version=$(VERSION)" \
		-o $(PLUGIN_ROOT)/$(VERSION)/$(PLATFORM)/$(BINARY)
	@echo "installed $(BINARY) for $(PLATFORM)"

test:
	go test ./...

# testacc runs the acceptance tests, which create and destroy real objects on a
# real controller. Needs OMADA_URL/USERNAME/PASSWORD/SITE plus the per-suite
# variables documented in the *_acc_test.go files.
testacc:
	TF_ACC=1 go test -v -cover -timeout 120m ./...

# generate rebuilds docs/ from the provider schema and examples/. CI fails if
# the result differs from what is committed.
generate:
	terraform fmt -recursive ./examples/
	go generate ./...

lint:
	golangci-lint run
