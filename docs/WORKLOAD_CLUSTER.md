# Workload cluster operations

Personas:
* Amy - Management Cluster Author (Org Administrator)
* John - Workload Cluster Author (Tenant user)

Refer to the rights required for the above roles [here](VCD_SETUP.md#user_role)

<a name="create_workload_cluster"></a>
## Create workload cluster on the Management cluster 

In order for John to create workload cluster, Amy should have  already enabled the user access for 
John on the management cluster. See [management cluster setup](QUICKSTART.md#management_cluster_setup) and 
[tenant_user_management](MANAGEMENT_CLUSTER.md#tenant_user_management) for more details on the Amy's steps).

1. John (Workload Cluster Author (Tenant user)) can now access the management cluster using `kubectl --namespace ${NAMESPACE} --kubeconfig=John-management-kubeconfig.conf get machines`
2. John edits the [sample CAPI.yaml](https://github.com/vmware/cluster-api-provider-cloud-director/blob/main/examples/capi-quickstart.yaml) to ensure every object gets created in the namespace allocated to him. The user 
   credentials/refresh token should also be embedded into the CAPI yaml file. Refer [how to create refreshToken](#create_refresh_token)
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

## Resize workload cluster

## Upgrade workload cluster

## Delete workload cluster

<a name="create_refresh_token"></a>
## How to create refreshToken?
Step 1: Register a client:
```sh
curl --location --request POST 'https://<vcd-fqdn>>/oauth/tenant/<org-name>/register' \
--header 'Accept: application/json;version=36.0' \
--header 'Authorization: Bearer eyJhbGciOiJSUzI1NiJ9.eyJzdWIiOiJvcmdhZG1pbiIsImlzcyI6ImZlZTYxOTI3LTU1NTUtNDY4Zi1iMTZiLWU2NDgxZDcyM2IwMUAyMDQ0ZmUwNC1jNTg5LTRjMmItODUxNC1hNTlkMWFhOTE1NGUiLCJleHAiOjE2NDQwMDg5ODMsInZlcnNpb24iOiJ2Y2xvdWRfMS4wIiwianRpIjoiYzNkODZhNDU5ODhlNDM1NDlmOTA3YzFhN2MxYTAxNDgifQ.aRLO7W_lrhQyWGDuwdY0sELCNn7bPXn2Aryz-mUhaSWrZuRHDayTL1vN3Y70Q3XnV8ayP_uBoa-7R-9qTj5hNHhydyvRCAxeXoAFz-3BEYo0hDAZ0S6OAy5iMcYQNmmFIdjIUwsrb3nFvrA2e8tqQI4X2UdnHPe-ZdCcnYsq7QCeiD4_vUfH3rJVAutuuSxWD6Uk_JukncxwgDpHi9HSqMTqZ6rOUlZiaOfgsILTm8lVZvzQhlMmrcyrc3ysiKoDtQjc2BJwaJ4Qxgb22_FjQwCzc0ixENRBpiY4Iiqyo44nKvaHutkRA9WNmJyR2HFLFuSqE8oi-WkML0gneEJz_A' \
--header 'Content-Type: application/json' \
--data-raw '{
"client_name": "management-cluster"
}'
```
Client ID will be obtained as part of the response

Step 2: Create a refresh token
Use client ID from the response from Step 1
```sh
curl --location --request POST 'https://<vcd-fqdn>/oauth/tenant/<org-name>/token' \
--header 'Accept: application/json;version=36.0' \
--header 'Content-Type: application/x-www-form-urlencoded' \
--data-urlencode 'client_id=1447f90a-2ca7-4b9c-ac9d-a42e529a870b' \
--data-urlencode 'grant_type=urn:ietf:params:oauth:grant-type:jwt-bearer' \
--data-urlencode 'assertion=eyJhbGciOiJSUzI1NiJ9.eyJzdWIiOiJvcmdhZG1pbiIsImlzcyI6ImZlZTYxOTI3LTU1NTUtNDY4Zi1iMTZiLWU2NDgxZDcyM2IwMUAyMDQ0ZmUwNC1jNTg5LTRjMmItODUxNC1hNTlkMWFhOTE1NGUiLCJleHAiOjE2NDQwMDg5ODMsInZlcnNpb24iOiJ2Y2xvdWRfMS4wIiwianRpIjoiYzNkODZhNDU5ODhlNDM1NDlmOTA3YzFhN2MxYTAxNDgifQ.aRLO7W_lrhQyWGDuwdY0sELCNn7bPXn2Aryz-mUhaSWrZuRHDayTL1vN3Y70Q3XnV8ayP_uBoa-7R-9qTj5hNHhydyvRCAxeXoAFz-3BEYo0hDAZ0S6OAy5iMcYQNmmFIdjIUwsrb3nFvrA2e8tqQI4X2UdnHPe-ZdCcnYsq7QCeiD4_vUfH3rJVAutuuSxWD6Uk_JukncxwgDpHi9HSqMTqZ6rOUlZiaOfgsILTm8lVZvzQhlMmrcyrc3ysiKoDtQjc2BJwaJ4Qxgb22_FjQwCzc0ixENRBpiY4Iiqyo44nKvaHutkRA9WNmJyR2HFLFuSqE8oi-WkML0gneEJz_A'Use Access token as value for assertion
```
Refresh token will be present as part of the response


