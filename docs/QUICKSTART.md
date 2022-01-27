# Quick Start

In this tutorial we’ll cover the basics of how to use Cluster API to create one or more Kubernetes 
clusters on VMware Cloud Director. This document expects the readers to be familiar with the 
[Core CAPI](https://cluster-api.sigs.k8s.io/introduction.html) terminology like management cluster and workload cluster.

### VMware Cloud Director Setup

Refer to [Cloud Director setup](VCD_SETUP.md) for setting up the infrastructure and user roles.

### Common Prerequisites

Install below in your local environment
* [Kubectl](https://kubernetes.io/docs/tasks/tools/) 
* [Kind](https://kind.sigs.k8s.io/) and [Docker](https://www.docker.com/)
* [Clusterctl](https://cluster-api.sigs.k8s.io/user/quick-start.html#install-clusterctl)

<a name="management_cluster_setup"></a>
### Create a Management cluster

It is recommended for VCD organization administrator to create at least one management cluster per tenant.
Refer to [Management cluster set up](MANAGEMENT_CLUSTER.md) for the detailed steps.

### Create a workload cluster
Once the management cluster is created and prepared for tenants' access, tenants can create their workload cluster(s).

Refer to [workload cluster operations](WORKLOAD_CLUSTER.md) for more details.
   
### Clean up
Delete Management cluster (Yet to be filled)

   


