# service-status
Project to harvest information about managed services and render it.

## To use
1. Extract github.com/ARO/ARO-HCP somewhere
2. Log into quay using podman (yeah, doesn't work with docker)
3. `make && ./service-status aro hcp release-markdown --aro-hcp-dir=/home/deads/workspaces/aro-hcp/src/github.com/Azure/ARO-HCP/ --output-dir=artifacts`
4. Review releases.md and then in each release, the environment-comparison.md is particularly interesting.
5. There is content in each release for content in each environment.

# Infrastructure

CI configuration https://github.com/openshift/release/tree/master/ci-operator/config/openshift-online/service-status

Site configuration is located at https://github.com/openshift/continuous-release-jobs/blob/master/argocd/clusters/apps/projects/openshift-online/service-status

Secrets can be edited by chatting with folks in #forum-ocp-crt and following the docs [here](https://github.com/openshift/continuous-release-jobs/blob/master/docs/openshift-dpcr-access.md#secrets)
