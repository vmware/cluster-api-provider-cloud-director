# Clusterctl 

## Set up
Install [Clusterctl v1.1.3](https://cluster-api.sigs.k8s.io/user/quick-start.html#install-clusterctl)
Currently, the below manual steps are required to enable clusterctl for CAPVCD.

1. Create a folder structure `~/infrastructure-vcd/v1.0.0/`.
2. Copy the contents from [templates directory](https://github.com/vmware/cluster-api-provider-cloud-director/tree/main/templates) to `~/infrastructure-vcd/v1.0.0/`
3. Copy [metadata.yaml](https://github.com/vmware/cluster-api-provider-cloud-director/blob/main/metadata.yaml) to `~/infrastructure-vcd/v1.0.0/`
4. Copy the `~/infrastructure-vcd/v1.0.0/clusterctl.yaml` to `~/.cluster-api/clusterctl.yaml`
5. Update the `providers.url` in `~/.cluster-api/clusterctl.yaml` to `~/infrastructure-vcd/v1.0.0/infrastructure-components.yaml`
```yaml
providers:
  - name: "vcd"
    url: "~/infrastructure-vcd/v1.0.0/infrastructure-components.yaml"
    type: "InfrastructureProvider"
```

## Initialize Management cluster
Run the below command to initialize the management cluster with the Cluster API and the associated provider for VMware Cloud Director
`clusterctl init --core cluster-api:v1.1.3 -b kubeadm:v1.1.3 -c kubeadm:v1.1.3 -i vcd:v1.0.0`

## Generate cluster manifests for workload cluster

1. Fill out the values for the environment variables in `~/.cluster-api/clusterctl.yaml`. Note that you may skip filling in few variables if you decide to choose template flavors.
2. Generate the CAPI manifest file.
   - `clusterctl generate cluster <clusterName> -f v1.21.8-crs > <clusterName>.yaml`.
   - Note we only have v1.20.8, v1.21.8, v1.22.9 as template flavors, and they each have their own etcd/dns versions. Please ensure your `~/.cluster-api/clusterctl.yaml` has VCD_TEMPLATE_NAME matching the correct versions of Kubernetes. For example, if `VCD_TEMPLATE_NAME=Ubuntu 20.04 and Kubernetes v1.21.8+vmware.1` then use `v1.21.8-crs` flavor.
3. Create the workload cluster
   - `kubectl apply -f <clusterName>.yaml`