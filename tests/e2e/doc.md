# CAPVCD automation tests

## Workload cluster CRUD based on the completed management Cluster
- Test Prerequisites
  - The management cluster can be accessible.
  - The management cluster has CAPI controller and CAPVCD controller.
  - User prepares the completed cluster yaml. See [clusterctl template flavors](../../docs/CLUSTERCTL.md#template_flavors) for the details.
  
Note: **You can also use [setup script](setup_kind_cluster.sh) to set up the kind cluster**. 
1. The script only works for mac OS. 
2. The script is expected to export the `GOROOT`already.


- Test input
  - PathToKubeconfigOfManagementCluster (absolute path to find the file)
  - PathToCapiYamlOfWorkloadCluster (absolute path to find the file)
  


- Test Workflow
  - use kubeConfigPath and capiYamlPath to get kubeConfig and capiYaml.
  - use kubeConfigPath of management cluster to apply capiYaml.
  - validate the workload cluster
  - get the kubeConfig of workload cluster
  - Configure CSI and CPI in the workload cluster. [instructions](../../docs/CRS.md#enable-cpi-and-csi-on-the-workload-cluster-to-access-vmware-cloud-director-resources)
  - resize the worker pool node of the workload cluster
  - monitor the new machine becomes provisioned
  - delete the workload cluster

## Tests to be added
* test the upgrade on an existing workload cluster
* automate the management cluster creation. Test the management cluster creation and workload cluster creation together.
* test the VCD resource name change and validate CAPVCD to fallback and use the correct VCD name change.