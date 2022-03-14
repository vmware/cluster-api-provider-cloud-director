#!/bin/bash -e

if [ "$#" -lt 2 ]
then
  echo "Usage: $0 <cluster name> <directory with secrets>"
  exit 1
fi

cluster_name=$1
secrets_dir=$2
if [ ! -d ${secrets_dir} ]
then
  echo "[${secrets_dir}] is not a path to a valid directory"
  exit 1
fi

[ -z "${VCD_USERNAME}" ] && echo "Environment variable `VCD_USERNAME` should be set" && exit 1
[ -z "${VCD_PASSWORD}" ] && echo "Environment variable `VCD_PASSWORD` should be set" && exit 1

set +e
vcd pwd
if [ "$?" -ne 0 ]
then
  echo "It looks like there is not an active session with VCD. Try `vcd login` first."
  exit 1
fi

vcd cse cluster info ${cluster_name} > ${cluster_name}_info.txt

vcd_site=$(yq e ".metadata.site" ${cluster_name}_info.txt)
[ -z "${vcd_site}" ] && echo "Could not retrieve vcd site from cluster info of [${cluster_name}]" && exit 1
echo "VCD Site is [${vcd_site}]"

org=$(yq e ".metadata.orgName" ${cluster_name}_info.txt)
[ -z "${org}" ] && echo "Could not retrieve orgName from cluster info of [${cluster_name}]" && exit 1
echo "Org is [${org}]"

ovdc=$(yq e ".metadata.virtualDataCenterName" ${cluster_name}_info.txt)
[ -z "${ovdc}" ] && echo "Could not retrieve virtualDataCenterName from cluster info of [${cluster_name}]" && exit 1
echo "OrgVDC is [${ovdc}]"

ovdc_nw=$(yq e ".spec.settings.ovdcNetwork" ${cluster_name}_info.txt)
[ -z "${ovdc_nw}" ] && echo "Could not retrieve ovdcNetwork from cluster info of [${cluster_name}]" && exit 1
echo "OrgVDCNetwork is [${ovdc_nw}]"

template=$(yq e ".spec.distribution.templateName" ${cluster_name}_info.txt)
[ -z "${template}" ] && echo "Could not retrieve templateName from cluster info of [${cluster_name}]" && exit 1
echo "Template is [${template}]"

out=$(yq e ".status.private.kubeToken" ${cluster_name}_info.txt)
token=$(echo ${out} | tr -s " " " " | cut -f5 -d' ')
[ -z "${token}" ] && echo "Could not retrieve kubernetes join token from cluster info of [${cluster_name}]" && exit 1
echo "Token is [${token}]"

ca_cert_hash=$(echo ${out} | tr -s " " " " | cut -f7 -d' ')
[ -z "${ca_cert_hash}" ] && echo "Could not retrieve kubernetes cert hash from cluster info of [${cluster_name}]" && exit 1
echo "Discovery Token CA Cert Hash is [${ca_cert_hash}]"

catalog_lst=$(vcd catalog list | tail -n +3 | tr -s " " " " | cut -f5 -d' ')
for catalog_name in ${catalog_lst}
do
  vcd catalog list ${catalog_name} | tail -n +3 | tr -s " " " " | cut -f4 -d' ' | grep ${template}
  if [ "$?" -eq 0 ]
  then
    catalog=${catalog_name}
  fi
done
[ -z "${catalog}" ] && echo "Could not retrieve catalog name for template [${template}]" && exit 1
echo "Catalog is [${catalog}]"

compute_policy=$(yq e ".status.nodes.controlPlane.sizingClass" ${cluster_name}_info.txt)
[ -z "${compute_policy}" ] && echo "Could not retrieve compute policy name for cluster [${cluster_name}]" && exit 1
echo "Compute policy is [${compute_policy}]"

file_lst=$(echo "${secrets_dir}/admin.conf" \
  "${secrets_dir}/ca.key ${secrets_dir}/ca.crt" \
  "${secrets_dir}/etcd/ca.key ${secrets_dir}/etcd/ca.crt" \
  "${secrets_dir}/front-proxy-ca.key ${secrets_dir}/front-proxy-ca.crt" \
  "${secrets_dir}/sa.key ${secrets_dir}/sa.pub" \
)

echo "Validating files in [${file_lst}]"
for file in ${file_lst}
do
  if [ ! -f "${file}" ]
  then
    echo "File [${file}] does not exist or is not a regular file."
    exit 1
  fi
done

ip_port=$(cat ${secrets_dir}/admin.conf | grep "server:" | cut -f3 -d"/")
[ -z "${ip_port}" ] && echo "Could not retrieve ip:port of api-server from admin.conf" && exit 1

