# Management Cluster Setup

## Create a Management cluster

All the below steps are expected to be performed by an organization administrator.

### Create a bootstrap Kubernetes cluster

It is a common practice to create a temporary bootstrap cluster which is then used to provision a
management cluster on the Cloud Director (infrastructure provider).

Choose one of the options below to set up a management cluster on VMware Cloud Director:

1. [CSE](https://github.com/vmware/container-service-extension) provisioned TKG cluster as a bootstrap cluster to
   further create a Management cluster in VCD tenant organization.
2. [Kind as a bootstrap cluster](https://cluster-api.sigs.k8s.io/user/quick-start.html#install-andor-configure-a-kubernetes-cluster)
   to create Management cluster in VCD tenant organization.
   
The next step is to initialize the bootstrap cluster with the Cluster API   

<a name="management_cluster_init"></a>
### Initialize the cluster with Cluster API
1. Set up the [clusterctl](CLUSTERCTL.md#clusterctl_set_up) on the cluster
2. [Initialize the Cluster API and CAPVCD](CLUSTERCTL.md#init_management_cluster) on the cluster
3. [Apply CRS definitions](CRS.md#apply_crs) to ensure CPI, CNI, and CSI are installed on the (children) workload clusters
4. Wait until `kubectl get pods -A` shows below pods in Running state
    1. ```shell
       kubectl get pods -A
       NAMESPACE                           NAME                                                            READY   STATUS
       capi-kubeadm-bootstrap-system       capi-kubeadm-bootstrap-controller-manager-7dc44947-v5nlv        1/1     Running
       capi-kubeadm-control-plane-system   capi-kubeadm-control-plane-controller-manager-cb9d954f5-ct5cp   1/1     Running
       capi-system                         capi-controller-manager-7594c7bc57-smjtg                        1/1     Running
       capvcd-system                       capvcd-controller-manager-769d64d4bf-54bf4                      1/1     Running
       ```

### Create a multi-controlplane management cluster
Now that bootstrap cluster is ready, you can use Cluster API to create a multi control-plane workload cluster

1. Generate the [cluster manifest](CLUSTERCTL.md#generate_cluster_manifest) and apply it on the bootstrap cluster to create a brand-new workload cluster. 
2. [Apply CRS labels](CRS.md#apply_crs_labels) on the workload cluster.
3. The next step is to [enable add-ons (CPI, CSI)](CRS.md#enable_add_ons) on the workload cluster to access VCD resources
4. Transform this workload cluster into management cluster by [initializing it with Cluster API and CAPVCD](#management_cluster_init).
5. This cluster is now a fully functional multi-control plane management cluster; you can use this to create and manage 
   multiple workload clusters.
   
## Resize, Upgrade and Delete Management cluster
These workflows need to be run from the bootstrap cluster (the parent of the management cluster).

In the `kubectl` commands specified in the below workflows, update the `namespace` parameter to the value `default`
and `kubeconfig` to the value of bootstrap cluster's admin Kubeconfig
* [Resize workflow](WORKLOAD_CLUSTER.md#resize_workload_cluster)
* [Upgrade workflow](WORKLOAD_CLUSTER.md#upgrade_workload_cluster)
* [Delete workflow](WORKLOAD_CLUSTER.md#delete_workload_cluster)

<a name="tenant_user_management"></a>
## Enable multitenancy on the management cluster

This is an advanced workflow to enable Cloud Director tenant users to deploy workload clusters from a single management 
cluster in an isolated manner.

Refer to [enable multitenancy](MULTITENANCY.md) on the management cluster for more details

## Configure Machine Health Check on the management cluster

Configuring Machine Health Checks on the management cluster will instruct Cluster API to detect unhealthy machines of a given cluster and remediate them.

Refer to [Machine Health Checks](MHC.md) for more details.
