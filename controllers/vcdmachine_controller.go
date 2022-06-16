/*
   Copyright 2021 VMware, Inc.
   SPDX-License-Identifier: Apache-2.0
*/

package controllers

import (
	"bytes"
	"context"
	_ "embed" // this needs go 1.16+
	b64 "encoding/base64"
	"fmt"
	"github.com/pkg/errors"
	"github.com/replicatedhq/troubleshoot/pkg/redact"
	cpiutil "github.com/vmware/cloud-provider-for-cloud-director/pkg/util"
	"github.com/vmware/cloud-provider-for-cloud-director/pkg/vcdsdk"
	infrav1 "github.com/vmware/cluster-api-provider-cloud-director/api/v1beta1"
	"github.com/vmware/cluster-api-provider-cloud-director/pkg/capisdk"
	"github.com/vmware/cluster-api-provider-cloud-director/pkg/config"
	"github.com/vmware/cluster-api-provider-cloud-director/release"
	"github.com/vmware/go-vcloud-director/v2/govcd"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/klog"
	"math"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/annotations"
	"sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/cluster-api/util/predicates"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/source"
	"strconv"
	"strings"
	"text/template"
	"time"
)

type CloudInitScriptInput struct {
	ControlPlane              bool   // control plane node
	NvidiaGPU                 bool   // configure containerd for NVIDIA libraries
	BootstrapRunCmd           string // bootstrap run command
	B64OrgUser                string // base 64 org/username
	B64Password               string // base64 password
	B64RefreshToken           string // refresh token
	K8sStorageClassName       string // default storage class name
	ReclaimPolicy             string // reclaim policy
	VcdStorageProfileName     string // vcd storage profile
	FileSystemFormat          string // filesystem
	HTTPProxy                 string // httpProxy endpoint
	HTTPSProxy                string // httpsProxy endpoint
	NoProxy                   string // no proxy values
	CpiVersion                string // cpi version
	VcdHostFormatted          string // vcd host
	ClusterOrgName            string // org
	ClusterOVDCName           string // ovdc
	NetworkName               string // network
	VipSubnetCidr             string // vip subnet cidr - empty for now for CPI to select subnet
	VAppName                  string // vApp name
	ClusterID                 string // cluster id
	CsiVersion                string // csi version
	EnableDefaultStorageClass string // is_storage_class_enabled
	MachineName               string // vm host name
}

const (
	ReclaimPolicyDelete = "Delete"
	ReclaimPolicyRetain = "Retain"

	VcdResourceTypeVM = "virtual-machine"
)

const Mebibyte = 1048576

// The following `embed` directives read the file in the mentioned path and copy the content into the declared variable.
// These variables need to be global within the package.
//go:embed cluster_scripts/cloud_init.tmpl
var cloudInitScriptTemplate string

// VCDMachineReconciler reconciles a VCDMachine object
type VCDMachineReconciler struct {
	client.Client
	Config *config.CAPVCDConfig
}

//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=vcdmachines,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=vcdmachines/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=vcdmachines/finalizers,verbs=update
func (r *VCDMachineReconciler) Reconcile(ctx context.Context, req ctrl.Request) (_ ctrl.Result, rerr error) {
	log := ctrl.LoggerFrom(ctx)

	// Fetch the VCDMachine instance.
	vcdMachine := &infrav1.VCDMachine{}
	if err := r.Client.Get(ctx, req.NamespacedName, vcdMachine); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	machine, err := util.GetOwnerMachine(ctx, r.Client, vcdMachine.ObjectMeta)
	if err != nil {
		return ctrl.Result{}, err
	}
	if machine == nil {
		log.Info("Waiting for Machine Controller to set OwnerRef on VCDMachine")
		return ctrl.Result{}, nil
	}

	log = log.WithValues("machine", machine.Name)

	// Fetch the Cluster from k8s etcd.
	cluster, err := util.GetClusterFromMetadata(ctx, r.Client, machine.ObjectMeta)
	if err != nil {
		log.Info("VCDMachine owner Machine is missing cluster label or cluster does not exist")
		return ctrl.Result{}, err
	}
	if cluster == nil {
		log.Info("Please associate this machine with a cluster using the label", "label", clusterv1.ClusterLabelName)
		return ctrl.Result{}, nil
	}

	log = log.WithValues("cluster", cluster.Name)

	// Return early if the object or Cluster is paused.
	if annotations.IsPaused(cluster, vcdMachine) {
		log.Info("Reconciliation is paused for this object")
		return ctrl.Result{}, nil
	}

	machineBeingDeleted := !vcdMachine.ObjectMeta.DeletionTimestamp.IsZero()

	// Fetch the VCD Cluster.
	vcdCluster := &infrav1.VCDCluster{}
	vcdClusterName := client.ObjectKey{
		Namespace: vcdMachine.Namespace,
		Name:      cluster.Spec.InfrastructureRef.Name,
	}
	if err := r.Client.Get(ctx, vcdClusterName, vcdCluster); err != nil {
		log.Info("VCDCluster is not available yet")
		if !machineBeingDeleted {
			return ctrl.Result{}, nil
		} else {
			log.Info("Continuing to delete the VCDMachine, since deletion timestamp is set")
		}
	}

	// Initialize the patch helper
	patchHelper, err := patch.NewHelper(vcdMachine, r)
	if err != nil {
		return ctrl.Result{}, err
	}
	// Always attempt to Patch the VCDMachine object and status after each reconciliation.
	defer func() {
		if err := patchVCDMachine(ctx, patchHelper, vcdMachine); err != nil {
			log.Error(err, "Failed to patch VCDMachine")
			if rerr == nil {
				rerr = err
			}
		}
	}()

	// Add finalizer first if not exist to avoid the race condition between init and delete
	if !controllerutil.ContainsFinalizer(vcdMachine, infrav1.MachineFinalizer) {
		controllerutil.AddFinalizer(vcdMachine, infrav1.MachineFinalizer)
		return ctrl.Result{}, nil
	}

	// If the machine is not being deleted, check if the infrastructure is ready. If not ready, return and wait for
	// the cluster object to be updated
	if !machineBeingDeleted && !cluster.Status.InfrastructureReady {
		log.Info("Waiting for VCDCluster Controller to create cluster infrastructure")
		conditions.MarkFalse(vcdMachine, ContainerProvisionedCondition,
			WaitingForClusterInfrastructureReason, clusterv1.ConditionSeverityInfo, "")
		return ctrl.Result{}, nil
	}

	// Handle deleted machines
	if machineBeingDeleted {
		return r.reconcileDelete(ctx, cluster, machine, vcdMachine, vcdCluster)
	}

	// Handle non-deleted machines
	return r.reconcileNormal(ctx, cluster, machine, vcdMachine, vcdCluster)
}

func patchVCDMachine(ctx context.Context, patchHelper *patch.Helper, vcdMachine *infrav1.VCDMachine) error {
	conditions.SetSummary(vcdMachine,
		conditions.WithConditions(
			ContainerProvisionedCondition,
			BootstrapExecSucceededCondition,
		),
		conditions.WithStepCounterIf(vcdMachine.ObjectMeta.DeletionTimestamp.IsZero()),
	)

	return patchHelper.Patch(
		ctx,
		vcdMachine,
		patch.WithOwnedConditions{Conditions: []clusterv1.ConditionType{
			clusterv1.ReadyCondition,
			ContainerProvisionedCondition,
			BootstrapExecSucceededCondition,
		}},
	)
}

