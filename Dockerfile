FROM registry.access.redhat.com/ubi9/ubi:latest AS builder
WORKDIR /go/src/service-status
RUN dnf install -y \
        git \
        go \
        podman \
        make
COPY . .
ENV PATH="/go/bin:${PATH}"
ENV GOPATH="/go"
RUN make build

FROM registry.access.redhat.com/ubi9/ubi:latest AS base
COPY --from=builder /go/src/service-status/service-status /bin/service-status
COPY --from=builder /go/src/service-status/hack/loop-clone-aro-hcp.sh /bin/loop-clone-aro-hcp.sh
ENTRYPOINT ["/bin/service-status"]
EXPOSE 8080
