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
  - Create the workload cluster
    - Get the clusterName and clusterNameSpace from capiYaml
    - Create the namespace when necessary
    - Apply the objects from capiYaml
    - Wait for the cluster becomes `provisioned` and all the machine states become `provisioned`
    - Validate the kubeConfig of the workload cluster
    - Apply the CRS of CNI/CPI/CSI
    - Wait for all the machine states become `running`
  - Resize the workload cluster
    - increase the worker pool node count of the workload cluster
    - monitor the new machine show up and then become running
    - decrease the worker pool node count of the workload cluster
    - monitor the replicas of machines become correct
  - delete the workload cluster
    - delete all the objects from capiYaml except `secret`
    - Ensure the workload cluster is deleted
    - Delete the cluster name space (Secret - capi-user-credentials is deleted at the same time)

## Tests to be added
* test the upgrade on an existing workload cluster
* automate the management cluster creation. Test the management cluster creation and workload cluster creation together.
* test the VCD resource name change and validate CAPVCD to fallback and use the correct VCD name change.