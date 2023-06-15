#!/bin/bash

export GOROOT=~/go
export PATH=$PATH:$GOROOT/bin/

# Install kind
GO111MODULE="on" go get sigs.k8s.io/kind@v0.11.1

# Install clusterctl on Mac
curl -L https://github.com/kubernetes-sigs/cluster-api/releases/download/v1.1.3/clusterctl-darwin-amd64 -o clusterctl
chmod +x ./clusterctl
sudo mv ./clusterctl /usr/local/bin/clusterctl

# Check clusterctl version
clusterctl version

# Create kind cluster configuration file
cat > kind-cluster-with-extramounts.yaml <<EOF
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
- role: control-plane
  extraMounts:
    - hostPath: /var/run/docker.sock
      containerPath: /var/run/docker.sock
EOF

# Create a local cluster using kind
kind create cluster --config kind-cluster-with-extramounts.yaml

# Set the context to the created cluster
kubectl cluster-info --context kind-kind
kubectl config set-context kind-kind

# Verify the cluster is up and running
kubectl get po -A -owide

# Initialize clusterctl with required providers
clusterctl init --core cluster-api:v1.4.0 -b kubeadm:v1.4.0 -c kubeadm:v1.4.0 -i vcd:v1.1.0
