default: build

build:
	go build -o terraform-provider-notion

install: build
	mkdir -p ~/.terraform.d/plugins/registry.terraform.io/andrew/notion/0.1.0/$$(go env GOOS)_$$(go env GOARCH)
	cp terraform-provider-notion ~/.terraform.d/plugins/registry.terraform.io/andrew/notion/0.1.0/$$(go env GOOS)_$$(go env GOARCH)/

testacc:
	TF_ACC=1 go test ./internal/provider/ -v -timeout 120m

generate:
	go generate ./...

.PHONY: build install testacc generate
