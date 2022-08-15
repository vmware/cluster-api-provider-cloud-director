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
   to create Management cluster in VCD tenant organization

We recommend CSE provisioned TKG cluster as bootstrap cluster.

<a name="management_cluster_init"></a>
### Initialize the cluster with Cluster API
**Preliminary Notes**: 
1. Typically, the command `clusterctl init` enables the initialization of the core Cluster API and allows the installation of  infrastructure provider specific Cluster API (in this case, CAPVCD). CAPVCD, as of now, is not yet available via
`clusterctl init`. Therefore, a separate set of commands are provided below for the installation purposes.
2. [ClusterResourceSets](https://cluster-api.sigs.k8s.io/tasks/experimental-features/cluster-resource-set.html) will be used to install CPI, CSI and CNI on the cluster. In the instructions below, we will install the following into the management cluster using ClusterResourceSets (please replace the versions by the versions of your interest):
   1. Antrea v0.13.1 as the CNI
   2. CPI for VCD (main)
   3. CSI for VCD (main)

The steps to create the management cluster with CAPVCD and Antrea CNI are as specified below:
1. Checkout CAPVCD source repository from git:
   ```shell
   git clone --branch main https://github.com/vmware/cluster-api-provider-cloud-director.git`
   ```
2. Install cluster-api core provider, kubeadm bootstrap and kubeadm control-plane providers along with Antrea CNI as a ClusterResourceSet:
    1. Enable the feature-gate `EXP_CLUSTER_RESOURCE_SET` to allow the cluster-api core provider to use ClusterResourceSets in the cluster.
       ```shell
       export EXP_CLUSTER_RESOURCE_SET=true
       ```
    2. Use the clusterctl version mentioned in the [support matrix](../README.md#support_matrix) to run the initialization command.
       ```shell
       clusterctl init --core cluster-api:v1.1.3 -b kubeadm:v1.1.3 -c kubeadm:v1.1.3
       ```
    3. Install Antrea using ClusterResourceSets:
       1. Download the Antrea manifest of interest:
          ```shell
          ANTREA_VERSION=v0.13.1
          wget -O antrea.yaml https://github.com/vmware-tanzu/antrea/releases/download/${ANTREA_VERSION}/antrea.yml```
       2. Create a `ConfigMap` from the manifest
          ```shell
          kubectl create configmap antrea-crs-cm --from-file=antrea.yaml
          ```
       3. Create the ClusterResourceSet yaml file from the ConfigMap. A sample ClusterResourceSet that uses the `antrea-crs-cm` ConfigMap is available in the CAPVCD source repository as [antrea-crs.yaml](../examples/antrea-crs.yaml).
         ```shell
         kubectl apply -f examples/antrea-crs.yaml
         ```
       5. Ensure that the [capi-quickstart.yaml](../examples/capi-quickstart.yaml) already has the label set indicating that the Antrea CNI is to be used. The label is as follows:
         ```yaml
         labels:
           cni: antrea
         ```
    4. Install CPI for VCD using ClusterResourceSets:
       1. Download the CPI CRS manifest of interest.
        ```shell
        wget -O cloud-director-ccm.yaml https://raw.githubusercontent.com/vmware/cloud-provider-for-cloud-director/main/manifests/cloud-director-ccm.yaml
        ```
       2. Create a configmap from the manifest:
         ```shell
         kubectl -n ${NAMESPACE} create configmap cloud-director-crs-cm --from-file=cloud-director-ccm.yaml
         ```
       3. Create a ClusterResourceSet yaml file `cloud-director-crs.yaml` from the Configmap. A sample ClusterResourceSet that uses the `cloud-director-crs-cm` Configmap is:
         ```yaml
         ---
         apiVersion: addons.cluster.x-k8s.io/v1beta1
         kind: ClusterResourceSet
         metadata:
         name: cloud-director-crs
         spec:
           clusterSelector:
             matchLabels:
               cluster-name: capi-cluster
           resources:
           - kind: ConfigMap
             name: cloud-director-crs-cm
         ---
         ```
       4. Install the ClusterResourceSet into the management cluster
         ```shell
         kubectl apply -f cloud-director-crs.yaml
         ```
       5. Ensure that the [capi-quickstart.yaml](../examples/capi-quickstart.yaml) already has the label set indicating that the CPI for VCD is to be used. The label is as follows:
         ```yaml
         labels:
           cluster-name: capi-cluster
         ```
       6.Note that CPI is a mandatory component for CAPI. CAPI will not mark the Cluster as `Running` until CPI is installed.  
    5. Install CSI for VCD using ClusterResourceSets:
       1. Download the CSI CRS manifests of interest.
         ```shell
         wget -O csi-controller-crs.yaml https://raw.githubusercontent.com/vmware/cloud-director-named-disk-csi-driver/main/manifests/csi-controller-crs.yaml
         wget -O csi-node-crs.yaml https://raw.githubusercontent.com/vmware/cloud-director-named-disk-csi-driver/main/manifests/csi-node-crs.yaml
         wget -O csi-node.yaml  https://raw.githubusercontent.com/vmware/cloud-director-named-disk-csi-driver/main/manifests/csi-driver.yaml
         ```
       2. Create configmaps from the manifests
         ```shell
         kubectl create configmap csi-controller-crs-cm --from-file=csi-controller.yaml
         kubectl create configmap csi-node-crs-cm --from-file=csi-node.yaml
         kubectl create configmap csi-driver-crs-cm --from-file=csi-driver.yaml
         ```
       3. Create a ClusterResourceSet manifest `csi-crs.yaml` from the configmaps
       ```yaml
       ---
       apiVersion: addons.cluster.x-k8s.io/v1beta1
       kind: ClusterResourceSet
       metadata:
       name: csi-crs
       spec:
         clusterSelector:
           matchLabels:
             cluster-name: capi-cluster
         resources:
         - kind: ConfigMap
           name: csi-controller-crs-cm
         - kind: ConfigMap
           name: csi-node-crs-cm
         - kind: ConfigMap
           name: csi-driver-crs-cm
       ---
       ```
       4. Install the ClusterResourceSet into the management cluster
         ```shell
         kubectl apply -f csi-crs.yaml
         ``` 
       5. Ensure that the [capi-quickstart.yaml](../examples/capi-quickstart.yaml) already has the label set indicating that the CPI for VCD is to be used. The label is as follows:
         ```yaml
         labels:
           cluster-name: capi-cluster
         ```
3. Install Infrastructure provider - CAPVCD
    1. Fill in the VCD details in `cluster-api-provider-cloud-director/config/manager/controller_manager_config.yaml`
    3. Run the command `kubectl apply -k config/default`
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
1. Now that bootstrap cluster is ready, you can use Cluster API to create multi control-plane workload cluster
   fronted by load balancer. Run the below command
    * `kubectl --kubeconfig=bootstrap_cluster.conf apply -f capi.yaml`. To configure the CAPI yaml, refer to [CAPI Yaml configuration](WORKLOAD_CLUSTER.md#capi_yaml).
2. Retrieve the cluster Kubeconfig
    * `clusterctl get kubeconfig {cluster-name} > capi.kubeconfig`
3. Create a secret for the ClusterID in the management cluster. This is a mandatory step.
   1. Get the ClusterID from the management cluster:
       ```shell
       export CLUSTERID=$(kubectl --kubeconfig=bootstrap_cluster.conf get vcdclusters <management cluster name> -o jsonpath="{.status.infraId}")
       ```
   2. Create a secret in the management cluster from the ClusterID:
       ```shell
       kubectl --kubeconfig=capi.kubeconfig -n kube-system create secret generic vcloud-clusterid-secret --from-literal=clusterid=${CLUSTERID}
       ```
4. Transform this cluster into management cluster by [initializing it with CAPVCD](#management_cluster_init).
5. This cluster is now a fully functional multi-control plane management cluster. The next section walks you through
   the steps to make management cluster ready for individual tenant users use


<a name="tenant_user_management"></a>
## Prepare the Management cluster to enable VCD tenant users' access
Below steps enable tenant users to deploy the workload clusters in their own private namespaces of a given management
cluster, while adhering to their own user quota in VCD.

The organization administrator creates a new and unique Kubernetes namespace for
each tenant user and creates a respective Kubernetes configuration with access to only the
required CRDs. This is a one-time operation per VCD tenant user.

Run below commands for each tenant user. The USERNAME and KUBE_APISERVER_ADDRESS parameter should be
changed as per your requirements.

```sh
USERNAME="user1"

NAMESPACE=${USERNAME}-ns
kubectl create ns ${NAMESPACE}

cat > user-rbac.yaml << END
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: ${USERNAME}
  namespace: ${NAMESPACE}
---
kind: Role
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  namespace: ${NAMESPACE}
  name: ${USERNAME}-full-access
rules:
- apiGroups: ["", "extensions", "apps", "cluster.x-k8s.io", "infrastructure.cluster.x-k8s.io", "bootstrap.cluster.x-k8s.io", "controlplane.cluster.x-k8s.io", "apiextensions.k8s.io"]
  resources: ["*"]
  verbs: ["*"]
- apiGroups: ["batch"]
  resources:
  - jobs
  - cronjobs
  verbs: ["*"]
---
kind: RoleBinding
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: ${USERNAME}-view-${NAMESPACE}
  namespace: ${NAMESPACE}
subjects:
- kind: ServiceAccount
  name: ${USERNAME}
  namespace: ${NAMESPACE}
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: ${USERNAME}-full-access
---
END

kubectl create -f user-rbac.yaml

SECRETNAME=$(kubectl -n ${NAMESPACE} describe sa ${USERNAME} | grep "Tokens" | cut -f2 -d: | tr -d " ")
USERTOKEN=$(kubectl -n ${NAMESPACE} get secret ${SECRETNAME} -o "jsonpath={.data.token}" | base64 -d)
CERT=$(kubectl -n ${NAMESPACE} get secret ${SECRETNAME} -o "jsonpath={.data['ca\.crt']}")
KUBE_APISERVER_ADDRESS=https://127.0.0.1:64265

cat > user1-management-kubeconfig.conf <<END
apiVersion: v1
kind: Config
users:
- name: ${USERNAME}
  user:
    token: ${USERTOKEN}
clusters:
- cluster:
    certificate-authority-data: ${CERT}
    server: ${KUBE_APISERVER_ADDRESS}
  name: my-cluster
contexts:
- context:
    cluster: my-cluster
    user: ${USERNAME}
  name: ${USERNAME}-context
current-context: ${USERNAME}-context
END
```
The "user1-management-kubeconfig.conf" generated at the end ensures that the user, user1, can only access CRDs of the
workload cluster in his/her own created namespace ${NAMESPACE} (user1-ns) of the management cluster.

Notes:
* Organization administrator relays the "user1-management-kubeconfig.conf" to the tenant user "user1"
* Once the above operation is complete, there is no need of further interaction between organization administrator and the tenant user.
* The mechanism used above to generate a Kubernetes Config has a default lifetime of one year.
* We recommend strongly that the USERNAME match that of VCD tenant username.

## Resize, Upgrade and Delete Management cluster
These workflows need to be run from the bootstrap cluster (the parent of the management cluster).

In the `kubectl` commands specified in the below workflows, update the `namespace` parameter to the value `default`
and `kubeconfig` to the value of bootstrap cluster's admin Kubeconfig
* [Resize workflow](WORKLOAD_CLUSTER.md#resize_workload_cluster)
* [Upgrade workflow](WORKLOAD_CLUSTER.md#upgrade_workload_cluster)
* [Delete workflow](WORKLOAD_CLUSTER.md#delete_workload_cluster)
