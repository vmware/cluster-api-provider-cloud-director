# Multitenancy on management cluster

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
KUBE_APISERVER_ADDRESS=https://127.0.0.1:64265 #Ensure you update this with your own Kubernetes API server address

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