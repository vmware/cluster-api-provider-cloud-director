# Workload cluster operations

Assuming a [management cluster is already setup](MANAGEMENT_CLUSTER.md#tenant_user_management) for use by the tenant user 'User1',
the User1 will use his/her kubeconfig of the management cluster `user1-management-kubeconfig.conf` to create and
operate all his/her workload clusters in his/her namespace

<a name="create_workload_cluster"></a>
## Create a workload cluster
1. User1 can now access the management cluster via kubeconfig specifically generated for him/her
    1. `kubectl --namespace ${NAMESPACE} --kubeconfig=user1-management-kubeconfig.conf get machines`
2. User1 generates the cluster configuration `capi.yaml`. Refer to [CAPI Yaml configuration](#capi_yaml) on how to generate the file.
3. User1 creates the workload cluster 
    1. `kubectl --namespace=${NAMESPACE} --kubeconfig=user1-management-kubeconfig.conf apply -f capi.yaml`. The output is similar to the below
        * ```sh
           cluster.cluster.x-k8s.io/capi-quickstart created
           vcdcluster.infrastructure.cluster.x-k8s.io/capi-quickstart created
           vcdmachinetemplate.infrastructure.cluster.x-k8s.io/capi-quickstart-control-plane created
           kubeadmcontrolplane.controlplane.cluster.x-k8s.io/capi-quickstart-control-plane created
           vcdmachinetemplate.infrastructure.cluster.x-k8s.io/capi-quickstart-md0 created
           kubeadmconfigtemplate.bootstrap.cluster.x-k8s.io/capi-quickstart-md0 created
           machinedeployment.cluster.x-k8s.io/capi-quickstart-md0 created
           ```
    2. Waits for control plane to be initialized `kubectl --namespace=${NAMESPACE} --kubeconfig=user1-management-kubeconfig.conf describe cluster user1-cluster`
4. User1 retrieves the Admin Kubeconfig of the workload cluster 
    1. `CLUSTERNAME="user1-cluster"`
    2. `kubectl -n ${NAMESPACE} --kubeconfig=user1-management-kubeconfig.conf get secret ${CLUSTERNAME}-kubeconfig -o json | jq ".data.value" | tr -d '"' | base64 -d > ${CLUSTERNAME}-workload-kubeconfig.conf`
5. User1 accesses his/her workload cluster
    1. `kubectl --kubeconfig=${CLUSTERNAME}-workload-kubeconfig.conf get pods -A -owide`

<a name="resize_workload_cluster"></a> 
## Resize a workload cluster
In the CAPI yaml, update the below properties and run `kubectl --namespace=${NAMESPACE} --kubeconfig=user1-management-kubeconfig.conf apply -f capi.yaml` 
on the management cluster.
1. To resize the control plane nodes of the workload cluster, update the property `KubeadmControlPlane.spec.replicas` 
   of desired `KubeadmControlPlane` objects to resize the control plane count. The value must be an odd number.
2. To resize the worker nodes, update the property `MachineDeployment.spec.replicas` of desired `MachineDeployment` objects to resize the worker count.

<a name="upgrade_workload_cluster"></a>
## Upgrade a workload cluster
In order to upgrade a workload cluster, Cloud Provider must upload the new Kubernetes version of Ubuntu 20.04 TKG OVA into VCD using VCD UI.
The upgrade of a Kubernetes cluster can only be done to the next incremental version, say from K8s 1.20 to K8s 1.21.

In the CAPI yaml, update the below properties and run `kubectl --namespace=${NAMESPACE} --kubeconfig=user1-management-kubeconfig.conf apply -f capi.yaml`
 on the management cluster.
1. Upgrade Control plane version
    1. Update `VCDMachineTemplate` object(s) with the new version of TKGm template details
        * Update `VCDMachineTemplate.spec.template.spec.template` and other properties under `VCDMachineTemplate.spec.template.spec` if needed.
    2. Update `KubeadmControlPlane` object(s) with the newer versions of Kubernetes components. 
        * Update `KubeadmControlPlane.spec.version`, `KubeadmControlPlane.spec.kubeadmConfigSpec.dns`, `KubeadmControlPlane.spec.kubeadmConfigSpec.etcd`, `KubeadmControlPlane.spec.kubeadmConfigSpec.imageRepository`.
2. Upgrade Worker node version
    1. Update `VCDMachineTemplate` objects with the new version of TKGm template details.
        * * Update `VCDMachineTemplate.spec.template.spec.template` and other properties under `VCDMachineTemplate.spec.template.spec` if needed.
    2. Update `MachineDeployment` objects with the newer version of the property `MachineDeployment.spec.version`. 

All the versions specified above must match the Kubernetes version of the TKG OVA specified in `VCDMachineTemplate` object(s).
See here on [how to retrieve the versions from respective TKGm bill of materials](#tkgm_bom).

<a name="delete_workload_cluster"></a>
## Delete workload cluster
To delete the cluster, run this command on the management cluster
* `kubectl --namespace=${NAMESPACE} --kubeconfig=user-management-kubeconfig.conf delete cluster ${CLUSTERNAME}`
    
It is not recommended using this command
* `kubectl --namespace=${NAMESPACE} --kubeconfig=user-management-kubeconfig.conf delete -f capi-quickstart.yaml`

<a name="capi_yaml"></a>
## CAPI YAML configuration

`clusterctl generate` command doesn't support the generation of CAPI yaml for Cloud Director; Follow the guidelines 
provided below configure the CAPI Yaml file

1. Retrieve the [sample YAML](https://github.com/vmware/cluster-api-provider-cloud-director/blob/main/examples/capi-quickstart.yaml) from github repo.
2. Update the name of the cluster 
    * Retrieve the value of `Cluster.metadata.name` and replace-all the value with the new cluster name.
3. Update the `namespace` property of all the objects with the namespace assigned to you (tenant user) on the management
   cluster by the tenant administrator.
3. Update the `VCDCluster.spec` parameters using the informational comments in the sample yaml. It is strongly recommended
   that the `refreshToken` parameter be used, the username and password fields should be omitted or set as empty strings. 
   Refer to [how to create refreshToken](https://docs.vmware.com/en/VMware-Cloud-Director/10.3/VMware-Cloud-Director-Tenant-Portal-Guide/GUID-A1B3B2FA-7B2C-4EE1-9D1B-188BE703EEDE.html).
4. Update `VCDMachineTemplate` objects of both Control plane and workers with the TKG OVA details.
5. Update `KubeadmControlPlane` object(s) with the below details of Kubernetes components. The values must match the Kubernetes 
   version of the corresponding template specified in `VCDMachineTemplate` object(s). See here on [how to retrieve the versions from respective TKGm bill of materials](#tkgm_bom).
    1. Update `KubeadmControlPlane.spec.version`, `KubeadmControlPlane.spec.kubeadmConfigSpec.dns`, 
       `KubeadmControlPlane.spec.kubeadmConfigSpec.etcd`, `KubeadmControlPlane.spec.kubeadmConfigSpec.imageRepository`.
       The above sample file has the values corresponding to v1.20.8 Kubernetes version of TKGm template.
    2. Update `KubeadmControlPlane.spec.replicas` to specify the control plane count. The value must be odd number.
    3. Update `KubeadmControlPlane.spec.kubeadmConfigSpec.users` with the ssh keys to access the control plane node VMs.
6. Update your ssh keys at `KubeadmConfigTemplate.spec.template.spec.users` to access the worker node VMs.
7. Update `MachineDeployment.spec` with the below
    1. To specify the worker count, update the property `MachineDeployment.spec.replicas`.
    2. To specify the Kubernetes version, update the property `MachineDeployment.spec.version` to specify the Kubernetes version.
       The values must match the Kubernetes version of the corresponding template specified in `VCDMachineTemplate` object(s).
       See here on [how to retrieve the versions from respective TKGm bill of materials](#tkgm_bom).
       
Sample sub-section of the YAML
```yaml
apiVersion: cluster.x-k8s.io/v1alpha4
kind: Cluster
metadata:
name: user1-cluster
namespace: user1-ns
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
name: user1-cluster-control-plane
namespace: user1-ns
infrastructureRef:
apiVersion: infrastructure.cluster.x-k8s.io/v1alpha4
kind: VCDCluster
name: user1-cluster
namespace: user1-ns
---
apiVersion: infrastructure.cluster.x-k8s.io/v1alpha4
kind: VCDCluster
metadata:
name: user1-cluster
namespace: user1-ns
context:
username: user1
password: password
refreshToken: ""
---
```

<a name="tkgm_bom"></a>
### Script to extract Kubernetes component versions from TKGm Bill of materials
Ensure docker and yq are pre-installed on your local machine.
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