control_plane_node=$(kubectl --kubeconfig=${secrets_dir}/admin.conf get no -oname | grep mstr | cut -f2 -d'/')
[ -z "${control_plane_node}" ] && echo "Could not retrieve control-plane node name from cluster" && exit 1
echo "Control plane node name is [${control_plane_node}]"

worker_nodes=$(kubectl --kubeconfig=${secrets_dir}/admin.conf get no -oname | grep "node-" | cut -f2 -d'/')
[ -z "${control_plane_node}" ] && echo "Could not retrieve worker node names from cluster" && exit 1
echo "Worker node names are [${worker_nodes}]"

exit 0

kubectl create ns ${cluster_name}
kubectl -n ${cluster_name} create secret tls ${cluster_name}-ca \
  --key "${secrets_dir}/ca.key" --cert "${secrets_dir}/ca.crt"
kubectl -n ${cluster_name} create secret tls ${cluster_name}-etcd \
  --key "${secrets_dir}/etcd/ca.key" --cert "${secrets_dir}/etcd/ca.crt"
kubectl -n ${cluster_name} create secret tls ${cluster_name}-proxy \
  --key "${secrets_dir}/front-proxy-ca.key" --cert "${secrets_dir}/front-proxy-ca.crt"
kubectl -n ${cluster_name} create secret generic ${cluster_name}-sa \
  --type cluster.x-k8s.io/secret --from-file=tls.key=${secrets_dir}/sa.key --from-file=tls.crt=${secrets_dir}/sa.pub

kubectl -n ${cluster_name} create secret generic ${cluster_name}-kubeconfig \
  --type "cluster.x-k8s.io/secret" --from-file=value=${secrets_dir}/admin.conf
kubectl -n ${cluster_name} label secret ${cluster_name}-kubeconfig cluster.x-k8s.io/cluster-name=${cluster_name}

cat > kubeadm-master-cloud-init.yaml <<END
## template: jinja
#cloud-config
write_files:
-   path: /etc/kubernetes/pki/ca.crt
    owner: root:root
    permissions: '0640'
    encoding: b64
    content: $(cat ${secrets_dir}/ca.crt | base64)
-   path: /etc/kubernetes/pki/ca.key
    owner: root:root
    permissions: '0600'
    encoding: b64
    content: $(cat ${secrets_dir}/ca.key | base64)
-   path: /etc/kubernetes/pki/etcd/ca.crt
    owner: root:root
    permissions: '0640'
    encoding: b64
    content: $(cat ${secrets_dir}/etcd/ca.crt | base64)
-   path: /etc/kubernetes/pki/etcd/ca.key
    owner: root:root
    permissions: '0600'
    encoding: b64
    content: $(cat ${secrets_dir}/etcd/ca.key | base64)
-   path: /etc/kubernetes/pki/front-proxy-ca.crt
    owner: root:root
    permissions: '0640'
    encoding: b64
    content: $(cat ${secrets_dir}/front-proxy-ca.crt | base64)
-   path: /etc/kubernetes/pki/front-proxy-ca.key
    owner: root:root
    permissions: '0600'
    encoding: b64
    content: $(cat ${secrets_dir}/front-proxy-ca.key | base64)
-   path: /etc/kubernetes/pki/sa.pub
    owner: root:root
    permissions: '0640'
    encoding: b64
    content: $(cat ${secrets_dir}/sa.pub | base64)
-   path: /etc/kubernetes/pki/sa.key
    owner: root:root
    permissions: '0600'
    encoding: b64
    content: $(cat ${secrets_dir}/sa.key | base64)
-   path: /run/kubeadm/kubeadm.yaml
    owner: root:root
    permissions: '0640'
    content: |
      ---
      apiServer:
        certSANs:
        - localhost
        - 127.0.0.1
        timeoutForControlPlane: 2m0s
      apiVersion: kubeadm.k8s.io/v1beta2
      clusterName: ${cluster_name}
      controlPlaneEndpoint: ${ip_port}
      controllerManager:
        extraArgs:
          enable-hostpath-provisioner: "true"
      dns:
        imageRepository: projects.registry.vmware.com/tkg
        imageTag: v1.8.0_vmware.5
      etcd:
        local:
          imageRepository: projects.registry.vmware.com/tkg
          imageTag: v3.4.13_vmware.15
      imageRepository: projects.registry.vmware.com/tkg
      kind: ClusterConfiguration
      kubernetesVersion: ${kubernetes_version}
      networking:
        dnsDomain: k8s.test
        podSubnet: 100.96.0.0/11
        serviceSubnet: 100.64.0.0/13
      scheduler: {}
      ---
      apiVersion: kubeadm.k8s.io/v1beta2
      kind: InitConfiguration
      localAPIEndpoint: {}
      nodeRegistration:
        criSocket: unix:///run/containerd/containerd.sock
        kubeletExtraArgs:
          cloud-provider: external
          eviction-hard: nodefs.available<0%,nodefs.inodesFree<0%,imagefs.available<0%