const (
	NetworkConfiguration                   = "guestinfo.postcustomization.networkconfiguration.status"
	ProxyConfiguration                     = "guestinfo.postcustomization.proxy.setting.status"
	KubeadmInit                            = "guestinfo.postcustomization.kubeinit.status"
	KubectlApplyCpi                        = "guestinfo.postcustomization.kubectl.cpi.install.status"
	KubectlApplyCsi                        = "guestinfo.postcustomization.kubectl.csi.install.status"
	KubeadmTokenGenerate                   = "guestinfo.postcustomization.kubeadm.token.generate.status"
	KubectlApplyDefaultStorageClass        = "guestinfo.postcustomization.kubectl.default_storage_class.install.status"
	KubeadmNodeJoin                        = "guestinfo.postcustomization.kubeadm.node.join.status"
	NvidiaRuntimeInstall                   = "guestinfo.postcustomization.nvidia.runtime.install.status"
	NvidiaContainerdConfiguration          = "guestinfo.postcustomization.containerd.nvidia.configuration.status"
	PostCustomizationScriptExecutionStatus = "guestinfo.post_customization_script_execution_status"
	PostCustomizationScriptFailureReason   = "guestinfo.post_customization_script_execution_failure_reason"
)

var controlPlanePostCustPhases = []string{
	NetworkConfiguration,
	ProxyConfiguration,
	KubeadmInit,
	KubectlApplyCpi,
	KubectlApplyCsi,
	KubeadmTokenGenerate,
	KubectlApplyDefaultStorageClass,
}

var joinPostCustPhases = []string{
	NetworkConfiguration,
	KubeadmNodeJoin,
}

func removeFromSlice(remove string, arr []string) []string {
	for ind, str := range arr {
		if str == remove {
			return append(arr[:ind], arr[ind+1:]...)
		}
	}
	return arr
}

func strInSlice(findStr string, arr []string) bool {
	for _, str := range arr {
		if str == findStr {
			return true
		}
	}
	return false
}

const phaseSecondTimeout = 600

func redactCloudInit(cloudInitYaml string, path []string) (string, error) {
	yamlRunner := redact.NewYamlRedactor(strings.Join(path, "."), "", "cloudInitRedactor")
	outReader := yamlRunner.Redact(bytes.NewReader([]byte(cloudInitYaml)), "")
	gotBytes, err := ioutil.ReadAll(outReader)
	if err != nil {
		return cloudInitYaml, fmt.Errorf("failed to read redacted yaml output : %v", err)
	}
	return string(gotBytes), nil

}

func (r *VCDMachineReconciler) waitForPostCustomizationPhase(ctx context.Context,
	workloadVCDClient *vcdsdk.Client, vm *govcd.VM, phase string) error {
	log := ctrl.LoggerFrom(ctx)

	startTime := time.Now()
	possibleStatuses := []string{"", "in_progress", "successful"}
	currentStatus := possibleStatuses[0]
	vdcManager, err := vcdsdk.NewVDCManager(workloadVCDClient, workloadVCDClient.ClusterOrgName,
		workloadVCDClient.ClusterOVDCName)
	if err != nil {
		return errors.Wrapf(err, "failed to create a vdc manager object when waiting for post customization phase of VM")
	}
	for {
		if err := vm.Refresh(); err != nil {
			return errors.Wrapf(err, "unable to refresh vm [%s]: [%v]", vm.VM.Name, err)
		}
		newStatus, err := vdcManager.GetExtraConfigValue(vm, phase)
		if err != nil {
			return errors.Wrapf(err, "unable to get extra config value for key [%s] for vm: [%s]: [%v]",
				phase, vm.VM.Name, err)
		}
		log.Info("Obtained machine status ", "phase", phase, "status", newStatus)

		if !strInSlice(newStatus, possibleStatuses) {
			return errors.Wrapf(err, "invalid postcustomiation phase: [%s] for key [%s] for vm [%s]",
				newStatus, phase, vm.VM.Name)
		}
		if newStatus != currentStatus {
			possibleStatuses = removeFromSlice(currentStatus, possibleStatuses)
			currentStatus = newStatus
		}
		if newStatus == possibleStatuses[len(possibleStatuses)-1] { // successful status
			return nil
		}

		// catch intermediate script execution failure
		scriptExecutionStatus, err := vdcManager.GetExtraConfigValue(vm, PostCustomizationScriptExecutionStatus)
		if err != nil {
			return errors.Wrapf(err, "unable to get extra config value for key [%s] for vm: [%s]: [%v]",
				PostCustomizationScriptExecutionStatus, vm.VM.Name, err)
		}
		if scriptExecutionStatus != "" {
			execStatus, err := strconv.Atoi(scriptExecutionStatus)
			if err != nil {
				return errors.Wrapf(err, "unable to convert script execution status [%s] to int: [%v]",
					scriptExecutionStatus, err)
			}
			if execStatus != 0 {
				scriptExecutionFailureReason, err := vdcManager.GetExtraConfigValue(vm, PostCustomizationScriptFailureReason)
				if err != nil {
					return errors.Wrapf(err, "unable to get extra config value for key [%s] for vm, "+
						"(script execution status [%d]): [%s]: [%v]",
						PostCustomizationScriptFailureReason, execStatus, vm.VM.Name, err)
				}
				return fmt.Errorf("script failed with status [%d] and reason [%s]", execStatus, scriptExecutionFailureReason)
			}
		}

		if seconds := int(time.Since(startTime) / time.Second); seconds > phaseSecondTimeout {
			return fmt.Errorf("time for postcustomization status [%s] exceeded timeout [%d]",
				phase, phaseSecondTimeout)
		}
		time.Sleep(10 * time.Second)
	}

}

