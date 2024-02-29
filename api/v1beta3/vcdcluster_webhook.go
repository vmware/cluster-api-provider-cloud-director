/*
Copyright 2021.

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

package v1beta3

import (
	"fmt"
	"github.com/google/go-cmp/cmp"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// log is for logging in this package.
var vcdclusterlog = logf.Log.WithName("vcdcluster-resource")

func (r *VCDCluster) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

// TODO(user): EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

// +kubebuilder:webhook:path=/mutate-infrastructure-cluster-x-k8s-io-v1beta3-vcdcluster,mutating=true,failurePolicy=fail,sideEffects=None,groups=infrastructure.cluster.x-k8s.io,resources=vcdclusters,verbs=create;update,versions=v1beta3,name=mutation.vcdcluster.infrastructure.cluster.x-k8s.io,admissionReviewVersions=v1
var _ webhook.Defaulter = &VCDCluster{}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (r *VCDCluster) Default() {
	vcdclusterlog.Info("default", "name", r.Name)

	// TODO(user): fill in your defaulting logic.
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
// +kubebuilder:webhook:path=/validate-infrastructure-cluster-x-k8s-io-v1beta3-vcdcluster,mutating=false,failurePolicy=fail,sideEffects=None,groups=infrastructure.cluster.x-k8s.io,resources=vcdclusters,verbs=create;update,versions=v1beta3,name=validation.vcdcluster.infrastructure.cluster.x-k8s.io,admissionReviewVersions=v1
var _ webhook.Validator = &VCDCluster{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *VCDCluster) ValidateCreate() (admission.Warnings, error) {
	vcdclusterlog.Info("validate create", "name", r.Name)

	// TODO(user): fill in your validation logic upon object creation.
	return nil, nil
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *VCDCluster) ValidateUpdate(old runtime.Object) (admission.Warnings, error) {
	vcdclusterlog.Info("validate update", "name", r.Name)

	var allErrs field.ErrorList

	oldVCDCluster, ok := old.(*VCDCluster)
	if !ok {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("expected a VCDCluster but got a %T", old))

	}
	if !cmp.Equal(oldVCDCluster.Spec.ControlPlaneEndpoint, APIEndpoint{}) &&
		!cmp.Equal(r.Spec.ControlPlaneEndpoint, oldVCDCluster.Spec.ControlPlaneEndpoint) {

		allErrs = append(allErrs,
			field.Invalid(field.NewPath("spec", "controlPlaneEndpoint"),
				r.Spec.ControlPlaneEndpoint, "field is immutable"))
	}

	if len(allErrs) == 0 {
		// no errors
		return nil, nil
	}
	return nil, apierrors.NewInvalid(r.GroupVersionKind().GroupKind(), r.Name, allErrs)
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *VCDCluster) ValidateDelete() (admission.Warnings, error) {
	vcdclusterlog.Info("validate delete", "name", r.Name)

	// TODO(user): fill in your validation logic upon object deletion.
	return nil, nil
}
