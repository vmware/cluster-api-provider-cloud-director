# Workload cluster operations

Assuming the [management cluster is already setup](MANAGEMENT_CLUSTER.md#tenant_user_management) for use by the tenant user 'User1',
the User1 will use his/her kubeconfig of the management cluster `user1-management-kubeconfig.conf` to create and
operate all his/her workload clusters in his/her namespace

<a name="create_workload_cluster"></a>
## Create a workload cluster
1. User1 can now access the management cluster via kubeconfig specifically generated for him/her
    1. `kubectl --namespace ${NAMESPACE} --kubeconfig=user1-management-kubeconfig.conf get machines`
2. User1 generates the cluster configuration `capi.yaml`. Refer to [clusterctl generate cmd](CLUSTERCTL.md#generate_cluster_manifest) on how to generate the file.
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
   of desired `KubeadmControlPlane` objects. The value must be an odd number.
2. To resize the worker nodes, update the property `MachineDeployment.spec.replicas` of desired `MachineDeployment` objects to the desired worker count.

<a name="upgrade_workload_cluster"></a>
## Upgrade a workload cluster
In order to upgrade a workload cluster, 
* Cloud Provider must upload the new Kubernetes version of Ubuntu 20.04 TKG OVA into VCD using VCD UI.
* The upgrade of a Kubernetes cluster can only be done to the next incremental version, say from K8s 1.20 to K8s 1.21.

In the CAPI yaml, update the below properties and run `kubectl --namespace=${NAMESPACE} --kubeconfig=user1-management-kubeconfig.conf apply -f capi.yaml`
 on the management cluster (or) use one of the existing [clusterctl template flavors](CLUSTERCTL.md#template_flavors) with 
pre-populated Kubernetes and the associated etcd and coredns versions.

1. Upgrade Control plane version
    1. Update `VCDMachineTemplate` object(s) with the new version of TKG template details
        * Update `VCDMachineTemplate.spec.template.spec.template` and other properties under `VCDMachineTemplate.spec.template.spec` if needed.
    2. Update `KubeadmControlPlane` object(s) with the newer versions of Kubernetes components. 
        * Update `KubeadmControlPlane.spec.version`, `KubeadmControlPlane.spec.kubeadmConfigSpec.dns`, `KubeadmControlPlane.spec.kubeadmConfigSpec.etcd`, `KubeadmControlPlane.spec.kubeadmConfigSpec.imageRepository`.
2. Upgrade Worker node version
    1. Update `VCDMachineTemplate` objects with the new version of TKG template details.
        * Update `VCDMachineTemplate.spec.template.spec.template` and other properties under `VCDMachineTemplate.spec.template.spec` if needed.
    2. Update `MachineDeployment` objects with the newer kubernetes version in the property `MachineDeployment.spec.version`. 

All the versions must come from new Kubernetes version of the TKG OVA specified in `VCDMachineTemplate` object(s).
See the [script to get Kubernetes, etcd, coredns versions from TKG OVA](#tkgm_bom).

<a name="delete_workload_cluster"></a>
## Delete workload cluster
To delete the cluster, run this command on the management cluster
* `kubectl --namespace=${NAMESPACE} --kubeconfig=user-management-kubeconfig.conf delete cluster ${CLUSTERNAME}`
    
It is not recommended using this command
* `kubectl --namespace=${NAMESPACE} --kubeconfig=user-management-kubeconfig.conf delete -f capi-quickstart.yaml`

<a name="tkgm_bom"></a>
### Script to get Kubernetes, etcd, coredns versions from TKG OVA
Ensure docker and yq are pre-installed on your local machine.
```shell

# Extract etcd and coredns info for capi.yaml
# Change the RAW version to match something similar from your TKG template
export K8S_VERSION_RAW=v1.20.8+vmware.1-tkg.1
 
export K8S_VERSION=$(echo ${K8S_VERSION_RAW} | tr -s "+" "_")
 
# We need to loop through the last value after `tkg.` because of some TKR unexpected design
no_tkg_found=false
last=$(echo ${K8S_VERSION//*.})  # get last value after `tkg.`
until docker pull projects.registry.vmware.com/tkg/tkr-bom:${K8S_VERSION}
do
  last=$((last+1))
  export K8S_VERSION=$(echo ${K8S_VERSION} | sed "s/.$/"$last"/")
  if [ $last -gt 10 ]; then
    no_tkg_found=true
    break
  fi
  last=$(echo ${K8S_VERSION//*.})  # get last value after `tkg.`
done

if $no_tkg_found; then
	echo "'no valid tkg.X version found"
	exit 1
fi

export K8S_VERSION_RAW=$(echo ${K8S_VERSION_RAW} | sed "s/.$/"$last"/")
docker save projects.registry.vmware.com/tkg/tkr-bom:${K8S_VERSION} | tar Oxf - --strip-components 1 */layer.tar | tar xf -
 
ETCD_VERSION=$(yq e ".components.etcd[0].version" tkr-bom-${K8S_VERSION_RAW}.yaml | tr -s "+" "_")
ETCD_IMAGE_PATH="projects.registry.vmware.com/tkg/$(yq e ".components.etcd[0].images.etcd.imagePath" tkr-bom-${K8S_VERSION_RAW}.yaml)"
ETCD_IMAGE_TAG=$(yq e ".components.etcd[0].images.etcd.tag" tkr-bom-${K8S_VERSION_RAW}.yaml)
 
COREDNS_VERSION=$(yq e ".components.coredns[0].version" tkr-bom-${K8S_VERSION_RAW}.yaml | tr -s "+" "_")
COREDNS_IMAGE_PATH="projects.registry.vmware.com/tkg/$(yq e ".components.coredns[0].images.coredns.imagePath" tkr-bom-${K8S_VERSION_RAW}.yaml)"
COREDNS_IMAGE_TAG=$(yq e ".components.coredns[0].images.coredns.tag" tkr-bom-${K8S_VERSION_RAW}.yaml)
```


