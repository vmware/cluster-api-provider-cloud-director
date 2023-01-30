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

package v1beta1

import (
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

// log is for logging in this package.
var vcdmachinelog = logf.Log.WithName("vcdmachine-resource")

func (r *VCDMachine) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

// TODO(user): EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

//+kubebuilder:webhook:path=/mutate-infrastructure-cluster-x-k8s-io-v1beta1-vcdmachine,mutating=true,failurePolicy=fail,sideEffects=None,groups=infrastructure.cluster.x-k8s.io,resources=vcdmachines,verbs=create;update,versions=v1beta1,name=mvcdmachine.kb.io,admissionReviewVersions=v1

var _ webhook.Defaulter = &VCDMachine{}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (r *VCDMachine) Default() {
	vcdmachinelog.Info("default", "name", r.Name)

	// TODO(user): fill in your defaulting logic.
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
//+kubebuilder:webhook:path=/validate-infrastructure-cluster-x-k8s-io-v1beta1-vcdmachine,mutating=false,failurePolicy=fail,sideEffects=None,groups=infrastructure.cluster.x-k8s.io,resources=vcdmachines,verbs=create;update,versions=v1beta1,name=vvcdmachine.kb.io,admissionReviewVersions=v1

var _ webhook.Validator = &VCDMachine{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *VCDMachine) ValidateCreate() error {
	vcdmachinelog.Info("validate create", "name", r.Name)

	// TODO(user): fill in your validation logic upon object creation.
	return nil
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *VCDMachine) ValidateUpdate(oldRaw runtime.Object) error {
	vcdmachinelog.Info("validate update", "name", r.Name)

	old, ok := oldRaw.(*VCDMachine)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected an VCDMachine but got a %T", oldRaw))
	}

	// the relation between CR->VM_NAME have to be consistent therefore VmNamingTemplate is immutable
	if r.Spec.VmNamingTemplate != old.Spec.VmNamingTemplate {
		return field.Forbidden(field.NewPath("spec.vmNamingTemplate"), "cannot be modified")
	}

	return nil
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *VCDMachine) ValidateDelete() error {
	vcdmachinelog.Info("validate delete", "name", r.Name)

	// TODO(user): fill in your validation logic upon object deletion.
	return nil
}
