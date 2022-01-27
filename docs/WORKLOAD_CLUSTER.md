# Workload cluster operations

Personas:
* Amy - Management Cluster Author ~ tenant admin
* John - Workload Cluster Author ~ tenant user

See the [rights required](VCD_SETUP.md#user_role) here

<a name="create_workload_cluster"></a>
## Create workload cluster on the Management cluster 

In order for John to create workload cluster, Amy should have already enabled the user access for 
John on the management cluster. See [management cluster setup](QUICKSTART.md#management_cluster_setup) and 
[tenant_user_management](MANAGEMENT_CLUSTER.md#tenant_user_management) for more details on the Amy's steps).

1. John can now access the management cluster via kubeconfig specifically generated for him
    1. `kubectl --namespace ${NAMESPACE} --kubeconfig=John-management-kubeconfig.conf get machines`
2. John generates the cluster configuration. Refer to [CAPI Yaml configuration](#capi_yaml) on how to fill the details.
3. John creates the workload cluster 
    1. `kubectl --namespace=${NAMESPACE} --kubeconfig=John-management-kubeconfig.conf apply -f capi.yaml`. The output is similar to the below
    2. ```sh
       cluster.cluster.x-k8s.io/capi-quickstart created
       vcdcluster.infrastructure.cluster.x-k8s.io/capi-quickstart created
       vcdmachinetemplate.infrastructure.cluster.x-k8s.io/capi-quickstart-control-plane created
       kubeadmcontrolplane.controlplane.cluster.x-k8s.io/capi-quickstart-control-plane created
       vcdmachinetemplate.infrastructure.cluster.x-k8s.io/capi-quickstart-md0 created
       kubeadmconfigtemplate.bootstrap.cluster.x-k8s.io/capi-quickstart-md0 created
       machinedeployment.cluster.x-k8s.io/capi-quickstart-md0 created
       ```
    3. Wait for control plane to be initialized `kubectl --namespace=${NAMESPACE} --kubeconfig=John-management-kubeconfig.conf describe cluster capi-john`
4. John retrieves the Admin Kubeconfig of the workload cluster 
    1. `CLUSTERNAME="capi-john"`
    2. `kubectl -n ${NAMESPACE} --kubeconfig=user-management-kubeconfig.conf get secret ${CLUSTERNAME}-kubeconfig -o json | jq ".data.value" | tr -d '"' | base64 -d > ${CLUSTERNAME}-workload-kubeconfig.conf`
    3. `kubectl --kubeconfig=${CLUSTERNAME}-workload-kubeconfig.conf get pods -A -owide`
   
## Resize workload cluster
Update the below properties in the CAPI Yaml and re-apply it.
1. Update the property `KubeadmControlPlane.spec.replicas` of desired KubeadmControlPlane objects to resize the control 
plane count. The value must be odd number.
2. Update the property `MachineDeployment.spec.replicas` of desired MachineDeployment objects to resize the worker count.

## Upgrade workload cluster
Update the below properties in the CAPI Yaml and re-apply it.
1. Upgrade Control plane version
    1. Update `VCDMachineTemplate` object(s) with the new version of TKGm template details.
    2. Update `KubeadmControlPlane` object(s) with the newer versions of Kubernetes components. 
        * Update `KubeadmControlPlane.spec.version`, `KubeadmControlPlane.spec.kubeadmConfigSpec.dns`, `KubeadmControlPlane.spec.kubeadmConfigSpec.etcd`, `KubeadmControlPlane.spec.kubeadmConfigSpec.imageRepository`.
2. Upgrade Worker node version
    1. Update `VCDMachineTemplate` objects with the new version of TKGm template details.
    2. Update `MachineDeployment` objects with the newer version of the property `MachineDeployment.spec.version`. 

Note that all the values must match the Kubernetes version of the corresponding template specified in (1). See here on [how to retrieve the versions from respective TKGm bill of materials](#tkgm_bom).
<Yet to be filled on manual configmap changed for etcd and coredns>

## Delete workload cluster
It is recommended to delete the cluster object directly - `kubectl --namespace=${NAMESPACE} --kubeconfig=user-management-kubeconfig.conf delete cluster ${CLUSTERNAME}` 
rather than `kubectl --namespace=${NAMESPACE} --kubeconfig=user-management-kubeconfig.conf delete -f capi-quickstart.yaml`

<a name="capi_yaml"></a>
## CAPI YAML configuration

`clusterctl generate` command doesn't yet support CAPVCD 0.5 CAPI yaml generation; please use below guidelines to 
configure the YAML

1. Retrieve the [sample YAML](https://github.com/vmware/cluster-api-provider-cloud-director/blob/main/examples/capi-quickstart.yaml) from github repo.
2. Update the name of the cluster carefully in all the relevant objects. 
3. Update the `namespace` property of all the objects with the allocated namespace to you (tenant user) on the management cluster.
3. Update the `VCDCluster.spec` with the VCD site, names of the organization, virtual datacenter, virtual datacenter network and userContext.
   In production cluster scenarios we recommend strongly that the refreshToken parameter be used, the username and 
   password fields should be omitted or set as empty strings. Refer to [how to create refreshToken](#create_refresh_token).
4. Update `VCDMachineTemplate` objects of both Controlplane and workers with the TKGm template details.
5. Update `KubeadmControlPlane` object(s) with the below details of Kubernetes components. The values must match the Kubernetes 
   version of the corresponding template specified in (4). See here on [how to retrieve the versions from respective TKGm bill of materials](#tkgm_bom).
    1. Update `KubeadmControlPlane.spec.version`, `KubeadmControlPlane.spec.kubeadmConfigSpec.dns`, 
       `KubeadmControlPlane.spec.kubeadmConfigSpec.etcd`, `KubeadmControlPlane.spec.kubeadmConfigSpec.imageRepository`.
       The above sample file has the values corresponding to v1.20.8 Kubernetes version of TKGm template.
    2. Update `KubeadmControlPlane.spec.replicas` to specify the control plane count. The value must be odd number.
    3. Update `KubeadmControlPlane.spec.kubeadmConfigSpec.users` with the ssh keys to access the control plane node VMs.
6. Update `KubeadmConfigTemplate.spec.template.spec.users` with ssh keys to access the worker node VMs.
7. Update `MachineDeployment.spec` with the below
    1. `MachineDeployment.spec.replicas` to specify the worker count
    2. `MachineDeployment.spec.version` to specify the Kubernetes version matching the corresponding template used for 
       the `MachineDeployment`. See here on [how to retrieve the versions from respective TKGm bill of materials](#tkgm_bom).
       
Sample sub-section of the YAML
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

<a name="tkgm_bom"></a>
### Script to extract Kubernetes component versions from TKGm Bill of materials

```shell

# Extract etcd and coredns info for capi.yaml
# Change the RAW version to match something similar from your TKG template
export K8S_VERSION_RAW=v1.20.8+vmware.1-tkg.1
 
export K8S_VERSION=$(echo ${K8S_VERSION_RAW} | tr -s "+" "_")
 
# We need to loop through the last value after `tkg.` because of some TKR unexpected design
until docker pull projects.registry.vmware.com/tkg/tkr-bom:${K8S_VERSION}
do
  last=$(echo ${K8S_VERSION} | rev | cut -f1 -d".")
  last=$((last+1))
  export K8S_VERSION=$(echo ${K8S_VERSION} | sed "s/.$/"$last"/")
  if [ $last -gt 10 ]
  then
    break
  fi
done
 
export K8S_VERSION_RAW=$(echo ${K8S_VERSION_RAW} | sed "s/.$/"$last"/")
docker save projects.registry.vmware.com/tkg/tkr-bom:${K8S_VERSION} | tar Oxf - --strip-components 1 */layer.tar | tar xf -
 
ETCD_VERSION=$(yq e ".components.etcd[0].version" tkr-bom-${K8S_VERSION_RAW}.yaml | tr -s "+" "_")
ETCD_IMAGE_PATH="projects.registry.vmware.com/tkg/$(yq e ".components.etcd[0].images.etcd.imagePath" tkr-bom-${K8S_VERSION_RAW}.yaml)"
ETCD_IMAGE_TAG=$(yq e ".components.etcd[0].images.etcd.tag" tkr-bom-${K8S_VERSION_RAW}.yaml)
 
COREDNS_VERSION=$(yq e ".components.coredns[0].version" tkr-bom-${K8S_VERSION_RAW}.yaml | tr -s "+" "_")
COREDNS_IMAGE_PATH="projects.registry.vmware.com/tkg/$(yq e ".components.coredns[0].images.coredns.imagePath" tkr-bom-${K8S_VERSION_RAW}.yaml)"
COREDNS_IMAGE_TAG=$(yq e ".components.coredns[0].images.coredns.tag" tkr-bom-${K8S_VERSION_RAW}.yaml)
```
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



