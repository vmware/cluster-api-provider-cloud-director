# Clusterctl 

<a name="clusterctl_set_up"></a>
## Set up
Install [Clusterctl v1.1.3](https://cluster-api.sigs.k8s.io/user/quick-start.html#install-clusterctl)
Currently, the below manual steps are required to enable clusterctl for CAPVCD 1.0.0.

1. Create a folder structure `~/infrastructure-vcd/v1.0.0/`.
2. Copy the contents from [templates directory](https://github.com/vmware/cluster-api-provider-cloud-director/tree/1.0.0/templates) to `~/infrastructure-vcd/v1.0.0/`
3. Copy [metadata.yaml](https://github.com/vmware/cluster-api-provider-cloud-director/blob/main/metadata.yaml) to `~/infrastructure-vcd/v1.0.0/`
4. Copy the `~/infrastructure-vcd/v1.0.0/clusterctl.yaml` to `~/.cluster-api/clusterctl.yaml`
5. Update the `providers.url` in `~/.cluster-api/clusterctl.yaml` to `~/infrastructure-vcd/v1.0.0/infrastructure-components.yaml`
```yaml
providers:
  - name: "vcd"
    url: "~/infrastructure-vcd/v1.0.0/infrastructure-components.yaml"
    type: "InfrastructureProvider"
```

<a name="init_management_cluster"></a>
## Initialize Management cluster
1. Run the below command to initialize the management cluster with the Cluster API and the associated provider for VMware Cloud Director
`clusterctl init --core cluster-api:v1.1.3 -b kubeadm:v1.1.3 -c kubeadm:v1.1.3 -i vcd:v1.0.0`
2. Apply [CRS definitions](CRS.md#apply_crs) to ensure CNI, CPI and CSI are automatically installed on the workload clusters.   

<a name="generate_cluster_manifest"></a>
## Generate cluster manifests for workload cluster

1. Fill out the values for the environment variables in `~/.cluster-api/clusterctl.yaml`. 
   - One of the variables is RefreshToken. Refer to [How to create refreshToken (or) API token in Cloud Director](https://docs.vmware.com/en/VMware-Cloud-Director/10.3/VMware-Cloud-Director-Tenant-Portal-Guide/GUID-A1B3B2FA-7B2C-4EE1-9D1B-188BE703EEDE.html).
   - Refer to the [script to get Kubernetes, etcd, coredns versions from TKG OVA](WORKLOAD_CLUSTER.md#tkgm_bom) to fill in few variables. Note that you may skip filling
     in few of these variables if you decide to use the existing [clusterctl template flavors](#template_flavors).
2. Generate the CAPI manifest file.
   - `clusterctl generate cluster <clusterName> -f v1.21.8-crs > <clusterName>.yaml`.
3. Create the workload cluster by applying it on the (parent) management cluster.
   - `kubectl apply -f <clusterName>.yaml`
4. [Apply CRS labels](CRS.md#apply_crs_labels) and [enable the resultant add-ons like CPI, CSI to access VCD resources](CRS.md#enable_add_ons)    


<a name="template_flavors"></a>   
## Template flavors

- All of the templates to generate the cluster manifests are located at `templates` directory under the root of the github repository.
- All the flavors listed support only v1beta1 API versions of CAPVCD and Core CAPI.  
- Currently, we have v1.20.8, v1.21.8, v1.22.9 as template flavors, and they each have their own etcd/dns versions pre-populated. 
Please ensure your `~/.cluster-api/clusterctl.yaml` has `VCD_TEMPLATE_NAME` matching the correct versions of Kubernetes. 
For example, if `VCD_TEMPLATE_NAME=Ubuntu 20.04 and Kubernetes v1.21.8+vmware.1` then use `v1.21.8-crs` flavor.
- It is strongly recommended to use `v1.y.z-crs` flavors to ensure CNI, CPI and CSI are automatically installed on the 
  workload clusters. CNI and CPI are required add-ons for the cluster creation to be successful.
