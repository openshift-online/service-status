BINARY_NAME=service-status

build:
	go build -o ${BINARY_NAME} ./cmd/service-status

test:
	go test ./...

clean:
	go clean
	rm -rf ${BINARY_NAME}

update:
	hack/update-aro-hcp-types.sh