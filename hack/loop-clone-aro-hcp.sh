#!/bin/bash

set -o errexit
set -o nounset
set -o pipefail

mkdir -p local-aro
pushd local-aro

while true; do
    # Create new directory for fresh clone
    rm -rf ARO-HCP-next
    mkdir ARO-HCP-next

    # Clone into new directory
    git clone https://github.com/Azure/ARO-HCP.git ARO-HCP-next

    # Replace old with new
    rm -rf ARO-HCP
    mv ARO-HCP-next ARO-HCP

    # Wait for an hour before next update
    echo "sleeping for 1h"
    sleep 3600
done

popd


