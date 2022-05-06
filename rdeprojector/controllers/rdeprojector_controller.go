/*
Copyright 2022.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/pkg/errors"
	"io"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/serializer/yaml"
	"k8s.io/apimachinery/pkg/types"
	k8syaml "k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/restmapper"
	_ "k8s.io/client-go/restmapper"
	"net/http"
	"time"

	"github.com/vmware/cloud-provider-for-cloud-director/pkg/vcdsdk"
	rdeprojectorv1alpha1 "github.com/vmware/cluster-api-provider-cloud-director/rdeprojector/api/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// RDEProjectorReconciler reconciles a RDEProjector object
type RDEProjectorReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

const (
	FieldManager = "rdeProjector"
)

//+kubebuilder:rbac:groups=capvcd.cloud-director.vmware.com,resources=rdeprojectors,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=capvcd.cloud-director.vmware.com,resources=rdeprojectors/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=capvcd.cloud-director.vmware.com,resources=rdeprojectors/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the RDEProjector object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.10.0/pkg/reconcile
func (r *RDEProjectorReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	rdeProjector := &rdeprojectorv1alpha1.RDEProjector{}
	if err := r.Client.Get(ctx, req.NamespacedName, rdeProjector); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}
	//TODO Evaluate if reconcileDelete() is really needed. Is there a need for finalizer at all?
	return r.reconcileNormal(ctx, rdeProjector)
}

// SetupWithManager sets up the controller with the Manager.
func (r *RDEProjectorReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&rdeprojectorv1alpha1.RDEProjector{}).
		Complete(r)
}

func (r *RDEProjectorReconciler) reconcileNormal(ctx context.Context, rdeProjector *rdeprojectorv1alpha1.RDEProjector) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)

	//TODO Ensure sysadmin persona works
	workloadVCDClient, err := vcdsdk.NewVCDClientFromSecrets(rdeProjector.Spec.Site, rdeProjector.Spec.Org,
		"", rdeProjector.Spec.Org, rdeProjector.Spec.UserCredentialsContext.Username,
		rdeProjector.Spec.UserCredentialsContext.Password, rdeProjector.Spec.UserCredentialsContext.RefreshToken,
		true, false)
	if err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "Error creating VCD client to reconcile RDE [%s]", rdeProjector.Spec.RDEId)
	}
	rde, resp, _, err := workloadVCDClient.APIClient.DefinedEntityApi.GetDefinedEntity(ctx, rdeProjector.Spec.RDEId)
	if err == nil && resp != nil && resp.StatusCode != http.StatusOK {
		log.Error(err, "Error retrieving RDEId of the cluster", "rdeId", rdeProjector.Spec.RDEId)
	}
	entity := rde.Entity
	capiYaml := entity["spec"].(map[string]interface{})["capiYaml"]
	log.Info("Retrieved Capi Yaml", "capiYaml", capiYaml)
	capiYaml = "apiVersion: cluster.x-k8s.io/v1alpha4\nkind: Cluster\nmetadata:\n  name: capi\n  namespace: default\nspec:\n  clusterNetwork:\n    pods:\n      cidrBlocks:\n        - 100.96.0.0/11 # pod CIDR for the cluster\n    serviceDomain: k8s.test\n    services:\n      cidrBlocks:\n        - 100.64.0.0/13 # service CIDR for the cluster\n  controlPlaneRef:\n    apiVersion: controlplane.cluster.x-k8s.io/v1alpha4\n    kind: KubeadmControlPlane\n    name: capi-control-plane # name of the KubeadmControlPlane object associated with the cluster.\n    namespace: default # kubernetes namespace in which the KubeadmControlPlane object reside. Should be the same namespace as that of the Cluster object\n  infrastructureRef:\n    apiVersion: infrastructure.cluster.x-k8s.io/v1beta1\n    kind: VCDCluster\n    name: capi # name of the VCDCluster object associated with the cluster.\n    namespace: default # kubernetes namespace in which the VCDCluster object resides. Should be the same namespace as that of the Cluster object\n---\napiVersion: infrastructure.cluster.x-k8s.io/v1beta1\nkind: VCDCluster\nmetadata:\n  name: capi\n  namespace: default\nspec:\n  site: https://bos1-vcloud-static-167-45.eng.vmware.com\n  org: org1 \n  ovdc: ovdc1\n  ovdcNetwork: ovdc1_nw \n  userContext:\n    username: clusteradmin\n    password: ca$hc0w\n    refreshToken: \"\"\n  defaultStorageClassOptions:\n    vcdStorageProfileName: gold\n  rdeId: \n\n---\napiVersion: infrastructure.cluster.x-k8s.io/v1beta1\nkind: VCDMachineTemplate\nmetadata:\n  name: capi-control-plane\n  namespace: default\nspec:\n  template:\n    spec:\n      catalog: cse \n      template: ubuntu-2004-kube-v1.20.8+vmware.1-tkg.1-17589475007677388652\n      sizingPolicy: 2core2gb \n---\napiVersion: controlplane.cluster.x-k8s.io/v1alpha4\nkind: KubeadmControlPlane\nmetadata:\n  name: capi-control-plane\n  namespace: default\nspec:\n  kubeadmConfigSpec:\n    clusterConfiguration:\n      apiServer:\n        certSANs:\n          - localhost\n          - 127.0.0.1\n      controllerManager:\n        extraArgs:\n          enable-hostpath-provisioner: \"true\"\n      dns:\n        imageRepository: projects.registry.vmware.com/tkg # image repository to pull the DNS image from\n        imageTag: v1.7.0_vmware.12 # DNS image tag associated with the TKGm OVA used. The values must be retrieved from the TKGm ova BOM. Refer to the github documentation for more details\n      etcd:\n        local:\n          imageRepository: projects.registry.vmware.com/tkg # image repository to pull the etcd image from\n          imageTag: v3.4.13_vmware.14 # etcd image tag associated with the TKGm OVA used. The values must be retrieved from the TKGm ova BOM. Refer to the github documentation for more details\n      imageRepository: projects.registry.vmware.com/tkg # image repository to use for the rest of kubernetes images\n    users:\n      - name: root\n        sshAuthorizedKeys:\n          - ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABgQC5uGlVEOWhHRETUyXCsG7YoFSGkbM8kj+YRvoEpjM0r+GoHCdjbYO3vA0RhpVOMpfRnw3jPjh0rtYWZleyEoxzhjHulF5DqOQ4eQHiXEztlI89e5XBCLBVUUsUG2HSnkANSqWQ7CzSbvXczJ1H6mhaZeYg6CklsO2IVukY3eopcCwE8RklkKN69opVyeC2JOEraj/lrjl6+NJzwcki/QnlJ/lgxReLXwZKQG0Owq0wnEM0s0ailCl+dH0vbcAD1rz/r6tez7qpN09hh1tbYdacohpzGJYRb+ufzpFwaGoy1U4ofbBWjR6ziO52QebuM1GgRoSNYTembKLwrFlySgojoA5oWP0bhqqxCeImvsKBfq9HNt7R2BDVDIZG7xenSMMCsU3GuzdPrVZAv5RMSHGSTO+JNhGqo6DEwWhl4QUgY18aNjQtXEYcPw7iRol2rjSrXWmOl3hmJX2g9hDXrLxpXojvvJwNowzmpGmo9UfgXfMU6JKY0wT/gGgBQZXkakE= sakthis@sakthis-a02.vmware.com  \n    initConfiguration:\n      nodeRegistration:\n        criSocket: /run/containerd/containerd.sock\n        kubeletExtraArgs:\n          eviction-hard: nodefs.available<0%,nodefs.inodesFree<0%,imagefs.available<0%\n          cloud-provider: external\n    joinConfiguration:\n      nodeRegistration:\n        criSocket: /run/containerd/containerd.sock\n        kubeletExtraArgs:\n          eviction-hard: nodefs.available<0%,nodefs.inodesFree<0%,imagefs.available<0%\n          cloud-provider: external\n  machineTemplate:\n    infrastructureRef:\n      apiVersion: infrastructure.cluster.x-k8s.io/v1beta1\n      kind: VCDMachineTemplate\n      name: capi-control-plane # name of the VCDMachineTemplate object used to deploy control plane VMs. Should be the same name as that of KubeadmControlPlane object\n      namespace: default # kubernetes namespace of the VCDMachineTemplate object. Should be the same namespace as that of the Cluster object\n  replicas: 1 \n  version: v1.20.8+vmware.1 # Kubernetes version to be used to create (or) upgrade the control plane nodes. The value needs to be retrieved from the respective TKGm ova BOM. Refer to the documentation.\n---\napiVersion: infrastructure.cluster.x-k8s.io/v1beta1\nkind: VCDMachineTemplate\nmetadata:\n  name: capi-md0\n  namespace: default\nspec:\n  template:\n    spec:\n      catalog: cse\n      template: ubuntu-2004-kube-v1.20.8+vmware.1-tkg.1-17589475007677388652 \n      sizingPolicy: 2core2gb \n---\napiVersion: bootstrap.cluster.x-k8s.io/v1alpha4\nkind: KubeadmConfigTemplate\nmetadata:\n  name: capi-md0\n  namespace: default\nspec:\n  template:\n    spec:\n      users:\n        - name: root\n          sshAuthorizedKeys:\n            - ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABgQC5uGlVEOWhHRETUyXCsG7YoFSGkbM8kj+YRvoEpjM0r+GoHCdjbYO3vA0RhpVOMpfRnw3jPjh0rtYWZleyEoxzhjHulF5DqOQ4eQHiXEztlI89e5XBCLBVUUsUG2HSnkANSqWQ7CzSbvXczJ1H6mhaZeYg6CklsO2IVukY3eopcCwE8RklkKN69opVyeC2JOEraj/lrjl6+NJzwcki/QnlJ/lgxReLXwZKQG0Owq0wnEM0s0ailCl+dH0vbcAD1rz/r6tez7qpN09hh1tbYdacohpzGJYRb+ufzpFwaGoy1U4ofbBWjR6ziO52QebuM1GgRoSNYTembKLwrFlySgojoA5oWP0bhqqxCeImvsKBfq9HNt7R2BDVDIZG7xenSMMCsU3GuzdPrVZAv5RMSHGSTO+JNhGqo6DEwWhl4QUgY18aNjQtXEYcPw7iRol2rjSrXWmOl3hmJX2g9hDXrLxpXojvvJwNowzmpGmo9UfgXfMU6JKY0wT/gGgBQZXkakE= sakthis@sakthis-a02.vmware.com \n      joinConfiguration:\n        nodeRegistration:\n          criSocket: /run/containerd/containerd.sock\n          kubeletExtraArgs:\n            eviction-hard: nodefs.available<0%,nodefs.inodesFree<0%,imagefs.available<0%\n            cloud-provider: external\n---\napiVersion: cluster.x-k8s.io/v1alpha4\nkind: MachineDeployment\nmetadata:\n  name: capi-md0\n  namespace: default\nspec:\n  clusterName: capi # name of the Cluster object\n  replicas: 1 \n  selector:\n    matchLabels: null\n  template:\n    spec:\n      bootstrap:\n        configRef:\n          apiVersion: bootstrap.cluster.x-k8s.io/v1alpha4\n          kind: KubeadmConfigTemplate\n          name: capi-md0 # name of the KubeadmConfigTemplate object\n          namespace: default # kubernetes namespace of the KubeadmConfigTemplate object. Should be the same namespace as that of the Cluster object\n      clusterName: capi # name of the Cluster object\n      infrastructureRef:\n        apiVersion: infrastructure.cluster.x-k8s.io/v1beta1\n        kind: VCDMachineTemplate\n        name: capi-md0 # name of the VCDMachineTemplate object used to deploy worker nodes\n        namespace: default # kubernetes namespace of the VCDMachineTemplate object used to deploy worker nodes\n      version: v1.20.8+vmware.1 # Kubernetes version to be used to create (or) upgrade the worker nodes. The value needs to be retrieved from the respective TKGm ova BOM. Refer to the documentation.\n"

	// Retrieve kubeconfig of the cluster
	kubeconfig := ctrl.GetConfigOrDie()

	// Prepare a RESTMapper to find GVR
	dc, err := discovery.NewDiscoveryClientForConfig(kubeconfig)
	if err != nil {
		return ctrl.Result{}, err
	}
	mapper := restmapper.NewDeferredDiscoveryRESTMapper(memory.NewMemCacheClient(dc))

	// Get the dynamic client
	dyn, err := dynamic.NewForConfig(kubeconfig)
	if err != nil {
		return ctrl.Result{}, err
	}
	var decUnstructured = yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)

	// Prepare the Yaml Reader
	yamlReader := k8syaml.NewYAMLReader(bufio.NewReader(bytes.NewReader([]byte(fmt.Sprintf("%v", capiYaml)))))

	for err == nil {
		// Read single API object at a time from the CAPI Yaml
		yamlBytes, err := yamlReader.Read()
		if err == io.EOF {
			break
		} else if err != nil {
			return ctrl.Result{}, errors.Wrapf(err, "Error reading object from the CAPI yaml of rdeId [%s]; yaml bytes: [%v]", rdeProjector.Spec.RDEId, yamlBytes)
		}

		// Decode the yaml API object into unstructured format
		obj := &unstructured.Unstructured{}
		_, gvk, err := decUnstructured.Decode(yamlBytes, nil, obj)
		if err != nil {
			return ctrl.Result{}, errors.Wrapf(err, "Error decoding CAPI object from the CAPI yaml of rdeId [%s] into unstructured format; yaml bytes: [%v] ", rdeProjector.Spec.RDEId, yamlBytes)
		}

		// Get GVR (Group, Version, Resource) of the object
		mapping, err := mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
		if err != nil {
			return ctrl.Result{}, errors.Wrapf(err, "Error constructing GVR of capi object retrieved from rdeId [%s]; gvk: [%v]", rdeProjector.Spec.RDEId, gvk)
		}

		// Get REST client/interface for the given GVR of the object
		var dr dynamic.ResourceInterface
		if mapping.Scope.Name() == meta.RESTScopeNameNamespace {
			// namespaced resources should specify the namespace
			dr = dyn.Resource(mapping.Resource).Namespace(obj.GetNamespace())
		} else {
			// for cluster-wide resources
			dr = dyn.Resource(mapping.Resource)
		}

		// Marshal object into the JSON
		data, err := json.Marshal(obj)
		if err != nil {
			return ctrl.Result{}, errors.Wrapf(err, "Error marshaling capi object into JSON format; rdeId: [%s]; capi object: [%v]", rdeProjector.Spec.RDEId, obj)
		}

		// Create/Update/Patch the object with the ServerSideApply; types.ApplyPatchType indicates SSA;
		// fieldManager specifies the owner of the fields getting patched.
		forceApply := true
		_, err = dr.Patch(ctx, obj.GetName(), types.ApplyPatchType, data, metav1.PatchOptions{
			FieldManager: FieldManager,
			Force:        &forceApply,
		})
		if err != nil {
			return ctrl.Result{}, errors.Wrapf(err, "Error patching capi object [%v] via SSA; rdeId: [%s]", rdeProjector.Spec.RDEId, obj)
		}
	}

	return ctrl.Result{RequeueAfter: time.Second * 60}, nil
}
