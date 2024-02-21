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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"reflect"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// log is for logging in this package.
var vcdclustertemplatelog = logf.Log.WithName("vcdclustertemplate-resource")

func (r *VCDClusterTemplate) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

// TODO(user): EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

//+kubebuilder:webhook:path=/mutate-infrastructure-cluster-x-k8s-io-v1beta3-vcdclustertemplate,mutating=true,failurePolicy=fail,sideEffects=None,groups=infrastructure.cluster.x-k8s.io,resources=vcdclustertemplates,verbs=create;update,versions=v1beta3,name=mutation.vcdclustertemplate.infrastructure.cluster.x-k8s.io,admissionReviewVersions=v1

var _ webhook.Defaulter = &VCDClusterTemplate{}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (r *VCDClusterTemplate) Default() {
	vcdclustertemplatelog.Info("default", "name", r.Name)

	// TODO(user): fill in your defaulting logic.
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
//+kubebuilder:webhook:path=/validate-infrastructure-cluster-x-k8s-io-v1beta3-vcdclustertemplate,mutating=false,failurePolicy=fail,sideEffects=None,groups=infrastructure.cluster.x-k8s.io,resources=vcdclustertemplates,verbs=create;update,versions=v1beta3,name=validation.vcdclustertemplate.infrastructure.cluster.x-k8s.io,admissionReviewVersions=v1

var _ webhook.Validator = &VCDClusterTemplate{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *VCDClusterTemplate) ValidateCreate() (admission.Warnings, error) {
	vcdclustertemplatelog.Info("validate create", "name", r.Name)

	// TODO(user): fill in your validation logic upon object creation.
	return nil, nil
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *VCDClusterTemplate) ValidateUpdate(old runtime.Object) (admission.Warnings, error) {
	vcdclustertemplatelog.Info("validate update", "name", r.Name)

	// Do not allow updates to vcdClusterTemplate.spec.template.spec
	var allErrs field.ErrorList
	oldVCDClusterTemplate := old.(*VCDClusterTemplate)
	if !reflect.DeepEqual(r.Spec.Template.Spec, oldVCDClusterTemplate.Spec.Template.Spec) {
		errMessage := "VCDClusterTemplate.spec.template.spec is immutable. " +
			"To change the VCDClusterTemplate used in a cluster, please create a new VCDClusterTemplate and ClusterClass and " +
			"update the cluster.spec.topology.class with the new clusterclass"
		allErrs = append(allErrs,
			field.Invalid(field.NewPath("VCDClusterTemplate", "spec", "template", "spec"), r, errMessage),
		)
	}
	if len(allErrs) == 0 {
		return nil, nil
	}
	return nil, apierrors.NewInvalid(r.GroupVersionKind().GroupKind(), r.Name, allErrs)
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *VCDClusterTemplate) ValidateDelete() (admission.Warnings, error) {
	vcdclustertemplatelog.Info("validate delete", "name", r.Name)

	// TODO(user): fill in your validation logic upon object deletion.
	return nil, nil
}
