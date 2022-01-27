# Management Cluster Setup

Personas
* Amy - Management Cluster Author ~ tenant admin
* John - Workload Cluster Author ~ tenant user 

See the [rights required](VCD_SETUP.md#user_role) here

## Create Management cluster

All the below steps are expected to be performed by the user Amy (Management Cluster Author).

### Install and/or configure a bootstrap Kubernetes cluster

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

See here to enable [VCD tenant user management on the management cluster](WORKLOAD_CLUSTER.md). This enables tenant users to deploy the
workload clusters in their own private namespaces, while adhering to VCD's  tenant/user quota management.

<a name="tenant_user_management"></a>
## Prepare Management cluster to enable VCD tenant users' access
Personas:
* Amy - Management Cluster Author ([Tenant Admin](VCD_SETUP.md#user_role))
* John - Workload Cluster Author ([Tenant user](VCD_SETUP.md#user_role))

Amy creates a new and unique Kubernetes namespace for John and creates Kubernetes configuration with access to only the
required CRDs in only this namespace. This is a one-time operation per VCD tenant user.

Below are the commands to be run. The USERNAME parameter should be changed as per your requirements.

```sh
USERNAME="john"
 
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
 
cat > John-management-kubeconfig.conf <<END
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
The "John-management-kubeconfig.conf" generated at the end ensures that the user 'John' can only access CRDs for
CAPVCD Workload Cluster Creation in the Management Cluster in a newly-created namespace ${NAMESPACE} (John-ns).

Notes:
* Once the above operation is complete, there is no need of further interaction between Amy and John.
* The mechanism used above to generate a Kubernetes Config has a default lifetime of one year.
* We recommend strongly that the USERNAME match that of VCD tenant username.
