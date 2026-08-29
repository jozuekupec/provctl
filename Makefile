.PHONY: test lint golden-update build deb

test:
	go vet ./...
	staticcheck ./...
	go test ./... -race -count=1

lint:
	go vet ./...
	staticcheck ./...

golden-update:
	go test ./internal/render/... -update

build:
	mkdir -p build
	CGO_ENABLED=0 go build -trimpath -o build/provctl ./cmd/provctl

deb:
	./scripts/build-deb.sh
