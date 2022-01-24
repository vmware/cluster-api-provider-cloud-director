# VCD tenant user management on the Management cluster

* Amy - Management Cluster Author (Org Admin)
* John - Workload Cluster Author (Tenant user)

Refer to rights required for the above roles [here](VCD_SETUP.md#user_role)

1. Amy creates a management cluster and she has access to Admin Kubeconfig of the management cluster.
2. John wants to create a workload cluster; asks Amy for the access to management cluster.
3. Amy creates a [new Kubernetes service account](#create_K8s_svc_account) and hands over the Kubeconfig file with limited privileges to 
   the John.
4. John uses the Kubeconfig to access the management cluster and [creates his first workload cluster](#create_workload_cluster).

<a name="create_K8s_svc_account"></a>
## Create Kubernetes Service Account on the Management cluster

Amy created a new and unique Kubernetes namespace for John and creates Kubernetes configuration with access to only the 
required CRDs in only this namespace. This is a one-time operation per VCD tenant user.

Below are the commands to be run. The USERNAME parameter should be changed as per your requirements.

```sh
USERNAME="John"
 
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

<a name="create_workload_cluster"></a>
## Create workload cluster on the Management cluster 

1. John can access the management cluster using `kubectl --namespace ${NAMESPACE} --kubeconfig=John-management-kubeconfig.conf get machines`
2. John edits the [sample CAPI.yaml](https://github.com/vmware/cluster-api-provider-cloud-director/blob/main/examples/capi-quickstart.yaml) to ensure every object gets created in the namespace allocated to him. The user 
   credentials/refresh token should also be embedded into the CAPI yaml file
    1. The user section in the Cluster object that contains the secrets used to log into VCD. For creation of the VMs, the LoadBalancer etc, the credentials passed will be used.
    2. In production cluster scenarios we recommend strongly that the refreshToken parameter be used, The username and password fields should be omitted or set as empty strings.
```yaml
apiVersion: cluster.x-k8s.io/v1alpha4
kind: Cluster
metadata:
name: capi-john
namespace: john-ns
spec:
clusterNetwork:
pods:
cidrBlocks:
- 100.96.0.0/11
serviceDomain: k8s.test
services:
cidrBlocks:
- 100.64.0.0/13
controlPlaneRef:
apiVersion: controlplane.cluster.x-k8s.io/v1alpha4
kind: KubeadmControlPlane
name: capi-control-plane-john
namespace: john-ns
infrastructureRef:
apiVersion: infrastructure.cluster.x-k8s.io/v1alpha4
kind: VCDCluster
name: capi-john
namespace: john-ns
---
apiVersion: infrastructure.cluster.x-k8s.io/v1alpha4
kind: VCDCluster
metadata:
name: capi-john
namespace: john-ns
context:
username: john
password: password
refreshToken: ""
---
```
3. John creates the workload cluster 
    1. `kubectl --namespace=${NAMESPACE} --kubeconfig=John-management-kubeconfig.conf apply -f capi.yaml`
    2. Wait for control plane to be initialized `kubectl --namespace=${NAMESPACE} --kubeconfig=John-management-kubeconfig.conf describe cluster capi-john`
4. John retrieves the Admin Kubeconfig of the workload cluster 
    1. `CLUSTERNAME="capi-john"`
    2. `kubectl -n ${NAMESPACE} --kubeconfig=user-management-kubeconfig.conf get secret ${CLUSTERNAME}-kubeconfig -o json | jq ".data.value" | tr -d '"' | base64 -d > ${CLUSTERNAME}-workload-kubeconfig.conf`
    3. `kubectl --kubeconfig=${CLUSTERNAME}-workload-kubeconfig.conf get pods -A -owide`
5. John can do other operations like resize, upgrade on the workload cluster by editing the capi.yaml. 
   For delete, it is recommended to delete the cluster object directly - `kubectl --namespace=${NAMESPACE} --kubeconfig=user-management-kubeconfig.conf delete cluster ${CLUSTERNAME}`

