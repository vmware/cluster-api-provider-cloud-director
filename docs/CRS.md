# Cluster Resource Sets

[ClusterResourceSets](https://cluster-api.sigs.k8s.io/tasks/experimental-features/cluster-resource-set.html) will be 
used to install CPI, CSI and CNI on the workload clusters.

Initialize the management cluster via [clusterctl](CLUSTERCTL.md)

<a name="apply_crs"></a>
## Apply CRS definitions on the (parent) management cluster
To install CRS components on the management cluster, in CAPVCD repo, copy the contents of  [templates](https://github.com/vmware/cluster-api-provider-cloud-director/tree/main/templates/crs) 
to the management cluster at `~/infrastructure-vcd/v1.0.0/`
1. Apply CNI (Antrea) CRS definitions:
   - Run `cd ~/infrastructure-vcd/v1.0.0/crs/cni`
   - kubectl create configmap antrea-crs-cm --from-file=antrea.yaml
   - kubectl apply -f antrea-crs.yaml
2. Apply CPI CRS definitions:
   - Run `cd ~/infrastructure-vcd/v1.0.0/crs/cpi`
   - kubectl create configmap cloud-director-crs-cm --from-file=cloud-director-ccm.yaml
   - kubectl apply -f cloud-director-crs.yaml
3. Apply CSI CRS definitions:
   - Run `cd ~/infrastructure-vcd/v1.0.0/crs/csi`
   - kubectl create configmap csi-controller-crs-cm --from-file=csi-controller-crs.yaml
   - kubectl create configmap csi-node-crs-cm --from-file=csi-node-crs.yaml
   - kubectl create configmap csi-driver-crs-cm --from-file=csi-driver.yaml
   - kubectl apply -f csi-crs.yaml

<a name="apply_crs_labels"></a>   
## Apply CRS labels on the Cluster manifests of the (children) workload cluster
1. Generate the [cluster manifest](CLUSTERCTL.md#generate_cluster_manifest) via clusterctl.
2. Ensure these labels are set under `metadata` section of `Cluster` definition in the [workload cluster manifest]. You can also use clusterctl [CRS template flavors](CLUSTERCTL.md#template_flavors) to
   [generate cluster manifests](CLUSTERCTL.md#generate_cluster_manifest) with the CRS labels preset
```yaml
labels:
    cni: antrea
    ccm: external
    csi: external
```
3. Apply the cluster manifest - `kubectl apply -f <clusterName>.yaml`

<a name="enable_add_ons"></a>
## Enable CPI and CSI on the workload cluster to access VMware Cloud Director resources

0. Say parent management cluster's kubeconfig is saved in `mangement_cluster.conf`
1. Retrieve the workload cluster's kubeconfig
   - `clusterctl get kubeconfig {cluster-name} > workload_cluster.conf`
2. Retrieve ClusterID and create a secret
   - `export CLUSTERID=$(kubectl --kubeconfig=management_cluster.conf get vcdclusters <workload cluster name> -o jsonpath="{.status.infraId}")`
   - `kubectl --kubeconfig=workload_cluster.conf -n kube-system create secret generic vcloud-clusterid-secret --from-literal=clusterid=${CLUSTERID}`
3. Retrieve RefreshToken of the user and create a secret with it on the workload cluster.
   - `export REFRESH_TOKEN=$(kubectl --kubeconfig=bootstrap_cluster.conf get secret <secret name> -o jsonpath="{.data.refreshToken}" | base64 -D)`
   - `kubectl --kubeconfig=workload_cluster.conf -n kube-system create secret generic vcloud-basic-auth --from-literal=refreshToken=${REFRESH_TOKEN} --from-literal=username="" --from-literal=password=""`
4. Create a config map for the CSI pod in the workload cluster.
   - Create a file with the following content, e.g vcloud-csi-config.yaml:
   ```yaml
       ---
       apiVersion: v1
       kind: ConfigMap
       metadata:
         name: vcloud-csi-configmap
         namespace: kube-system
       data:
         vcloud-csi-config.yaml: |+
           vcd:
             host: VCD_HOST
             org: ORG
             vdc: OVDC
             vAppName: VAPP
           clusterid: CLUSTER_ID
       immutable: true
       ---
       ```
   - Replace VCD_HOST, ORG, OVDC, VAPP, and CLUSTER_ID with the relevant values.
   - Create the config map in the workload cluster:
     `kubectl --kubeconfig=workload_cluster.conf apply -f vcloud-csi-config.yaml`
5. Create a config map for the CCM/CPI pod in the workload cluster. 
   - Create a file with the following content, e.g vcloud-ccm-config.yaml:
   ```yaml
       apiVersion: v1
       kind: ConfigMap
       metadata:
         name: vcloud-ccm-configmap
         namespace: kube-system
       data:
         vcloud-ccm-config.yaml: |+
           vcd:
             host: VCD_HOST
             org: ORG
             vdc: OVDC
           loadbalancer:
             oneArm:
               startIP: "192.168.8.2"
               endIP: "192.168.8.100"
             ports:
               http: 80
               https: 443
             network: NETWORK
             vipSubnet: ""
             certAlias: CLUSTER_ID-cert
             enableVirtualServiceSharedIP: false # supported for VCD >= 10.4
           clusterid: CLUSTER_ID
           vAppName: VAPP
       immutable: true
       ```
   - Replace VCD_HOST, ORG, OVDC, NETWORK, VAPP, and CLUSTER_ID with the relevant values.
   - Create the config map in the workload cluster:
     `kubectl --kubeconfig=workload_cluster.conf apply -f vcloud-ccm-config.yaml`



