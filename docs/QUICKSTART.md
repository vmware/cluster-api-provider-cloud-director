# Quick start guide to create Kubernetes workload cluster on Cloud Director

In this tutorial we’ll cover the basics of how to use Cluster API provider - CAPVCD to create one or more Kubernetes 
clusters on Cloud Director. This document expects the readers to be familiar with the 
[Core CAPI](https://cluster-api.sigs.k8s.io/introduction.html) terminology like management cluster and workload cluster.

### Cloud Director Set up

Refer to [Cloud Director setup](VCD_SETUP.md) for setting up the infrastructure and user roles.

### Common Prerequisites

Install below in your local environment
* [Kubectl](https://kubernetes.io/docs/tasks/tools/) 
* [Kind](https://kind.sigs.k8s.io/) and [Docker](https://www.docker.com/)
* [Clusterctl](https://cluster-api.sigs.k8s.io/user/quick-start.html#install-clusterctl)

<a name="management_cluster_setup"></a>
### Create Management cluster
Cluster API requires an existing Kubernetes cluster accessible via kubectl. During the installation
process the Kubernetes cluster will be transformed into a [management cluster](https://cluster-api.sigs.k8s.io/reference/glossary.html#management-cluster)
by installing the Core CAPI and Cluster API provider components, so it is recommended to keep it separated from any application workload.

It is recommended for VCD organization administrator to create at least one management cluster per tenant.
Refer to [Management cluster set up](MANAGEMENT_CLUSTER.md) for the detailed steps.

At high-level, below describes the interaction between VCD tenant administrator and tenant user.

Personas:
* Amy - Management Cluster Author ([Tenant Admin](VCD_SETUP.md#user_role))
* John - Workload Cluster Author ([Tenant user](VCD_SETUP.md#user_role))

1. Amy creates a management cluster, and she has access to Admin Kubeconfig of the management cluster.
2. John wants to create a workload cluster; John asks Amy for the access to management cluster.
3. Amy prepares the management cluster by creating a new Kubernetes namespace and service account for John.
4. Amy hands over the newly generated Kubeconfig file with limited privileges to the John.
5. John uses the Kubeconfig to access the management cluster and creates his first workload cluster.

### Create workload cluster
Once the management cluster is created and prepared for tenants' access, tenants can create their workload cluster(s).

Refer to [workload cluster operations](WORKLOAD_CLUSTER.md) for more details.
   
### Clean up
1. Delete workload cluster (Yet to be filled)
2. Delete Management cluster (Yet to be filled)

   


