BINARY_NAME=service-status

DOCKER := $(or $(DOCKER),podman)


build:
	go build -o ${BINARY_NAME} ./cmd/service-status

test:
	go test ./...

clean:
	go clean
	rm -rf ${BINARY_NAME}

update:
	hack/update-aro-hcp-types.sh

images:
	$(DOCKER) build .
