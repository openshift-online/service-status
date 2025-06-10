#!/bin/bash

go get github.com/atombender/go-jsonschema/...
go install github.com/atombender/go-jsonschema@latest

go-jsonschema \
    --capitalization AKS,AZ,KV,SAN,ARM,ACR,ARO,DNS,OIDC,MCE,OCP,JSON,MSI,NSP,RG,API,CSI\
    --min-sized-ints \
    --only-models \
    -p github.com/openshift-online/service-status/pkg/apis/aro-hcp \
    /home/deads/workspaces/aro-hcp/src/github.com/Azure/ARO-HCP/config/config.schema.json \
    > pkg/apis/aro-hcp/config.go

sed -i 's/package aro-hcp/package arohcpapi/' pkg/apis/aro-hcp/config.go