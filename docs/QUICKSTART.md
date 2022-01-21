# Quick start guide to create Kubernetes workload cluster on vCloud Director

In this tutorial we’ll cover the basics of how to use Cluster API provider - CAPVCD to create one or more Kubernetes 
clusters on vCloud Director.

## Installation

### Common Prerequisites

* Install and setup [kubectl](https://kubernetes.io/docs/tasks/tools/) in your local environment 
* Install [Kind](https://kind.sigs.k8s.io/) and [Docker](https://www.docker.com/)

### Install and/or configure a Kubernetes cluster

Cluster API requires an existing Kubernetes cluster accessible via kubectl. During the installation 
process the Kubernetes cluster will be transformed into a [management cluster](https://cluster-api.sigs.k8s.io/reference/glossary.html#management-cluster) 
by installing the Cluster API provider component - CAPVCD, so it is recommended to keep it separated from any application workload.

It is a common practice to create a temporary, local bootstrap cluster which is then used to provision a 
target management cluster on the selected infrastructure provider (CAPVCD in our case).
  
Choose one of the options below to set up a management cluster:

1. [Kind as a bootstrap cluster](https://cluster-api.sigs.k8s.io/user/quick-start.html#install-andor-configure-a-kubernetes-cluster) 
   to create Management cluster in vCD tenant organization
2. [CSE](https://github.com/vmware/container-service-extension) provisioned TKGm cluster as a bootstrap cluster to create Management cluster in vCD tenant organization.

(It is recommended to have at least one management cluster per organization).

### Install clusterctl
[Install clusterctl](https://cluster-api.sigs.k8s.io/user/quick-start.html#install-clusterctl)

### Prepare VCD organization, organization VDC and CAPVCD user role

Refer [VCD SETUP](VCD_SETUP.md) for setting up AVI Controller, NSX-T cloud and CAPVCD user role.

<a name="management_cluster_init"></a>
### Initialize the management cluster
Now that we’ve got all the prerequisites in place, let’s transform the Kubernetes cluster into 
a management cluster by using `clusterctl init`.

The command lets us choose the infrastructure provider to install. However, CAPVCD 0.5 is not yet part of the provider list 
supported by `clusterctl init`. Hence, we will provide separate set of commands to initialize the infrastructure provider component.
Run below commands against the bootstrap Kubernetes cluster created above

1. Install cluster-api core provider, kubeadm bootstrap and kubeadm control-plane providers
    1. `clusterctl init --core cluster-api:v0.4.2 -b kubeadm:v0.4.2 -c kubeadm:v0.4.2`
2. Install Infrastructure provider - CAPVCD 
    1. Download CAPVCD repo - `git clone git@github.com:vmware/cluster-api-provider-cloud-director.git`
    2. Fill in the VCD details in `cluster-api-provider-cloud-director/config/manager/controller_manager_config.yaml`
    3. Input username and password in `config/manager/kustomization.yaml`. Refer to the rights required for the role [here](VCD_SETUP.md)
    4. Run the command `kubectl apply -k config/default`
   
Wait until `kubectl get pods -A` shows below pods in Running state
```
> kubectl get pods -A
NAMESPACE                           NAME                                                            READY   STATUS 
capi-kubeadm-bootstrap-system       capi-kubeadm-bootstrap-controller-manager-7dc44947-v5nlv        1/1     Running 
capi-kubeadm-control-plane-system   capi-kubeadm-control-plane-controller-manager-cb9d954f5-ct5cp   1/1     Running
capi-system                         capi-controller-manager-7594c7bc57-smjtg                        1/1     Running 
capvcd-system                       capvcd-controller-manager-769d64d4bf-54bf4                      1/1     Running
```  
Now that bootstrap management cluster is ready, you can use Cluster API to create multi control-plane workload clusters fronted by 
load balancers. You can choose to re-transform the workload clusters into management clusters by repeating the 
[CAPVCD initialization step](#management_cluster_init)

### Create your first workload cluster
Once the management cluster is ready, you can create your first workload cluster.

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

   


