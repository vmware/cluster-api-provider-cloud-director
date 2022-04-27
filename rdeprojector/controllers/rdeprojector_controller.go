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
	"context"
	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"net/http"
	"time"

	"github.com/vmware/cloud-provider-for-cloud-director/pkg/vcdsdk"
	capvcdv1alpha1 "github.com/vmware/cluster-api-provider-cloud-director/api/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// RDEProjectorReconciler reconciles a RDEProjector object
type RDEProjectorReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

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
	rdeProjector := &capvcdv1alpha1.RDEProjector{}
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
		For(&capvcdv1alpha1.RDEProjector{}).
		Complete(r)
}

func (r *RDEProjectorReconciler) reconcileNormal(ctx context.Context, rdeProjector *capvcdv1alpha1.RDEProjector) (ctrl.Result, error) {
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

	//TODO Apply CAPI YAML

	return ctrl.Result{RequeueAfter: time.Second * 20}, nil
}
