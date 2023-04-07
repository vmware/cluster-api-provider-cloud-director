# CAPVCD Upgrades

## Upgrades from CAPVCD 0.5.x to 1.0.0

1. Delete the capvcd 0.5.x deployment - `kubectl delete deployment -n capvcd-system capvcd-controller-manager`
2. Run `clusterctl upgrade apply --contract v1beta1`
3. Configure [clusterctl](CLUSTERCTL.md#clusterctl_set_up) to refer to local CAPVCD 1.0.0 manifests.
4. Run `clusterctl init -i vcd:v1.0.0`

## Upgrades from CAPVCD 0.5.x to 1.1.0

1. Delete the capvcd 0.5.x deployment - `kubectl delete deployment -n capvcd-system capvcd-controller-manager`
2. Run `clusterctl upgrade apply --contract v1beta1`
3. Configure [clusterctl](CLUSTERCTL.md#clusterctl_set_up) to refer to local CAPVCD 1.1.0 manifests.
4. Run `clusterctl init -i vcd:v1.1.0`

## Upgrades from CAPVCD 1.0.0 to 1.1.0
1. Configure [clusterctl](CLUSTERCTL.md#clusterctl_set_up) to refer to local CAPVCD 1.1.0 manifests.
2. Run `clusterctl upgrade apply -i capvcd-system/vcd:v1.1.0`
