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

1. Fill out the values for the environment variables in `clusterctl.yaml`
2. 
Fill out clusterctl.yaml and update it in ~/.cluster-api/clusterctl.yaml (refer section Prepare CAPVCD Manifests for more details)
Fill out the values for the environment variables in clusterctl.yaml file
After `clusterctl.yaml` has been filled, and management cluster is setup you may now generate the capi yaml.
Run `clusterctl generate cluster clusterName -f v1.21.8-crs > clusterName.yaml`.
Note we only have v1.20.8, v1.21.8, v1.22.9 as the supported flavors and they each have their own etcd/dns versions. Please ensure your `clusterctl.yaml` VCD_TEMPLATE_NAME are matching the correct versions of Kubernetes. For example, if my `VCD_TEMPLATE_NAME=Ubuntu 20.04 and Kubernetes v1.21.8+vmware.1` then I would want to use `v1.21.8-crs` flavor.
After generating the yaml file, you can directly apply it using
kubectl apply -f clusterName.yaml