func (r *VCDMachineReconciler) reconcileNormal(ctx context.Context, cluster *clusterv1.Cluster,
	machine *clusterv1.Machine, vcdMachine *infrav1.VCDMachine, vcdCluster *infrav1.VCDCluster) (res ctrl.Result, retErr error) {

	log := ctrl.LoggerFrom(ctx, "machine", machine.Name, "cluster", vcdCluster.Name)

	userCreds, err := getUserCredentialsForCluster(ctx, r.Client, vcdCluster.Spec.UserCredentialsContext)
	if err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "Error getting client credentials to reconcile Cluster [%s] infrastructure", vcdCluster.Name)
	}
	workloadVCDClient, err := vcdsdk.NewVCDClientFromSecrets(vcdCluster.Spec.Site, vcdCluster.Spec.Org,
		vcdCluster.Spec.Ovdc, vcdCluster.Spec.Org, userCreds.Username, userCreds.Password, userCreds.RefreshToken, true, true)
	if err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "Unable to create VCD client to reconcile infrastructure for the Machine [%s]", machine.Name)
	}
	capvcdRdeManager := capisdk.NewCapvcdRdeManager(workloadVCDClient, vcdCluster.Status.InfraId)
	if vcdMachine.Spec.ProviderID != nil {
		vcdMachine.Status.Ready = true
		conditions.MarkTrue(vcdMachine, ContainerProvisionedCondition)
		err = capvcdRdeManager.AddToEventSet(ctx, capisdk.InfraVmBootstrapped, "", machine.Name, "")
		if err != nil {
			log.Error(err, "failed to add InfraVmBootstrapped event into RDE", "rdeID", vcdCluster.Status.InfraId)
		}
		return ctrl.Result{}, nil
	}

	if machine.Spec.Bootstrap.DataSecretName == nil {
		if !util.IsControlPlaneMachine(machine) && !conditions.IsTrue(cluster,
			clusterv1.ControlPlaneInitializedCondition) {

			log.Info("Waiting for the control plane to be initialized")
			conditions.MarkFalse(vcdMachine, ContainerProvisionedCondition,
				clusterv1.WaitingForControlPlaneAvailableReason, clusterv1.ConditionSeverityInfo, "")
			return ctrl.Result{}, nil
		}

		log.Info("Waiting for the Bootstrap provider controller to set bootstrap data")
		conditions.MarkFalse(vcdMachine, ContainerProvisionedCondition,
			WaitingForBootstrapDataReason, clusterv1.ConditionSeverityInfo, "")
		return ctrl.Result{}, nil
	}

	patchHelper, err := patch.NewHelper(vcdMachine, r.Client)
	if err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "Error patching VCDMachine [%s] of cluster [%s]", vcdMachine.Name, vcdCluster.Name)
	}
	conditions.MarkTrue(vcdMachine, ContainerProvisionedCondition)

	if !conditions.Has(vcdMachine, BootstrapExecSucceededCondition) {
		conditions.MarkFalse(vcdMachine, BootstrapExecSucceededCondition,
			BootstrappingReason, clusterv1.ConditionSeverityInfo, "")
		if err := patchVCDMachine(ctx, patchHelper, vcdMachine); err != nil {
			return ctrl.Result{}, errors.Wrapf(err, "Error patching VCDMachine [%s] of cluster [%s]", vcdMachine.Name, vcdCluster.Name)
		}
	}

	vdcManager, err := vcdsdk.NewVDCManager(workloadVCDClient, workloadVCDClient.ClusterOrgName,
		workloadVCDClient.ClusterOVDCName)
	if err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "failed to create a vdc manager object when reconciling machine [%s]", vcdMachine.Name)
	}

	// The vApp should have already been created, so this is more of a Get of the vApp
	vAppName := cluster.Name
	vApp, err := vdcManager.Vdc.GetVAppByName(vAppName, true)
	if err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "Error provisioning infrastructure for the machine [%s] of the cluster [%s]", machine.Name, vcdCluster.Name)
	}

	bootstrapJinjaScript, err := r.getBootstrapData(ctx, machine)
	if err != nil {
		err1 := capvcdRdeManager.AddToErrorSet(ctx, capisdk.ScriptGenerationError, "", machine.Name, fmt.Sprintf("%v", err))
		if err1 != nil {
			log.Error(err1, "failed to add ScriptGenerationError into RDE", "rdeID", vcdCluster.Status.InfraId)
		}
		return ctrl.Result{}, errors.Wrapf(err, "Error retrieving bootstrap data for machine [%s] of the cluster [%s]",
			machine.Name, vcdCluster.Name)
	}
	// In a multimaster cluster, the initial control plane node runs `kubeadm init`; additional control plane nodes
	// run `kubeadm join`. The joining control planes run `kubeadm join`, so these nodes use the join script.
	// Although it is sufficient to just check if `kubeadm join` is in the bootstrap script, using the
	// isControlPlaneMachine function is a simpler operation, so this function is called first.
	useControlPlaneScript := util.IsControlPlaneMachine(machine) && !strings.Contains(bootstrapJinjaScript, "kubeadm join")

	// Construct a CloudInitScriptInput struct to pass into template.Execute() function to generate the necessary
	// cloud init script for the relevant node type, i.e. control plane or worker node
	cloudInitInput := CloudInitScriptInput{}
	if !vcdMachine.Spec.Bootstrapped {
		if useControlPlaneScript {
			var (
				orgUserStr                = fmt.Sprintf("%s/%s", workloadVCDClient.VCDAuthConfig.UserOrg, workloadVCDClient.VCDAuthConfig.User)
				enableDefaultStorageClass = vcdCluster.Spec.DefaultStorageClassOptions.VCDStorageProfileName != ""
				k8sStorageClassName       = ""
				fileSystemFormat          = ""
				vcdStorageProfileName     = ""
				reclaimPolicy             = ReclaimPolicyRetain
			)
			if enableDefaultStorageClass {
				k8sStorageClassName = vcdCluster.Spec.DefaultStorageClassOptions.K8sStorageClassName
				if vcdCluster.Spec.DefaultStorageClassOptions.UseDeleteReclaimPolicy {
					reclaimPolicy = ReclaimPolicyDelete
				}
				fileSystemFormat = vcdCluster.Spec.DefaultStorageClassOptions.FileSystem
				vcdStorageProfileName = vcdCluster.Spec.DefaultStorageClassOptions.VCDStorageProfileName
			}

			cloudInitInput = CloudInitScriptInput{
				ControlPlane:              true,
				B64OrgUser:                b64.StdEncoding.EncodeToString([]byte(orgUserStr)),
				B64Password:               b64.StdEncoding.EncodeToString([]byte(vcdCluster.Spec.UserCredentialsContext.Password)),
				B64RefreshToken:           b64.StdEncoding.EncodeToString([]byte(vcdCluster.Spec.UserCredentialsContext.RefreshToken)),
				K8sStorageClassName:       k8sStorageClassName,
				ReclaimPolicy:             reclaimPolicy,
				VcdStorageProfileName:     vcdStorageProfileName,
				FileSystemFormat:          fileSystemFormat,
				CpiVersion:                r.Config.ClusterResources.CpiVersion,
				CsiVersion:                r.Config.ClusterResources.CsiVersion,
				VcdHostFormatted:          strings.Replace(vcdCluster.Spec.Site, "/", "\\/", -1),
				ClusterOrgName:            workloadVCDClient.ClusterOrgName,
				ClusterOVDCName:           workloadVCDClient.ClusterOVDCName,
				NetworkName:               vcdCluster.Spec.OvdcNetwork,
				VipSubnetCidr:             "", // vip subnet cidr - empty for now for CPI to select subnet
				VAppName:                  vAppName,
				ClusterID:                 vcdCluster.Status.InfraId,
				EnableDefaultStorageClass: strconv.FormatBool(enableDefaultStorageClass),
			}
		}
		cloudInitInput.HTTPSProxy = vcdCluster.Spec.ProxyConfig.HTTPProxy
		cloudInitInput.HTTPSProxy = vcdCluster.Spec.ProxyConfig.HTTPSProxy
		cloudInitInput.NoProxy = vcdCluster.Spec.ProxyConfig.NoProxy
		cloudInitInput.MachineName = machine.Name
		cloudInitInput.NvidiaGPU = vcdMachine.Spec.NvidiaGPU

	}

	mergedCloudInitBytes, err := MergeJinjaToCloudInitScript(cloudInitInput, bootstrapJinjaScript)
	if err != nil {
		err1 := capvcdRdeManager.AddToErrorSet(ctx, capisdk.ScriptGenerationError, "", machine.Name, fmt.Sprintf("%v", err))
		if err1 != nil {
			log.Error(err1, "failed to add ScriptGenerationError into RDE", "rdeID", vcdCluster.Status.InfraId)
		}
		return ctrl.Result{}, errors.Wrapf(err,
			"Error merging bootstrap jinja script with the cloudInit script for [%s/%s] [%s]",
			vAppName, machine.Name, bootstrapJinjaScript)
	}

	redactedCloudInit := string(mergedCloudInitBytes)
	if util.IsControlPlaneMachine(machine) {
		// redact secrets
		// NOTE: the position of the key in cluster_scripts/cloud_init is important as the following
		// code expects the secret to be the first element in write_files.
		redactedCloudInit, err = redactCloudInit(string(mergedCloudInitBytes), []string{"write_files", "0", "content"})
		if err != nil {
			log.Error(err, "failed to redact cloud init script")
		}
	}

	log.Info(fmt.Sprintf("Cloud init Script: [%s]", redactedCloudInit))
	err = capvcdRdeManager.AddToEventSet(ctx, capisdk.CloudInitScriptGenerated, "", machine.Name, "")
	if err != nil {
		log.Error(err, "failed to add CloudInitScriptGenerated event into RDE", "rdeID", vcdCluster.Status.InfraId)
	}
	err = capvcdRdeManager.RdeManager.RemoveErrorByNameOrIdFromErrorSet(ctx, vcdsdk.ComponentCAPVCD, capisdk.ScriptGenerationError, "", machine.Name)
	if err != nil {
		log.Error(err, "failed to remove ScriptGenerationError from RDE", "rdeID", vcdCluster.Status.InfraId)
	}

	vmExists := true
	vm, err := vApp.GetVMByName(machine.Name, true)
	if err != nil && err != govcd.ErrorEntityNotFound {
		return ctrl.Result{}, errors.Wrapf(err, "Error provisioning infrastructure for the machine; unable to query for VM [%s] in vApp [%s]",
			machine.Name, vAppName)
	} else if err == govcd.ErrorEntityNotFound {
		vmExists = false
	}
	if !vmExists {
		log.Info("Adding infra VM for the machine")
		err = vdcManager.AddNewVM(vcdCluster.Name, machine.Name, 1,
			vcdMachine.Spec.Catalog, vcdMachine.Spec.Template, vcdMachine.Spec.PlacementPolicy,
			vcdMachine.Spec.SizingPolicy, vcdMachine.Spec.StorageProfile, "", false)
		if err != nil {
			err1 := capvcdRdeManager.AddToErrorSet(ctx, capisdk.InfraVMCreationError, "", machine.Name, fmt.Sprintf("%v", err))
			if err1 != nil {
				log.Error(err1, "failed to add InfraVMCreationError into RDE", "rdeID", vcdCluster.Status.InfraId)
			}
			return ctrl.Result{}, errors.Wrapf(err, "Error provisioning infrastructure for the machine; unable to create VM [%s] in vApp [%s]",
				machine.Name, vApp.VApp.Name)
		}
		vm, err = vApp.GetVMByName(machine.Name, true)
		if err != nil {
			err1 := capvcdRdeManager.AddToErrorSet(ctx, capisdk.InfraVMCreationError, "", machine.Name, fmt.Sprintf("%v", err))
			if err1 != nil {
				log.Error(err1, "failed to add InfraVMCreationError into RDE", "rdeID", vcdCluster.Status.InfraId)
			}
			return ctrl.Result{}, errors.Wrapf(err, "Error provisioning infrastructure for the machine; unable to find newly created VM [%s] in vApp [%s]",
				vm.VM.Name, vAppName)
		}
		if vm == nil || vm.VM == nil {
			err1 := capvcdRdeManager.AddToErrorSet(ctx, capisdk.InfraVMCreationError, "", machine.Name, fmt.Sprintf("%v", err))
			if err1 != nil {
				log.Error(err1, "failed to add InfraVMCreationError into RDE", "rdeID", vcdCluster.Status.InfraId)
			}
			return ctrl.Result{}, errors.Wrapf(err, "Obtained nil VM after creating VM [%s]", machine.Name)
		}

		// NOTE: VMs are not added to VCDResourceSet intentionally as the VMs can be obtained from the VApp and
		// 	VCDResourceSet can get bloated with VMs if the cluster contains a large number of worker nodes
	}

	// set address in machine status
	if vm.VM == nil ||
		vm.VM.NetworkConnectionSection == nil ||
		len(vm.VM.NetworkConnectionSection.NetworkConnection) == 0 ||
		vm.VM.NetworkConnectionSection.NetworkConnection[0] == nil ||
		vm.VM.NetworkConnectionSection.NetworkConnection[0].IPAddress == "" {

		log.Error(nil, fmt.Sprintf("Requeuing...; failed to get the machine address of vm [%#v]", vm))
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}

	machineAddress := vm.VM.NetworkConnectionSection.NetworkConnection[0].IPAddress
	vcdMachine.Status.Addresses = []clusterv1.MachineAddress{
		{
			Type:    clusterv1.MachineHostName,
			Address: vm.VM.Name,
		},
		{
			Type:    clusterv1.MachineInternalIP,
			Address: machineAddress,
		},
		{
			Type:    clusterv1.MachineExternalIP,
			Address: machineAddress,
		},
	}

	gateway, err := vcdsdk.NewGatewayManager(ctx, workloadVCDClient, vcdCluster.Spec.OvdcNetwork, r.Config.VCD.VIPSubnet)
	if err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "failed to create gateway manager object while reconciling machine [%s]", vcdMachine.Name)
	}

	// Update loadbalancer pool with the IP of the control plane node as a new member.
	// Note that this must be done before booting on the VM!
	if util.IsControlPlaneMachine(machine) {
		virtualServiceName := capisdk.GetVirtualServiceNameUsingPrefix(
			capisdk.GetVirtualServiceNamePrefix(vcdCluster.Name, vcdCluster.Status.InfraId), "tcp")
		lbPoolName := capisdk.GetLoadBalancerPoolNameUsingPrefix(
			capisdk.GetLoadBalancerPoolNamePrefix(vcdCluster.Name, vcdCluster.Status.InfraId), "tcp")
		lbPoolRef, err := gateway.GetLoadBalancerPool(ctx, lbPoolName)
		if err != nil {
			return ctrl.Result{}, errors.Wrapf(err, "Error retrieving/updating load balancer pool [%s] for the "+
				"control plane machine [%s] of the cluster [%s]", lbPoolName, machine.Name, vcdCluster.Name)
		}
		controlPlaneIPs, err := gateway.GetLoadBalancerPoolMemberIPs(ctx, lbPoolRef)
		if err != nil {
			return ctrl.Result{}, errors.Wrapf(err,
				"Error retrieving/updating load balancer pool members [%s] for the "+
					"control plane machine [%s] of the cluster [%s]", lbPoolName, machine.Name, vcdCluster.Name)
		}

		updatedIPs := append(controlPlaneIPs, machineAddress)
		updatedUniqueIPs := cpiutil.NewSet(updatedIPs).GetElements()
		resourcesAllocated := &cpiutil.AllocatedResourcesMap{}
		var oneArm *vcdsdk.OneArm = nil
		if vcdCluster.Spec.LoadBalancer.UseOneArm {
			oneArm = &vcdsdk.OneArm{
				StartIP: r.Config.LB.OneArm.StartIP,
				EndIP:   r.Config.LB.OneArm.EndIP,
			}
		}
		// At this point the vcdCluster.Spec.ControlPlaneEndpoint should have been set correctly.
		_, err = gateway.UpdateLoadBalancer(ctx, lbPoolName, virtualServiceName, updatedUniqueIPs,
			int32(vcdCluster.Spec.ControlPlaneEndpoint.Port), int32(vcdCluster.Spec.ControlPlaneEndpoint.Port),
			oneArm, !vcdCluster.Spec.LoadBalancer.UseOneArm, resourcesAllocated)
		if err != nil {
			return ctrl.Result{}, errors.Wrapf(err,
				"Error updating the load balancer pool [%s] for the "+
					"control plane machine [%s] of the cluster [%s]", lbPoolName, machine.Name, vcdCluster.Name)
		}
		log.Info("Updated the load balancer pool with the control plane machine IP",
			"lbpool", lbPoolName)
	}

	// only resize hard disk if the user has requested so by specifying such in the VCDMachineTemplate spec
	// check isn't strictly required as we ensure that specified number is larger than what's in the template and left
	// empty this will just be 0. However, this makes it clear from a standpoint of inspecting the code what we are doing
	if !vcdMachine.Spec.DiskSize.IsZero() {
		// go-vcd expects value in MB (2^10 = 1024 * 1024 bytes), so we scale it as such
		diskSize, ok := vcdMachine.Spec.DiskSize.AsInt64()
		if !ok {
			return ctrl.Result{},
				fmt.Errorf("error while provisioning the infrastructure VM for the machine [%s] of the cluster [%s]; failed to parse disk size quantity [%s]", vm.VM.Name, vApp.VApp.Name, vcdMachine.Spec.DiskSize.String())
		}
		diskSize = int64(math.Floor(float64(diskSize) / float64(Mebibyte)))
		diskSettings := vm.VM.VmSpecSection.DiskSection.DiskSettings
		// if the specified disk size is less than what is defined in the template, then we ignore the field
		if len(diskSettings) != 0 && diskSettings[0].SizeMb < diskSize {
			log.Info(
				fmt.Sprintf("resizing hard disk on VM for machine [%s] of cluster [%s]; resizing from [%dMB] to [%dMB]",
					vm.VM.Name, vApp.VApp.Name, diskSettings[0].SizeMb, diskSize))

			diskSettings[0].SizeMb = diskSize
			vm.VM.VmSpecSection.DiskSection.DiskSettings = diskSettings

			if _, err = vm.UpdateInternalDisks(vm.VM.VmSpecSection); err != nil {
				return ctrl.Result{},
					errors.Wrapf(err, "Error while provisioning the infrastructure VM for the machine [%s] of the cluster [%s]; failed to resize hard disk", vm.VM.Name, vApp.VApp.Name)
			}
		}
	}

	vmStatus, err := vm.GetStatus()
	if err != nil {
		err1 := capvcdRdeManager.AddToErrorSet(ctx, capisdk.InfraVMCreationError, "", machine.Name, fmt.Sprintf("%v", err))
		if err1 != nil {
			log.Error(err1, "failed to add InfraVMCreationError into RDE", "rdeID", vcdCluster.Status.InfraId)
		}
		return ctrl.Result{},
			errors.Wrapf(err, "Error while provisioning the infrastructure VM for the machine [%s] of the cluster [%s]; failed to get status of vm", vm.VM.Name, vApp.VApp.Name)
	}

	if vmStatus != "POWERED_ON" {
		// try to power on the VM
		b64CloudInitScript := b64.StdEncoding.EncodeToString(mergedCloudInitBytes)
		keyVals := map[string]string{
			"guestinfo.userdata":          b64CloudInitScript,
			"guestinfo.userdata.encoding": "base64",
			"disk.enableUUID":             "1",
		}

		for key, val := range keyVals {
			err = vdcManager.SetVmExtraConfigKeyValue(vm, key, val, true)
			if err != nil {
				return ctrl.Result{}, errors.Wrapf(err, "Error while enabling cloudinit on the machine [%s/%s]; unable to set vm extra config key [%s] for vm ",
					vcdCluster.Name, vm.VM.Name, key)
			}

			if err = vm.Refresh(); err != nil {
				return ctrl.Result{}, errors.Wrapf(err, "Error while enabling cloudinit on the machine [%s/%s]; unable to refresh vm", vcdCluster.Name, vm.VM.Name)
			}

			if err = vApp.Refresh(); err != nil {
				return ctrl.Result{}, errors.Wrapf(err, "Error while enabling cloudinit on the machine [%s/%s]; unable to refresh vapp", vAppName, vm.VM.Name)
			}

			log.Info(fmt.Sprintf("Configured the infra machine with variable [%s] to enable cloud-init", key))
		}

		task, err := vm.PowerOn()
		if err != nil {
			err1 := capvcdRdeManager.AddToErrorSet(ctx, capisdk.InfraVMCreationError, "", machine.Name, fmt.Sprintf("%v", err))
			if err1 != nil {
				log.Error(err1, "failed to add InfraVMCreationError into RDE", "rdeID", vcdCluster.Status.InfraId)
			}
			return ctrl.Result{}, errors.Wrapf(err, "Error while deploying infra for the machine [%s/%s]; unable to power on VM", vcdCluster.Name, vm.VM.Name)
		}
		if err = task.WaitTaskCompletion(); err != nil {
			err1 := capvcdRdeManager.AddToErrorSet(ctx, capisdk.InfraVMCreationError, "", machine.Name, fmt.Sprintf("%v", err))
			if err1 != nil {
				log.Error(err1, "failed to add InfraVMCreationError into RDE", "rdeID", vcdCluster.Status.InfraId)
			}
			return ctrl.Result{}, errors.Wrapf(err, "Error while deploying infra for the machine [%s/%s]; error waiting for VM power-on task completion", vcdCluster.Name, vm.VM.Name)
		}

		if err = vApp.Refresh(); err != nil {
			err1 := capvcdRdeManager.AddToErrorSet(ctx, capisdk.InfraVMCreationError, "", machine.Name, fmt.Sprintf("%v", err))
			if err1 != nil {
				log.Error(err1, "failed to add InfraVMCreationError into RDE", "rdeID", vcdCluster.Status.InfraId)
			}
			return ctrl.Result{}, errors.Wrapf(err, "Error while deploying infra for the machine [%s/%s]; unable to refresh vapp after VM power-on", vAppName, vm.VM.Name)
		}
	}
	if hasCloudInitFailedBefore, err := r.hasCloudInitExecutionFailedBefore(ctx, workloadVCDClient, vm); hasCloudInitFailedBefore {
		err1 := capvcdRdeManager.AddToErrorSet(ctx, capisdk.ScriptExecutionError, "", machine.Name, fmt.Sprintf("%v", err))
		if err1 != nil {
			log.Error(err1, "failed to add ScriptExecutionError into RDE", "rdeID", vcdCluster.Status.InfraId)
		}
		return ctrl.Result{}, errors.Wrapf(err, "Error bootstrapping the machine [%s/%s]; machine is probably in unreconciliable state", vAppName, vm.VM.Name)
	}
	err = capvcdRdeManager.AddToEventSet(ctx, capisdk.InfraVmPoweredOn, "", machine.Name, "")
	if err != nil {
		log.Error(err, "failed to add InfraVmPoweredOn event into RDE", "rdeID", vcdCluster.Status.InfraId)
	}
	err = capvcdRdeManager.RdeManager.RemoveErrorByNameOrIdFromErrorSet(ctx, vcdsdk.ComponentCAPVCD, capisdk.InfraVMCreationError, "", machine.Name)
	if err != nil {
		log.Error(err, "failed to remove InfraVMCreationError from RDE", "rdeID", vcdCluster.Status.InfraId)
	}
	// wait for each vm phase
	phases := controlPlanePostCustPhases
	if !useControlPlaneScript {
		if vcdMachine.Spec.NvidiaGPU {
			phases = []string{joinPostCustPhases[0], NvidiaRuntimeInstall, joinPostCustPhases[1], NvidiaContainerdConfiguration}
		} else {
			phases = joinPostCustPhases
		}
	}
	for _, phase := range phases {
		if err = vApp.Refresh(); err != nil {
			return ctrl.Result{},
				errors.Wrapf(err, "Error while bootstrapping the machine [%s/%s]; unable to refresh vapp",
					vAppName, vm.VM.Name)
		}
		log.Info(fmt.Sprintf("Start: waiting for the bootstrapping phase [%s] to complete", phase))
		if err = r.waitForPostCustomizationPhase(ctx, workloadVCDClient, vm, phase); err != nil {
			log.Error(err, fmt.Sprintf("Error waiting for the bootstrapping phase [%s] to complete", phase))
			err1 := capvcdRdeManager.AddToErrorSet(ctx, capisdk.ScriptExecutionError, "", machine.Name, fmt.Sprintf("%v", err))
			if err1 != nil {
				log.Error(err1, "failed to add ScriptExecutionError into RDE", "rdeID", vcdCluster.Status.InfraId)
			}
			return ctrl.Result{}, errors.Wrapf(err, "Error while bootstrapping the machine [%s/%s]; unable to wait for post customization phase [%s]",
				vAppName, vm.VM.Name, phase)
		}
		log.Info(fmt.Sprintf("End: waiting for the bootstrapping phase [%s] to complete", phase))
	}

	log.Info("Successfully bootstrapped the machine")
	err = capvcdRdeManager.AddToEventSet(ctx, capisdk.InfraVmBootstrapped, "", machine.Name, "")
	if err != nil {
		log.Error(err, "failed to add InfraVmBootstrapped event into RDE", "rdeID", vcdCluster.Status.InfraId)
	}

	if err = vm.Refresh(); err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "Unexpected error after the machine [%s/%s] is bootstrapped; unable to refresh vm", vAppName, vm.VM.Name)
	}
	if err = vApp.Refresh(); err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "Unexpected error after the machine [%s/%s] is bootstrapped; unable to refresh vapp", vAppName, vm.VM.Name)
	}

	vcdMachine.Spec.Bootstrapped = true
	conditions.MarkTrue(vcdMachine, BootstrapExecSucceededCondition)

	// Set ProviderID so the Cluster API Machine Controller can pull it
	providerID := fmt.Sprintf("%s://%s", infrav1.VCDProviderID, vm.VM.ID)
	vcdMachine.Spec.ProviderID = &providerID
	vcdMachine.Status.Ready = true
	vcdMachine.Status.Template = vcdMachine.Spec.Template
	vcdMachine.Status.ProviderID = vcdMachine.Spec.ProviderID
	conditions.MarkTrue(vcdMachine, ContainerProvisionedCondition)

	return ctrl.Result{}, nil
}

