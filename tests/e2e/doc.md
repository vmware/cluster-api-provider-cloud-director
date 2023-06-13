# CAPVCD automation tests

## Workload cluster CRUD based on the completed management Cluster

- Test input
  - KubeConfigPath (absolute path to find the file)
  - capiYamlPath (absolute path to find the file)
  - host
  - org
  - userOrg
  - ovdc
  - userName
  - refreshToken

- Test Preflight
  - The management cluster can be accessible - **You can also use [setup script](setup_kind_cluster.sh) to set up the kind cluster**
  - The management cluster has CAPI controller and CAPVCD controller.
  - User prepares the completed cluster yaml. See [clusterctl template flavors](CLUSTERCTL.md#template_flavors) for the details.


- Test Workflow
  - use kubeConfigPath and capiYamlPath to get kubeConfig and capiYaml.
  - use kubeConfigPath of management to apply capiYaml.
  - validate the workload cluster
  - get the kubeConfig of workload cluster
  TODO: apply CSI/CPI config map  https://github.com/ymo24/cluster-api-provider-cloud-director/blob/main/docs/CRS.md#enable-cpi-and-csi-on-the-workload-cluster-to-access-vmware-cloud-director-resources
  - resize the worker pool node of the workload cluster
  - monitor the new machine becomes provisioned
  - delete the workload cluster

## Tests to be added
* test the upgrade on an existing workload cluster
* automate the management cluster creation. Test the management cluster creation and workload cluster creation together.
* test the VCD resource name change and validate CAPVCD to fallback and use the correct VCD name change.