-   path: /run/cluster-api/placeholder
    owner: root:root
    permissions: '0640'
    content: "This placeholder file is used to create the /run/cluster-api sub directory in a way that is compatible with both Linux and Windows (mkdir -p /run/cluster-api does not work with Windows)"
runcmd:
  - 'kubeadm init --config /run/kubeadm/kubeadm.yaml  && echo success > /run/cluster-api/bootstrap-success.complete'
END

kubectl -n ${cluster_name} create secret generic ${control_plane_node} \
  --type "cluster.x-k8s.io/secret" --from-file=value=kubeadm-master-cloud-init.yaml

cat > kubeadm-worker-cloud-init.yaml <<END
## template: jinja
#cloud-config
write_files:
-   path: /run/kubeadm/kubeadm-join-config.yaml
    owner: root:root
    permissions: '0640'
    content: |
      ---
      apiVersion: kubeadm.k8s.io/v1beta2
      discovery:
        bootstrapToken:
          apiServerEndpoint: ${ip_port}
          caCertHashes:
          - ${ca_cert_hash}
          token: ${token}
      kind: JoinConfiguration
      nodeRegistration:
        criSocket: unix:///run/containerd/containerd.sock
        kubeletExtraArgs:
          cloud-provider: external
          eviction-hard: nodefs.available<0%,nodefs.inodesFree<0%,imagefs.available<0%
-   path: /run/cluster-api/placeholder
    owner: root:root
    permissions: '0640'
    content: "This placeholder file is used to create the /run/cluster-api sub directory in a way that is compatible with both Linux and Windows (mkdir -p /run/cluster-api does not work with Windows)"
runcmd:
  - kubeadm join --config /run/kubeadm/kubeadm-join-config.yaml  && echo success > /run/cluster-api/bootstrap-success.complete
END

for worker_node in ${worker_nodes}
do
  kubectl -n ${cluster_name} create secret generic ${worker_node} \
    --type "cluster.x-k8s.io/secret" --from-file=value=kubeadm-worker-cloud-init.yaml
done

cat > ${control_plane_node}-control-plane.yaml <<END
apiVersion: bootstrap.cluster.x-k8s.io/v1alpha4
kind: KubeadmConfig
metadata:
  annotations:
    cluster.x-k8s.io/cloned-from-groupkind: KubeadmConfigTemplate.bootstrap.cluster.x-k8s.io
  labels:
    cluster.x-k8s.io/cluster-name: ${cluster_name}
    cluster.x-k8s.io/deployment-name: ${control_plane_node}
  name: ${control_plane_node}-control-plane
  namespace: ${cluster_name}
  uid: $(uuidgen)
spec:
  clusterConfiguration:
    controlPlaneEndpoint: ${ip_port}
    apiServer:
      timeoutForControlPlane: 2m0s
      certSANs:
      - localhost
      - 127.0.0.1
    controllerManager:
      extraArgs:
        enable-hostpath-provisioner: "true"
  initConfiguration:
    apiVersion: kubeadm.k8s.io/v1beta2
    bootstrapTokens:
    - ttl: 0s
      token: ${token}
    nodeRegistration:
      criSocket: unix:///run/containerd/containerd.sock
      kubeletExtraArgs:
        cloud-provider: external
        eviction-hard: nodefs.available<0%,nodefs.inodesFree<0%,imagefs.available<0%
END

kubectl apply -f ${control_plane_node}-control-plane.yaml

# TODO: validate all parameters above
# TODO: Get kubernetes_version, etcd version etc from a template

kubernetes_version="v1.21.2+vmware.1"
num_workers=$(echo ${worker_nodes} | wc -w)

cat > capialreadyexisting-no-workers.yaml <<END
---
apiVersion: cluster.x-k8s.io/v1alpha4
kind: Cluster
metadata:
  name: ${cluster_name}
  namespace: ${cluster_name}
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
    name: ${control_plane_node}-control-plane
    namespace: ${cluster_name}
  infrastructureRef:
    apiVersion: infrastructure.cluster.x-k8s.io/v1alpha4
    kind: VCDCluster
    name: ${cluster_name}
    namespace: ${cluster_name}