func (r *VCDMachineReconciler) getBootstrapData(ctx context.Context, machine *clusterv1.Machine) (string, error) {
	log := ctrl.LoggerFrom(ctx)
	if machine.Spec.Bootstrap.DataSecretName == nil {
		return "", errors.New("error retrieving bootstrap data: linked Machine's bootstrap.dataSecretName is nil")
	}

	s := &corev1.Secret{}
	key := client.ObjectKey{Namespace: machine.GetNamespace(), Name: *machine.Spec.Bootstrap.DataSecretName}
	if err := r.Client.Get(ctx, key, s); err != nil {
		return "", errors.Wrapf(err, "failed to retrieve bootstrap data secret for VCDMachine %s/%s", machine.GetNamespace(), machine.GetName())
	}

	value, ok := s.Data["value"]
	if !ok {
		return "", errors.New("error retrieving bootstrap data: secret value key is missing")
	}

	log.Info(fmt.Sprintf("Auto-generated bootstrap script: [%s]", string(value)))

	return string(value), nil
}

func (r *VCDMachineReconciler) reconcileDelete(ctx context.Context, cluster *clusterv1.Cluster, machine *clusterv1.Machine,
	vcdMachine *infrav1.VCDMachine, vcdCluster *infrav1.VCDCluster) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx, "machine", machine.Name, "cluster", vcdCluster.Name)

	patchHelper, err := patch.NewHelper(vcdMachine, r.Client)
	if err != nil {
		return ctrl.Result{}, err
	}

	conditions.MarkFalse(vcdMachine, ContainerProvisionedCondition,
		clusterv1.DeletingReason, clusterv1.ConditionSeverityInfo, "")
	if err := patchVCDMachine(ctx, patchHelper, vcdMachine); err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "Failed to patch VCDMachine [%s/%s]", vcdCluster.Name, vcdMachine.Name)
	}

	if vcdCluster.Spec.Site == "" {
		controllerutil.RemoveFinalizer(vcdMachine, infrav1.MachineFinalizer)
		return ctrl.Result{}, nil
	}

	userCreds, err := getUserCredentialsForCluster(ctx, r.Client, vcdCluster.Spec.UserCredentialsContext)
	if err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "Error getting client credentials to reconcile Cluster [%s] infrastructure", vcdCluster.Name)
	}
	workloadVCDClient, err := vcdsdk.NewVCDClientFromSecrets(vcdCluster.Spec.Site, vcdCluster.Spec.Org,
		vcdCluster.Spec.Ovdc, vcdCluster.Spec.Org, userCreds.Username, userCreds.Password, userCreds.RefreshToken, true, true)
	capvcdRdeManager := capisdk.NewCapvcdRdeManager(workloadVCDClient, vcdCluster.Status.InfraId)
	if err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "Unable to create VCD client to reconcile infrastructure for the Machine [%s]", machine.Name)
	}

	gateway, err := vcdsdk.NewGatewayManager(ctx, workloadVCDClient, vcdCluster.Spec.OvdcNetwork, r.Config.VCD.VIPSubnet)
	if err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "failed to create gateway manager object while reconciling machine [%s]", vcdMachine.Name)
	}

	if util.IsControlPlaneMachine(machine) {
		// remove the address from the lbpool
		log.Info("Deleting the control plane IP from the load balancer pool")
		lbPoolName := capisdk.GetLoadBalancerPoolNameUsingPrefix(
			capisdk.GetLoadBalancerPoolNamePrefix(vcdCluster.Name, vcdCluster.Status.InfraId), "tcp")
		virtualServiceName := capisdk.GetVirtualServiceNameUsingPrefix(
			capisdk.GetVirtualServiceNamePrefix(vcdCluster.Name, vcdCluster.Status.InfraId), "tcp")
		lbPoolRef, err := gateway.GetLoadBalancerPool(ctx, lbPoolName)
		if err != nil && err != govcd.ErrorEntityNotFound {
			return ctrl.Result{}, errors.Wrapf(err, "Error while deleting the infra resources of the machine [%s/%s]; failed to get load balancer pool [%s]", vcdCluster.Name, vcdMachine.Name, lbPoolName)
		}
		// Do not try to update the load balancer if lbPool is not found
		if err != govcd.ErrorEntityNotFound {
			controlPlaneIPs, err := gateway.GetLoadBalancerPoolMemberIPs(ctx, lbPoolRef)
			if err != nil {
				return ctrl.Result{}, errors.Wrapf(err,
					"Error while deleting the infra resources of the machine [%s/%s]; failed to retrieve members from the load balancer pool [%s]",
					vcdCluster.Name, vcdMachine.Name, lbPoolName)
			}
			addresses := vcdMachine.Status.Addresses
			addressToBeDeleted := ""
			for _, address := range addresses {
				if address.Type == clusterv1.MachineInternalIP {
					addressToBeDeleted = address.Address
				}
			}
			updatedIPs := controlPlaneIPs
			for i, IP := range controlPlaneIPs {
				if IP == addressToBeDeleted {
					updatedIPs = append(controlPlaneIPs[:i], controlPlaneIPs[i+1:]...)
				}
			}
			resourcesAllocated := &cpiutil.AllocatedResourcesMap{}
			var oneArm *vcdsdk.OneArm = nil
			if vcdCluster.Spec.LoadBalancer.UseOneArm {
				oneArm = &vcdsdk.OneArm{
					StartIP: r.Config.LB.OneArm.StartIP,
					EndIP:   r.Config.LB.OneArm.EndIP,
				}
			}
			// At this point the vcdCluster.Spec.ControlPlaneEndpoint should have been set correctly.
			_, err = gateway.UpdateLoadBalancer(ctx, lbPoolName, virtualServiceName, updatedIPs,
				int32(vcdCluster.Spec.ControlPlaneEndpoint.Port), int32(vcdCluster.Spec.ControlPlaneEndpoint.Port),
				oneArm, !vcdCluster.Spec.LoadBalancer.UseOneArm, resourcesAllocated)
			if err != nil {
				return ctrl.Result{}, errors.Wrapf(err,
					"Error while deleting the infra resources of the machine [%s/%s]; error deleting the control plane from the load balancer pool [%s]",
					vcdCluster.Name, vcdMachine.Name, lbPoolName)
			}
		}
	}

	vdcManager, err := vcdsdk.NewVDCManager(workloadVCDClient, workloadVCDClient.ClusterOrgName,
		workloadVCDClient.ClusterOVDCName)
	if err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "failed to create a vdc manager object when reconciling machine [%s]", vcdMachine.Name)
	}

	// get the vApp
	vAppName := cluster.Name
	vApp, err := vdcManager.Vdc.GetVAppByName(vAppName, true)
	if err != nil {
		if err == govcd.ErrorEntityNotFound {
			log.Error(err, "Error while deleting the machine; vApp not found")
		} else {
			return ctrl.Result{}, errors.Wrapf(err, "Error while deleting the machine [%s/%s]; failed to find vapp by name", vAppName, machine.Name)
		}
	}
	if vApp != nil {
		// Delete the VM if and only if rdeId (matches) present in the vApp
		if !vcdCluster.Status.VAppMetadataUpdated {
			return ctrl.Result{}, errors.Errorf("Error occurred during the machine deletion; Metadata not found in vApp")
		}
		metadataInfraId, err := vdcManager.GetMetadataByKey(vApp, CapvcdInfraId)
		if err != nil {
			return ctrl.Result{}, errors.Errorf("Error occurred during fetching metadata in vApp")
		}
		// checking the metadata value and vcdCluster.Status.InfraId are equal or not
		if metadataInfraId != vcdCluster.Status.InfraId {
			return ctrl.Result{}, errors.Wrapf(err,
				"Error occurred during the machine deletion; failed to delete vApp [%s]", vcdCluster.Name)
		}
		// delete the vm
		vm, err := vApp.GetVMByName(machine.Name, true)
		if err != nil {
			if err == govcd.ErrorEntityNotFound {
				log.Error(err, "Error while deleting the machine; VM  not found")
			} else {
				return ctrl.Result{}, errors.Wrapf(err, "Error while deleting the machine [%s/%s]; unable to check if vm exists in vapp", vAppName, machine.Name)
			}
		}
		if vm != nil {
			// check if there are any disks attached to the VM
			if vm.VM.VmSpecSection != nil && vm.VM.VmSpecSection.DiskSection != nil {
				for _, diskSettings := range vm.VM.VmSpecSection.DiskSection.DiskSettings {
					if diskSettings.Disk != nil {
						log.Info("Cannot delete VM until named disk is detached from VM (by CSI)",
							"vm", vm.VM.Name, "disk", diskSettings.Disk.Name)
						err1 := capvcdRdeManager.AddToErrorSet(ctx, capisdk.InfraVmDeleteError, "", machine.Name, fmt.Sprintf("%v", err))
						if err1 != nil {
							log.Error(err1, "failed to add InfraVmDeleteError into RDE", "rdeID", vcdCluster.Status.InfraId)
						}
						return ctrl.Result{}, fmt.Errorf(
							"error deleting VM [%s] since named disk [%s] is attached to VM (by CSI)",
							vm.VM.Name, diskSettings.Disk.Name)
					}
				}
			}

			// power-off the VM if it is powered on
			vmStatus, err := vm.GetStatus()
			if err != nil {
				klog.Warningf("Unable to get VM status for VM [%s]: [%v]", vm.VM.Name, err)
			} else {
				// continue and try to power-off in any case
				klog.Infof("VM [%s] has status [%s]", vm.VM.Name, vmStatus)
				task, err := vm.PowerOff()
				if err != nil {
					klog.Warningf("Error while powering off VM [%s]: [%v]", vm.VM.Name, err)
				} else {
					if err = task.WaitTaskCompletion(); err != nil {
						err1 := capvcdRdeManager.AddToErrorSet(ctx, capisdk.InfraVmDeleteError, "", machine.Name, fmt.Sprintf("%v", err))
						if err1 != nil {
							log.Error(err1, "failed to add InfraVmDeleteError into RDE", "rdeID", vcdCluster.Status.InfraId)
						}
						return ctrl.Result{}, fmt.Errorf("error waiting for task completion after reconfiguring vm: [%v]", err)
					}
				}
			}

			// in any case try to delete the machine
			log.Info("Deleting the infra VM of the machine")
			if err := vm.Delete(); err != nil {
				err1 := capvcdRdeManager.AddToErrorSet(ctx, capisdk.InfraVmDeleteError, "", machine.Name, fmt.Sprintf("%v", err))
				if err1 != nil {
					log.Error(err1, "failed to add InfraVmDeleteError into RDE", "rdeID", vcdCluster.Status.InfraId)
				}
				return ctrl.Result{}, errors.Wrapf(err, "error deleting the machine [%s/%s]", vAppName, vm.VM.Name)
			}
		}
		log.Info("Successfully deleted infra resources of the machine")
		err = capvcdRdeManager.AddToEventSet(ctx, capisdk.InfraVmDeleted, "", machine.Name, "")
		if err != nil {
			log.Error(err, "failed to add InfraVmDeleted event into RDE", "rdeID", vcdCluster.Status.InfraId)
		}
		err = capvcdRdeManager.RdeManager.RemoveErrorByNameOrIdFromErrorSet(ctx, vcdsdk.ComponentCAPVCD, capisdk.InfraVmDeleteError, "", machine.Name)
		if err != nil {
			log.Error(err, "failed to remove InfraVmDeleteError from RDE", "rdeID", vcdCluster.Status.InfraId)
		}
	}
	// Remove VM from VCDResourceSet of RDE
	rdeManager := vcdsdk.NewRDEManager(workloadVCDClient, vcdCluster.Status.InfraId, capisdk.StatusComponentNameCAPVCD, release.CAPVCDVersion)
	err = rdeManager.RemoveFromVCDResourceSet(ctx, vcdsdk.ComponentCAPVCD, VcdResourceTypeVM, machine.Name)
	if err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "failed to delete VCD Resource [%s] of type [%s] from VCDResourceSet of RDE [%s]: [%v]",
			machine.Name, VcdResourceTypeVM, vcdCluster.Status.InfraId, err)
	}

	controllerutil.RemoveFinalizer(vcdMachine, infrav1.MachineFinalizer)
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *VCDMachineReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager,
	options controller.Options) error {
	clusterToVCDMachines, err := util.ClusterToObjectsMapper(mgr.GetClient(),
		&infrav1.VCDMachineList{}, mgr.GetScheme())
	if err != nil {
		return err
	}

	c, err := ctrl.NewControllerManagedBy(mgr).
		For(&infrav1.VCDMachine{}).
		WithOptions(options).
		WithEventFilter(predicates.ResourceNotPaused(ctrl.LoggerFrom(ctx))).
		Watches(
			&source.Kind{Type: &clusterv1.Machine{}},
			handler.EnqueueRequestsFromMapFunc(
				util.MachineToInfrastructureMapFunc(infrav1.GroupVersion.WithKind("VCDMachine"))),
		).
		Watches(
			&source.Kind{Type: &infrav1.VCDCluster{}},
			handler.EnqueueRequestsFromMapFunc(r.VCDClusterToVCDMachines),
		).
		Build(r)
	if err != nil {
		return err
	}
	return c.Watch(
		&source.Kind{Type: &clusterv1.Cluster{}},
		handler.EnqueueRequestsFromMapFunc(clusterToVCDMachines),
		predicates.ClusterUnpausedAndInfrastructureReady(ctrl.LoggerFrom(ctx)),
	)
}

