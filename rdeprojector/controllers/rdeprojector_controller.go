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
	log := ctrl.LoggerFrom(ctx, "rdeId", rdeProjector.Spec.RDEId)

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
