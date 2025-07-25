#!/bin/bash
set -o errexit
set -o nounset
set -o pipefail

go get github.com/atombender/go-jsonschema/...
go install github.com/atombender/go-jsonschema@latest

go-jsonschema \
    --capitalization AKS,AZ,KV,SAN,ARM,ACR,ARO,DNS,OIDC,MCE,OCP,JSON,MSI,NSP,RG,API,CSI,TCP,ACM,ARO,RP,MDSD,AFEC\
    --min-sized-ints \
    --only-models \
    --minimal-names \
    -p github.com/openshift-online/service-status/pkg/apis/aro-hcp \
    /home/deads/workspaces/aro-hcp/src/github.com/Azure/ARO-HCP/config/config.schema.json \
    > pkg/apis/aro-hcp/config.go

go-jsonschema \
    --capitalization AKS,AZ,KV,SAN,ARM,ACR,ARO,DNS,OIDC,MCE,OCP,JSON,MSI,NSP,RG,API,CSI,TCP,ACM,ARO,RP,MDSD,AFEC\
    --min-sized-ints \
    --only-models \
    --minimal-names \
    -p github.com/openshift-online/service-status/pkg/apis/aro-hcp \
    /home/deads/workspaces/aro-hcp/src/github.com/Azure/ARO-HCP/config/config.meta.schema.json \
    > pkg/apis/aro-hcp/config_meta.go

sed -i 's/package aro-hcp/package arohcpapi/' pkg/apis/aro-hcp/config.go
sed -i 's/package aro-hcp/package arohcpapi/' pkg/apis/aro-hcp/config_meta.go