// VCDClusterToVCDMachines is a handler.ToRequestsFunc to be used to enqueue
// requests for reconciliation of VCDMachines.
func (r *VCDMachineReconciler) VCDClusterToVCDMachines(o client.Object) []ctrl.Request {
	var result []ctrl.Request
	c, ok := o.(*infrav1.VCDCluster)
	if !ok {
		klog.Errorf("Expected a VCDCluster found [%T]", o)
		return nil
	}

	cluster, err := util.GetOwnerCluster(context.TODO(), r.Client, c.ObjectMeta)
	switch {
	case apierrors.IsNotFound(err) || cluster == nil:
		return result
	case err != nil:
		return result
	}

	labels := map[string]string{clusterv1.ClusterLabelName: cluster.Name}
	machineList := &clusterv1.MachineList{}
	if err := r.Client.List(context.TODO(), machineList, client.InNamespace(c.Namespace),
		client.MatchingLabels(labels)); err != nil {
		return nil
	}
	for _, m := range machineList.Items {
		if m.Spec.InfrastructureRef.Name == "" {
			continue
		}
		name := client.ObjectKey{Namespace: m.Namespace, Name: m.Name}
		result = append(result, ctrl.Request{NamespacedName: name})
	}

	return result
}
func (r *VCDMachineReconciler) hasCloudInitExecutionFailedBefore(ctx context.Context,
	workloadVCDClient *vcdsdk.Client, vm *govcd.VM) (bool, error) {
	vdcManager, err := vcdsdk.NewVDCManager(workloadVCDClient, workloadVCDClient.ClusterOrgName,
		workloadVCDClient.ClusterOVDCName)
	if err != nil {
		return false, errors.Wrapf(err, "failed to create a vdc manager object while checking cloud init failures")
	}
	scriptExecutionStatus, err := vdcManager.GetExtraConfigValue(vm, PostCustomizationScriptExecutionStatus)
	if err != nil {
		return false, errors.Wrapf(err, "unable to get extra config value for key [%s] for vm: [%s]: [%v]",
			PostCustomizationScriptExecutionStatus, vm.VM.Name, err)
	}
	if scriptExecutionStatus != "" {
		execStatus, err := strconv.Atoi(scriptExecutionStatus)
		if err != nil {
			return false, errors.Wrapf(err, "unable to convert script execution status [%s] to int: [%v]",
				scriptExecutionStatus, err)
		}
		if execStatus != 0 {
			scriptExecutionFailureReason, err := vdcManager.GetExtraConfigValue(vm, PostCustomizationScriptFailureReason)
			if err != nil {
				return false, errors.Wrapf(err, "unable to get extra config value for key [%s] for vm, "+
					"(script execution status [%d]): [%s]: [%v]",
					PostCustomizationScriptFailureReason, execStatus, vm.VM.Name, err)
			}
			return true, fmt.Errorf("script failed with status [%d] and reason [%s]", execStatus, scriptExecutionFailureReason)
		}
	}
	return false, nil
}

