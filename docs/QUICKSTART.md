# Quick start guide to create Kubernetes workload cluster on vCloud Director

In this tutorial we’ll cover the basics of how to use Cluster API provider - CAPVCD to create one or more Kubernetes 
clusters on Cloud Director. This document expects the readers to be familiar with the Core CAPI terminology like 
management cluster and workload cluster.

## Installation

### Prepare VCD organization, organization VDC and CAPVCD user role

Refer [VCD SETUP](VCD_SETUP.md) for setting up AVI Controller, NSX-T cloud and CAPVCD user role.

At high-level, below describes the interaction between VCD tenant administrator and tenant user.

* Amy - Management Cluster Author (Tenant Admin)
* John - Workload Cluster Author (Tenant user)

Refer to the rights required for the above roles [here](VCD_SETUP.md#user_role)

1. Amy creates a management cluster, and she has access to Admin Kubeconfig of the management cluster.
2. John wants to create a workload cluster; John asks Amy for the access to management cluster.
3. Amy [prepares the management cluster](#create_K8s_svc_account) by creating a new Kubernetes namespace and service account for John.
4. Amy hands over the newly generated Kubeconfig file with limited privileges to the John.
5. John uses the Kubeconfig to access the management cluster and [creates his first workload cluster](#create_workload_cluster).

### Common Prerequisites

* Install [kubectl](https://kubernetes.io/docs/tasks/tools/) in your local environment
* Install [Kind](https://kind.sigs.k8s.io/) and [Docker](https://www.docker.com/)
* Install [clusterctl](https://cluster-api.sigs.k8s.io/user/quick-start.html#install-clusterctl)

### Create Management cluster
Cluster API requires an existing Kubernetes cluster accessible via kubectl. During the installation
process the Kubernetes cluster will be transformed into a [management cluster](https://cluster-api.sigs.k8s.io/reference/glossary.html#management-cluster)
by installing the Core CAPI and Cluster API provider components, so it is recommended to keep it separated from any application workload.

It is recommended for VCD organization administrator to create at least one management cluster per tenant.
Refer to [Management cluster set up](MANAGEMENT_CLUSTER_SET_UP.md) for the detailed steps.

### Create workload cluster
Once the management cluster is ready, tenants can create their first workload cluster.

1. Generate the cluster configuration (`clusterctl generate` command doesn't yet support CAPVCD 0.5 CAPI yaml generation; please use below steps).
    1. Get the sample [capi-quickstart.yaml](https://github.com/vmware/cluster-api-provider-cloud-director/blob/main/examples/capi-quickstart.yaml)
    2. Follow the comments and update the `capi-quickstart.yaml` with the desired configuration for the workload cluster.
2. Apply the workload cluster configuration
    1. `kubectl apply -f capi-quickstart.yaml`. The output is similar to the below
    2. ```
       cluster.cluster.x-k8s.io/capi-quickstart created
       vcdcluster.infrastructure.cluster.x-k8s.io/capi-quickstart created
       vcdmachinetemplate.infrastructure.cluster.x-k8s.io/capi-quickstart-control-plane created
       kubeadmcontrolplane.controlplane.cluster.x-k8s.io/capi-quickstart-control-plane created
       vcdmachinetemplate.infrastructure.cluster.x-k8s.io/capi-quickstart-md0 created
       kubeadmconfigtemplate.bootstrap.cluster.x-k8s.io/capi-quickstart-md0 created
       machinedeployment.cluster.x-k8s.io/capi-quickstart-md0 created
       ```
3. Accessing the workload cluster
   (Yet to be filled)
4. Resize the workload cluster
   (Yet to be filled)
5. Upgrade the workload cluster
   (Yet to be filled)
   
### Clean up
1. Delete workload cluster (Yet to be filled)
2. Delete Management cluster (Yet to be filled)

   


