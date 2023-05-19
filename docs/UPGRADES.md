# CAPVCD Upgrades

## Upgrades from CAPVCD 0.5.x to 1.0.0

1. Delete the capvcd 0.5.x deployment - `kubectl delete deployment -n capvcd-system capvcd-controller-manager`
2. Run `clusterctl upgrade apply --contract v1beta1`
3. Configure [clusterctl](CLUSTERCTL.md#clusterctl_set_up) to refer to local CAPVCD 1.0.0 manifests.
4. Run `clusterctl init -i vcd:v1.0.0`

## Upgrades from CAPVCD 1.0.x to 1.0.2
To upgrade CAPVCD from verison 1.0.x to 1.0.2, a patch for CAPVCD deployment to update the image will be needed. Please execute the following command for each cluster:

```kubectl patch deployment -n capvcd-system capvcd-controller-manager -p '{"spec": {"template": {"spec": {"containers": [{"name": "manager", "image": "projects.registry.vmware.com/vmware-cloud-director/cluster-api-provider-cloud-director:1.0.2"}]}}}}'```