---
apiVersion: infrastructure.cluster.x-k8s.io/v1alpha4
kind: VCDCluster
metadata:
  name: ${cluster_name}
  namespace: ${cluster_name}
spec:
  site: ${vcd_site}
  org: ${org}
  ovdc: ${ovdc}
  ovdcNetwork: ${ovdc_nw}
  isMigratedR1Cluster: true
  r1ClusterEndpoint: ${ip_port}
  userContext:
    username: ${username}
    password: ${password}
    refreshToken:
---
apiVersion: infrastructure.cluster.x-k8s.io/v1alpha4
kind: VCDMachineTemplate
metadata:
  name: capi-md-0
  namespace: ${cluster_name}
spec:
  template:
    spec:
      catalog: ${catalog}
      template: ${template}
      computePolicy: "${compute_policy}"
---
apiVersion: bootstrap.cluster.x-k8s.io/v1alpha4
kind: KubeadmConfigTemplate
metadata:
  name: capi-md-0
  namespace: ${cluster_name}
spec:
  template:
    spec:
      joinConfiguration:
        nodeRegistration:
          criSocket: unix:///run/containerd/containerd.sock
          kubeletExtraArgs:
            cloud-provider: external
            eviction-hard: nodefs.available<0%,nodefs.inodesFree<0%,imagefs.available<0%
---
apiVersion: cluster.x-k8s.io/v1alpha4
kind: MachineDeployment
metadata:
  name: capi-md-0
  namespace: ${cluster_name}
spec:
  clusterName: ${cluster_name}
  replicas: ${num_workers}
  selector:
    matchLabels: null
  template:
    spec:
      bootstrap:
        configRef:
          apiVersion: bootstrap.cluster.x-k8s.io/v1alpha4
          kind: KubeadmConfigTemplate
          name: capi-md-0
          namespace: ${cluster_name}
      clusterName: ${cluster_name}
      infrastructureRef:
        apiVersion: infrastructure.cluster.x-k8s.io/v1alpha4
        kind: VCDMachineTemplate
        name: capi-md-0
        namespace: ${cluster_name}
      version: ${kubernetes_version}
---
apiVersion: bootstrap.cluster.x-k8s.io/v1alpha4
kind: KubeadmConfigTemplate
metadata:
  name: ${control_plane_node}
  namespace: ${cluster_name}
spec:
  template:
    spec:
      clusterConfiguration:
        controlPlaneEndpoint: ${ip_port}
        apiServer:
          timeoutForControlPlane: 2m0s
          certSANs:
          - localhost
          - 127.0.0.1
        controllerManager:
          extraArgs:
            enable-hostpath-provisioner: "true"
      initConfiguration:
        apiVersion: kubeadm.k8s.io/v1beta2
        bootstrapTokens:
        - ttl: 0s
          token: ${token}
        nodeRegistration:
          criSocket: unix:///run/containerd/containerd.sock
          kubeletExtraArgs:
            cloud-provider: external
            eviction-hard: nodefs.available<0%,nodefs.inodesFree<0%,imagefs.available<0%
---
apiVersion: cluster.x-k8s.io/v1alpha4
kind: Machine
metadata:
  name: ${control_plane_node}
  namespace: ${cluster_name}
  labels:
    cluster.x-k8s.io/control-plane: nil
spec:
  clusterName: ${cluster_name}
  bootstrap:
    configRef:
      apiVersion: bootstrap.cluster.x-k8s.io/v1alpha4
      kind: KubeadmConfig
      name: ${control_plane_node}-control-plane
      namespace: ${cluster_name}
  infrastructureRef:
    apiVersion: infrastructure.cluster.x-k8s.io/v1alpha4
    kind: VCDMachine
    name: ${control_plane_node}
    namespace: ${cluster_name}
  version: ${kubernetes_version}
---
apiVersion: infrastructure.cluster.x-k8s.io/v1alpha4
kind: VCDMachine
metadata:
  name: ${control_plane_node}
  namespace: ${cluster_name}
spec:
  isMigratedR1Machine: true
---
apiVersion: controlplane.cluster.x-k8s.io/v1alpha4
kind: KubeadmControlPlane
metadata:
  name: ${control_plane_node}-control-plane
  namespace: ${cluster_name}
