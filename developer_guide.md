# CAPVCD Developer guide

## Pre-release check-list
* Create a feature branch if needed and add features to the chosen release-branches.
* Bump up the CAPVCD release version.
* Bump up the RDE version.
* Decide whether it is necessary to bump up the CAPVCD API version.

## RDE management
We strongly recommend that developers evaluate upgrades for RDE Versions every time a minor version of CAPVCD is released.
Bump up the RDE major version only if you are anticipating breaking changes.
### opening Upgrade path (for minor/patch upgrade )
1. Create new schema file under /schemas/schema_x_y_z.json
2. Locate the function ConvertToLatestRDEVersionFormat in the [`pkg/capisdk/defined_entity.go`](pkg/capisdk/defined_entity.go).
3. Make sure to invoke the correct converter function to convert the source RDE version into the latest RDE version that's being used.
4. Update the existing converters or add new converter functions to create upgrade paths (for example, `convertFrom100Format()`)
- Provide an automatic conversion of the content in `srcCapvcdEntity.entity.status.CAPVCD` content to the latest RDE version format (`types.CAPVCDStatus`)
- Add the placeholder for any special conversion logic inside `types.CAPVCDStatus`.
- Update the `srcCapvcdEntity.entityType` to the latest RDE version that's being used.
- Call the API update call to update CAPVCD entity and persist data into VCD.

## CAPVCD API Upgrade
We strongly recommend that developers evaluate upgrades for CAPVCD APIs every time a minor version of CAPVCD is released. This section outlines the general steps that must be performed for every CAPVCD API version bump. For detailed information, please refer to the internal technical documentation.

* Create New APIs with the new Version (e.g., `v1beta1`).
* Enable clusterctl upgrade command and do the upgrade to the new version.
* Enable CAPVCD handle other supported **`API versions`**.
* Enable conversion webhooks in CAPVCD.
* Enable validation and mutation webhooks in CAPVCD.
* Enable webhooks in our Custom Resource Definitions (CRDs).
* Implement Converters between existing old API version and new API version.
* Update Labels on CRDs.
* Restore fields in conversion.


## Dev Set up
To set up your development environment, install [Kubectl](https://kubernetes.io/docs/tasks/tools/), [Kind](https://kind.sigs.k8s.io/), [Docker](https://www.docker.com/) and [Clusterctl](https://cluster-api.sigs.k8s.io/user/quick-start.html#install-clusterctl) in your local environment. It is recommended to create the management cluster first, and then create the workload cluster. For detailed instructions, please refer to the [DEV Setup](docs/QUICKSTART.md) documentation, which provides step-by-step guidance on the setup process.

## Post-release check-list
* Collect user feedback to identify any areas for improvement or new feature requests.
* Update any relevant documentation, such as user manuals, API documentation, and template samples.



