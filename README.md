# service-status
Project to harvest information about managed services and render it.

## To use
1. Extract github.com/ARO/ARO-HCP somewhere
2. Log into quay using podman (yeah, doesn't work with docker)
3. `make && ./service-status aro hcp release-markdown --aro-hcp-dir=/home/deads/workspaces/aro-hcp/src/github.com/Azure/ARO-HCP/ --output-dir=artifacts`
4. Review releases.md and then in each release, the environment-comparison.md is particularly interesting.
5. There is content in each release for content in each environment.