spec:
  kubeadmConfigSpec:
    clusterConfiguration:
      apiServer:
        timeoutForControlPlane: 2m0s
        certSANs:
        - localhost
        - 127.0.0.1
      controllerManager:
        extraArgs:
          enable-hostpath-provisioner: "true"
      dns:
        imageRepository: projects.registry.vmware.com/tkg
        imageTag: v1.8.0_vmware.5
      etcd:
        local:
          imageRepository: projects.registry.vmware.com/tkg
          imageTag: v3.4.13_vmware.15
      imageRepository: projects.registry.vmware.com/tkg
      kubernetesVersion: ${kubernetes_version}
    initConfiguration:
      nodeRegistration:
        criSocket: unix:///run/containerd/containerd.sock
        kubeletExtraArgs:
          cloud-provider: external
          eviction-hard: nodefs.available<0%,nodefs.inodesFree<0%,imagefs.available<0%
    joinConfiguration:
      nodeRegistration:
        criSocket: unix:///run/containerd/containerd.sock
        kubeletExtraArgs:
          cloud-provider: external
          eviction-hard: nodefs.available<0%,nodefs.inodesFree<0%,imagefs.available<0%
  machineTemplate:
    infrastructureRef:
      apiVersion: infrastructure.cluster.x-k8s.io/v1alpha4
      kind: VCDMachineTemplate
      name: ${control_plane_node}-template
      namespace: ${cluster_name}
  replicas: 1
  version: ${kubernetes_version}
---
apiVersion: infrastructure.cluster.x-k8s.io/v1alpha4
kind: VCDMachineTemplate
metadata:
  name: ${control_plane_node}-template
  namespace: ${cluster_name}
spec:
  template:
    spec:
      catalog: ${catalog}
      template: ${template}
      computePolicy: "${compute_policy}"
---
END

kubectl apply -f capialreadyexisting-no-workers.yaml

for worker_node in ${worker_nodes}
do
  cat > capialreadyexisting-${worker_node}.yaml << END
---
apiVersion: bootstrap.cluster.x-k8s.io/v1alpha4
kind: KubeadmConfigTemplate
metadata:
  name: ${worker_node}
  namespace: ${cluster_name}
spec:
  template:
    spec:
      joinConfiguration:
        nodeRegistration:
          criSocket: unix:///run/containerd/containerd.sock
          kubeletExtraArgs:
            cloud-provider: external
            eviction-hard: nodefs.available<0%,nodefs.inodesFree<0%,imagefs.available<0%
---
apiVersion: cluster.x-k8s.io/v1alpha4
kind: Machine
metadata:
  name: ${worker_node}
  namespace: ${cluster_name}
spec:
  clusterName: ${cluster_name}
  bootstrap:
    configRef:
      apiVersion: bootstrap.cluster.x-k8s.io/v1alpha4
      kind: KubeadmConfig
      name: ${worker_node}
      namespace: ${cluster_name}
  infrastructureRef:
    apiVersion: infrastructure.cluster.x-k8s.io/v1alpha4
    kind: VCDMachine
    name: ${worker_node}
    namespace: ${cluster_name}
  version: ${kubernetes_version}
---
apiVersion: infrastructure.cluster.x-k8s.io/v1alpha4
kind: VCDMachine
metadata:
  name: ${worker_node}
  namespace: ${cluster_name}
spec:
  isMigratedR1Machine: true
---
END

  kubectl apply -f capialreadyexisting-${worker_node}.yaml
done

echo "Migration of cluster [${cluster_name}] is successfully started."

machine_lst=$(kubectl -n ${cluster_name} get machines -o name)
echo "Checking if machines [${machine_lst}] are in Running state"
while true
do
  all_running=true
  for machine in ${machine_lst}
  do
    machine_state=$(kubectl -n ${cluster_name} get ${machine} -o jsonpath="{.status.phase}")
    if [ "${machine_state}" != "Running" ]
    then
    echo "[${machine}] has state [${machine_state}]"
      all_running=false
      break
    fi
  done

  if ${all_running}
  then
    echo "All machines in [${machine_lst}] are in Running state"
    break
  fi

  echo "Sleeping for 30 seconds since all machines are not in Running state"
  sleep 30
done

# Now wait for old control-plane node to go down if it exists
set +e
echo "Waiting until the old control-plane node [${control_plane_node}] has successfully been deleted."
while true
do
  kubectl -n ${cluster_name} get machine "${control_plane_node} 2>/dev/null"
  if [ "$?" -ne 0 ]
  then
    break
  fi
  echo "Sleeping for 30 seconds waiting for old control-plane-node [${control_plane_node}] to go down..."
  sleep 30
done
set -e

echo "Cluster [${cluster_name}] migrated successfully. Please verify if the workloads are still in place."
echo "You may need to delete old workers manually."

# TODO: script the following:
# 1. delete old worker nodes
# 2. Move CRDs to cluster thereby making it self-managing


exit 0