// MergeJinjaToCloudInitScript : merges the cloud init config with a jinja config and adds a
// `#cloudconfig` header. Does a couple of special handling: takes jinja's runcmd and embeds
// it into a fixed location in the cloudInitConfig. Returns the merged bytes or nil and error.
func MergeJinjaToCloudInitScript(cloudInitConfig CloudInitScriptInput, jinjaConfig string) ([]byte, error) {
	jinja := make(map[string]interface{})
	err := yaml.Unmarshal([]byte(jinjaConfig), &jinja)
	if err != nil {
		return nil, fmt.Errorf("unable to unmarshal yaml [%s]: [%v]", jinjaConfig, err)
	}

	// handle runcmd before parsing vcd cloud init yaml all to simplify things
	cloudInitModified := ""
	jinjaRunCmd, ok := jinja["runcmd"]
	if ok {
		jinjaLines, ok := jinjaRunCmd.([]interface{})
		if !ok {
			return nil, fmt.Errorf("expected []interface{}, found [%T] for jinja runcmd [%v]",
				jinjaRunCmd, jinjaRunCmd)
		}

		formattedJinjaCmd := "\n"
		indent := strings.Repeat(" ", 4)
		for _, jinjaLine := range jinjaLines {
			jinjaLineStr, ok := jinjaLine.(string)
			if !ok {
				return nil, fmt.Errorf("unable to convert [%#v] to string", jinjaLineStr)
			}
			formattedJinjaCmd += indent + jinjaLineStr + "\n"
		}
		cloudInitConfig.BootstrapRunCmd = strings.Trim(strings.Trim(formattedJinjaCmd, "\n"), "\r\n")
	}
	cloudInitTemplate := template.New("cloud_init_script_template")

	if cloudInitTemplate, err = cloudInitTemplate.Parse(cloudInitScriptTemplate); err != nil {
		return nil, errors.Wrapf(err, "Error parsing cloudInitScriptTemplate [%s]", cloudInitTemplate.Name())
	}
	buff := bytes.Buffer{}
	if err = cloudInitTemplate.Execute(&buff, cloudInitConfig); err != nil {
		return nil, errors.Wrapf(err, "Error rendering cloud init template: [%s]", cloudInitTemplate.Name())
	}
	cloudInit := buff.String()

	vcdCloudInit := make(map[string]interface{})
	if err := yaml.Unmarshal([]byte(cloudInit), &vcdCloudInit); err != nil {
		return nil, fmt.Errorf("unable to unmarshal cloud init with embedded jinja script: [%v]: [%v]",
			cloudInitModified, err)
	}

	mergedCloudInit := make(map[string]interface{})
	for key, vcdVal := range vcdCloudInit {
		jinjaVal, ok := jinja[key]
		if !ok || key == "runcmd" {
			mergedCloudInit[key] = vcdVal
			continue
		}

		switch vcdVal.(type) {
		case []interface{}:
			mergedCloudInit[key] = append(vcdVal.([]interface{}), jinjaVal.([]interface{})...)
		default:
			return nil, fmt.Errorf("unable to handle type [%T] for key [%v]", vcdVal, key)
		}
	}

	// consume the remaining keys not used in VCD
	for key, jinjaVal := range jinja {
		if _, ok := vcdCloudInit[key]; !ok {
			mergedCloudInit[key] = jinjaVal
			continue
		}
	}

	out := []byte("#cloud-config\n")
	for _, key := range []string{
		"write_files",
		"runcmd",
		"users",
		"timezone",
		"disable_root",
		"preserve_hostname",
		"hostname",
		"final_message",
	} {
		val, ok := mergedCloudInit[key]
		if !ok {
			continue
		}

		deltaMap := map[string]interface{}{
			key: val,
		}
		delta, err := yaml.Marshal(deltaMap)
		if err != nil {
			return nil, fmt.Errorf("unable to marshal [%#v]", deltaMap)
		}

		out = append(out, delta...)
	}

	return out, nil
}
