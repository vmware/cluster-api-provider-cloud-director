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
	"math"
	"math/rand"
	"net"
	"reflect"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/Masterminds/sprig/v3"
	"github.com/go-logr/logr"

	"github.com/pkg/errors"
	cpiutil "github.com/vmware/cloud-provider-for-cloud-director/pkg/util"
	"github.com/vmware/cloud-provider-for-cloud-director/pkg/vcdsdk"
	infrav1beta3 "github.com/vmware/cluster-api-provider-cloud-director/api/v1beta3"
	"github.com/vmware/cluster-api-provider-cloud-director/pkg/capisdk"
	capvcdutil "github.com/vmware/cluster-api-provider-cloud-director/pkg/util"
	"github.com/vmware/cluster-api-provider-cloud-director/release"
	"github.com/vmware/go-vcloud-director/v2/govcd"
	"github.com/vmware/go-vcloud-director/v2/types/v56"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/klog"
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
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
	"sigs.k8s.io/yaml"
)

type CloudInitScriptInput struct {
	ControlPlane        bool   // control plane node
	NvidiaGPU           bool   // configure containerd for NVIDIA libraries
	BootstrapRunCmd     string // bootstrap run command
	HTTPProxy           string // httpProxy endpoint
	HTTPSProxy          string // httpsProxy endpoint
	NoProxy             string // no proxy values
	MachineName         string // vm host name
	ResizedControlPlane bool   // resized node type: worker | control_plane
	VcdHostFormatted    string // vcd host
	TKGVersion          string // tkgVersion
	ClusterID           string //cluster id
}

type IgnitionNetworkInitScriptSectionInput struct {
	Primary     bool
	Network     string
	IPAddress   string
	MACAddress  string
	NetmaskCidr int
	Gateway     string
	DNS1        string
	DNS2        string
}

const (
	VcdResourceTypeVM = "virtual-machine"
)

const (
	BootstrapFormatCloudConfig = "cloud-config"
	BootstrapFormatIgnition    = "ignition"
)

const Mebibyte = 1048576

// The following `embed` directives read the file in the mentioned path and copy the content into the declared variable.
// These variables need to be global within the package.
//
//go:embed cluster_scripts/cloud_init.tmpl
var cloudInitScriptTemplate string

//go:embed cluster_scripts/ignition_network_init_script.tmpl
var ignitionNetworkInitScriptTemplate string

type VCDMachineReconcilerParams struct {
	// UseNormalVms
	// in third party installations needs to use 'Normal' vmware vms instead of TGK vms
	UseNormalVms bool
	// ResizeDiskBeforeNetworkReconciliation
	// When vm network will bootstrap by cloud init (in DHCP mode) it be able to bootstrap slowly
	// because vm using default small size disc with small IOPS, and we should resize disc before reconcile networks
	ResizeDiskBeforeNetworkReconciliation bool
	// PassHostnameByGuestInfo
	// When wm will be bootstrapped by third party cloud-init script it may require set hostname before
	// running the script by GuestInfo
	PassHostnameByGuestInfo bool

	// DefaultNetworkModeForNewVM
	// default network mode for new VM used in func getNetworkConnection
	// in some cases POOL is not good choice
	DefaultNetworkModeForNewVM string
}

// VCDMachineReconciler reconciles a VCDMachine object
type VCDMachineReconciler struct {
	client.Client
	Params VCDMachineReconcilerParams
}

// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=vcdmachines,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=vcdmachines/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=vcdmachines/finalizers,verbs=update
func (r *VCDMachineReconciler) Reconcile(ctx context.Context, req ctrl.Request) (_ ctrl.Result, rerr error) {
	log := ctrl.LoggerFrom(ctx)

	// Fetch the VCDMachine instance.
	vcdMachine := &infrav1beta3.VCDMachine{}
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
		log.Info("Please associate this machine with a cluster using the label", "label", clusterv1.ClusterNameLabel)
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
	vcdCluster := &infrav1beta3.VCDCluster{}
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
	if !controllerutil.ContainsFinalizer(vcdMachine, infrav1beta3.MachineFinalizer) {
		controllerutil.AddFinalizer(vcdMachine, infrav1beta3.MachineFinalizer)
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
		return r.reconcileDelete(ctx, machine, vcdMachine, vcdCluster)
	}

	// Handle non-deleted machines
	return r.reconcileNormal(ctx, cluster, machine, vcdMachine, vcdCluster)
}

func patchVCDMachine(ctx context.Context, patchHelper *patch.Helper, vcdMachine *infrav1beta3.VCDMachine) error {
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
	MeteringConfiguration                  = "guestinfo.metering.status"
	KubeadmInit                            = "guestinfo.postcustomization.kubeinit.status"
	KubeadmNodeJoin                        = "guestinfo.postcustomization.kubeadm.node.join.status"
	PostCustomizationScriptExecutionStatus = "guestinfo.post_customization_script_execution_status"
	PostCustomizationScriptFailureReason   = "guestinfo.post_customization_script_execution_failure_reason"
)

var postCustPhases = []string{
	NetworkConfiguration,
	MeteringConfiguration,
	ProxyConfiguration,
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

func (r *VCDMachineReconciler) waitForPostCustomizationPhase(ctx context.Context,
	vcdClient *vcdsdk.Client, vm *govcd.VM, phase string) error {
	log := ctrl.LoggerFrom(ctx)

	startTime := time.Now()
	possibleStatuses := []string{"", "in_progress", "successful"}
	currentStatus := possibleStatuses[0]
	vdcManager, err := vcdsdk.NewVDCManager(vcdClient, vcdClient.ClusterOrgName,
		vcdClient.ClusterOVDCName)
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

// getVMIDFromProviderID returns VMID from the provider ID if providerID is not nil. If the providerID is nil, empty string is returned as providerID.
func getVMIDFromProviderID(providerID *string) string {
	if providerID == nil {
		return ""
	}
	return strings.TrimPrefix(*providerID, "vmware-cloud-director://")
}

// checkIfMachineNodeIsUnhealthy returns true if the Node associated with the Machine is regarded as unhealthy, and false otherwise.
func checkIfMachineNodeIsUnhealthy(machine *clusterv1.Machine) bool {
	if conditions.IsUnknown(machine, clusterv1.MachineNodeHealthyCondition) || conditions.IsFalse(machine, clusterv1.MachineNodeHealthyCondition) {
		switch conditions.GetReason(machine, clusterv1.MachineNodeHealthyCondition) {
		case clusterv1.NodeNotFoundReason, clusterv1.NodeConditionsFailedReason:
			return true
		default:
			// handles the reasons clusterv1.WaitingForNodeRefReason and clusterv1.NodeProvisioningReason
			// These reasons are set on the Machine when provider ID is not set on the Machine and when the Node is being provisioned, respectively
			// These conditions are not regarded as Machine being unhealthy in CAPVCD.
			return false
		}
	}
	return false
}

func (r *VCDMachineReconciler) reconcileNodeSetupScripts(ctx context.Context, vcdClient *vcdsdk.Client,
	machine *clusterv1.Machine, cluster *clusterv1.Cluster, vcdMachine *infrav1beta3.VCDMachine,
	vcdCluster *infrav1beta3.VCDCluster, vAppName, vmName string, skipRDEEventUpdates bool) ([]byte, string, bool, bool, error) {

	log := ctrl.LoggerFrom(ctx, "cluster", vcdCluster.Name, "machine", machine.Name, "vAppName", vAppName)
	capvcdRdeManager := capisdk.NewCapvcdRdeManager(vcdClient, vcdCluster.Status.InfraId)

	bootstrapFormat, bootstrapJinjaScript, err := r.getBootstrapData(ctx, machine)
	if err != nil {
		capvcdRdeManager.AddToErrorSet(ctx, capisdk.VCDMachineScriptGenerationError, "", machine.Name, fmt.Sprintf("%v", err))

		return nil, "", false, false, errors.Wrapf(err, "Error retrieving bootstrap data for machine [%s] of the cluster [%s]",
			machine.Name, vcdCluster.Name)
	}

	// In a multimaster cluster, the initial control plane node runs `kubeadm init`; additional control plane nodes
	// run `kubeadm join`. The joining control planes run `kubeadm join`, so these nodes use the join script.
	// Although it is sufficient to just check if `kubeadm join` is in the bootstrap script, using the
	// isControlPlaneMachine function is a simpler operation, so this function is called first.
	isInitialControlPlane := util.IsControlPlaneMachine(machine) &&
		!strings.Contains(bootstrapJinjaScript, "kubeadm join")

	// Scaling up Control Plane initially creates the nodes as worker, which eventually joins the original control plane
	// Hence we are checking if it contains the control plane label and has kubeadm join in the script
	isResizedControlPlane := util.IsControlPlaneMachine(machine) && strings.Contains(bootstrapJinjaScript, "kubeadm join")

	var bootstrapData string
	var bootstrapDataBytes []byte
	if bootstrapFormat == BootstrapFormatCloudConfig {
		// Construct a CloudInitScriptInput struct to pass into template.Execute() function to generate the necessary
		// cloud init script for the relevant node type, i.e. control plane or worker node
		cloudInitInput := CloudInitScriptInput{
			HTTPProxy:           vcdCluster.Spec.ProxyConfigSpec.HTTPProxy,
			HTTPSProxy:          vcdCluster.Spec.ProxyConfigSpec.HTTPSProxy,
			NoProxy:             vcdCluster.Spec.ProxyConfigSpec.NoProxy,
			MachineName:         vmName,
			VcdHostFormatted:    strings.ReplaceAll(vcdCluster.Spec.Site, "/", "\\/"),
			NvidiaGPU:           false,
			TKGVersion:          getTKGVersion(cluster),    // needed for both worker & control plane machines for metering
			ClusterID:           vcdCluster.Status.InfraId, // needed for both worker & control plane machines for metering
			ResizedControlPlane: isResizedControlPlane,
		}
		if !vcdMachine.Spec.Bootstrapped && isInitialControlPlane {
			cloudInitInput.ControlPlane = true
		}

		bootstrapDataBytes, err = MergeJinjaToCloudInitScript(cloudInitInput, bootstrapJinjaScript)
		if err != nil {
			capvcdRdeManager.AddToErrorSet(ctx, capisdk.VCDMachineScriptGenerationError, "", machine.Name, fmt.Sprintf("%v", err))

			return nil, bootstrapFormat, isInitialControlPlane, isResizedControlPlane, errors.Wrapf(err,
				"Error merging bootstrap jinja script with the cloudInit script for [%s/%s] [%s]",
				vAppName, machine.Name, bootstrapJinjaScript)
		}

		bootstrapData = string(bootstrapDataBytes)
	} else if bootstrapFormat == BootstrapFormatIgnition {
		bootstrapDataBytes = []byte(bootstrapJinjaScript)
		bootstrapData = bootstrapJinjaScript
	} else {
		return nil, bootstrapFormat, isInitialControlPlane, isResizedControlPlane, errors.Wrapf(err, "Error unsupported bootstrap format [%s]", bootstrapFormat)
	}

	// nothing is redacted in the cloud init script - please ensure no secrets are present
	log.V(2).Info(fmt.Sprintf("Cloud init Script: [%s]", bootstrapData))
	capvcdRdeManager.AddToEventSet(ctx, capisdk.CloudInitScriptGenerated, "", machine.Name, "", skipRDEEventUpdates)

	err = capvcdRdeManager.RdeManager.RemoveErrorByNameOrIdFromErrorSet(ctx, vcdsdk.ComponentCAPVCD, capisdk.VCDMachineScriptGenerationError, "", machine.Name)
	if err != nil {
		log.Error(err, "failed to remove VCDMachineScriptGenerationError from RDE", "rdeID", vcdCluster.Status.InfraId)
	}

	return bootstrapDataBytes, bootstrapFormat, isInitialControlPlane, isResizedControlPlane, nil
}

func (r *VCDMachineReconciler) reconcileVMBootstrap(ctx context.Context, vcdClient *vcdsdk.Client,
	vdcManager *vcdsdk.VdcManager, vApp *govcd.VApp, vm *govcd.VM, vmName string, bootstrapData []byte, bootstrapFormat string,
	vcdCluster *infrav1beta3.VCDCluster, machine *clusterv1.Machine,
	isInitialControlPlane, isResizedControlPlane, skipRDEEventUpdates bool) error {

	if vApp == nil || vApp.VApp == nil {
		return fmt.Errorf("reconcileVMBootstrap is called with a nil VAPP")
	}
	log := ctrl.LoggerFrom(ctx, "cluster", vcdCluster.Name, "machine", machine.Name, "vAppName", vApp.VApp.Name)
	capvcdRdeManager := capisdk.NewCapvcdRdeManager(vcdClient, vcdCluster.Status.InfraId)

	vmStatus, err := vm.GetStatus()
	if err != nil {
		capvcdRdeManager.AddToErrorSet(ctx, capisdk.VCDMachineCreationError, "", machine.Name, fmt.Sprintf("%v", err))

		return errors.Wrapf(err, "Error while provisioning the infrastructure VM for the machine [%s] of the cluster [%s]; failed to get status of vm", vm.VM.Name, vApp.VApp.Name)
	}
	if err = capvcdRdeManager.RdeManager.RemoveErrorByNameOrIdFromErrorSet(ctx, vcdsdk.ComponentCAPVCD, capisdk.VCDMachineCreationError, "", ""); err != nil {
		log.Error(err, "failed to remove VCDMachineCreationError from RDE")
	}

	vAppName, err := CreateFullVAppName(ctx, r.Client, vdcManager.Vdc.Vdc.ID, vcdCluster, machine)
	if err != nil {
		return errors.Wrapf(err, "error occurred while creating vApp name for the cluster [%s] with VDC ID [%s]",
			vcdCluster.Name, vdcManager.Vdc.Vdc.ID)
	}

	if vmStatus != "POWERED_ON" {
		// try to power on the VM
		b64BootstrapData := b64.StdEncoding.EncodeToString([]byte(bootstrapData))

		var keyVals map[string]string
		if bootstrapFormat == BootstrapFormatCloudConfig {
			keyVals = map[string]string{
				"guestinfo.userdata":          b64BootstrapData,
				"guestinfo.userdata.encoding": "base64",
				"disk.enableUUID":             "1",
			}
			if r.Params.PassHostnameByGuestInfo {
				metadata := fmt.Sprintf(`local-hostname: %s`, vmName)
				b64metadata := b64.StdEncoding.EncodeToString([]byte(metadata))
				keyVals["guestinfo.metadata"] = b64metadata
				keyVals["guestinfo.metadata.encoding"] = "base64"
				keyVals["guestinfo.hostname"] = vmName
			}
		} else if bootstrapFormat == BootstrapFormatIgnition {
			networkMetadata, err := generateNetworkInitializationScriptForIgnition(vm.VM.NetworkConnectionSection, vdcManager)
			if err != nil {
				capvcdRdeManager.AddToErrorSet(ctx, capisdk.VCDMachineCreationError, "", machine.Name, fmt.Sprintf("%v", err))

				return errors.Wrapf(err, "Error while generating network initialization script for ignition [%s/%s]", vcdCluster.Name, vm.VM.Name)
			}
			keyVals = map[string]string{
				"guestinfo.ignition.config.data":          b64BootstrapData,
				"guestinfo.ignition.config.data.encoding": "base64",
				"guestinfo.ignition.vmname":               vmName,
				"disk.enableUUID":                         "1",
				"guestinfo.ignition.network":              networkMetadata,
			}
		}

		keys := capvcdutil.Keys(keyVals)
		task, err := vdcManager.SetMultiVmExtraConfigKeyValuePairs(vm, keyVals, true)
		if err != nil {
			capvcdRdeManager.AddToErrorSet(ctx, capisdk.VCDMachineCreationError, "", machine.Name, fmt.Sprintf("%v", err))

			return errors.Wrapf(err, "Error while enabling cloudinit on the machine [%s/%s]; unable to set vm extra config keys [%v] for vm ",
				vAppName, vm.VM.Name, keys)
		}
		if err = task.WaitTaskCompletion(); err != nil {
			capvcdRdeManager.AddToErrorSet(ctx, capisdk.VCDMachineCreationError, "", machine.Name, fmt.Sprintf("%v", err))

			return errors.Wrapf(err, "Error while waiting for task that sets keys [%v] machine [%s/%s]",
				keys, vAppName, vm.VM.Name)
		}
		err = capvcdRdeManager.RdeManager.RemoveErrorByNameOrIdFromErrorSet(ctx, vcdsdk.ComponentCAPVCD, capisdk.VCDMachineCreationError, "", machine.Name)
		if err != nil {
			log.Error(err, "failed to remove VCDMachineCreationError from RDE", "rdeID", vcdCluster.Status.InfraId)
		}

		log.Info(fmt.Sprintf("Configured the infra machine with keys [%v] to enable cloud-init", keys))

		task, err = vm.PowerOn()
		if err != nil {
			capvcdRdeManager.AddToErrorSet(ctx, capisdk.VCDMachineCreationError, "", machine.Name, fmt.Sprintf("%v", err))

			return errors.Wrapf(err, "Error while deploying infra for the machine [%s/%s]; unable to power on VM", vcdCluster.Name, vm.VM.Name)
		}
		if err = task.WaitTaskCompletion(); err != nil {
			capvcdRdeManager.AddToErrorSet(ctx, capisdk.VCDMachineCreationError, "", machine.Name, fmt.Sprintf("%v", err))

			return errors.Wrapf(err, "Error while deploying infra for the machine [%s/%s]; error waiting for VM power-on task completion", vcdCluster.Name, vm.VM.Name)
		}

		if err = vApp.Refresh(); err != nil {
			capvcdRdeManager.AddToErrorSet(ctx, capisdk.VCDMachineCreationError, "", machine.Name, fmt.Sprintf("%v", err))

			return errors.Wrapf(err, "Error while deploying infra for the machine [%s/%s]; unable to refresh vapp after VM power-on", vAppName, vm.VM.Name)
		}
	}

	if hasCloudInitFailedBefore, err := r.hasCloudInitExecutionFailedBefore(vcdClient, vm); hasCloudInitFailedBefore {
		capvcdRdeManager.AddToErrorSet(ctx, capisdk.VCDMachineScriptExecutionError, "", machine.Name, fmt.Sprintf("%v", err))

		return errors.Wrapf(err, "Error bootstrapping the machine [%s/%s]; machine is probably in unreconciliable state", vAppName, vm.VM.Name)
	}
	capvcdRdeManager.AddToEventSet(ctx, capisdk.InfraVmPoweredOn, "", machine.Name, "", skipRDEEventUpdates)

	err = capvcdRdeManager.RdeManager.RemoveErrorByNameOrIdFromErrorSet(ctx, vcdsdk.ComponentCAPVCD, capisdk.VCDMachineCreationError, "", machine.Name)
	if err != nil {
		log.Error(err, "failed to remove VCDMachineCreationError from RDE", "rdeID", vcdCluster.Status.InfraId)
	}

	if bootstrapFormat == BootstrapFormatCloudConfig {
		phases := postCustPhases
		if isInitialControlPlane {
			phases = append(phases, KubeadmInit)
		} else {
			phases = append(phases, KubeadmNodeJoin)
		}

		if vcdCluster.Spec.ProxyConfigSpec.HTTPSProxy == "" &&
			vcdCluster.Spec.ProxyConfigSpec.HTTPProxy == "" {
			phases = removeFromSlice(ProxyConfiguration, phases)
		}

		for _, phase := range phases {
			if err = vApp.Refresh(); err != nil {
				capvcdRdeManager.AddToErrorSet(ctx, capisdk.VCDMachineScriptExecutionError, "", machine.Name, fmt.Sprintf("%v", err))

				return errors.Wrapf(err, "Error while bootstrapping the machine [%s/%s]; unable to refresh vapp",
					vAppName, vm.VM.Name)
			}
			log.Info(fmt.Sprintf("Start: waiting for the bootstrapping phase [%s] to complete", phase))
			if err = r.waitForPostCustomizationPhase(ctx, vcdClient, vm, phase); err != nil {
				log.Error(err, fmt.Sprintf("Error waiting for the bootstrapping phase [%s] to complete", phase))
				capvcdRdeManager.AddToErrorSet(ctx, capisdk.VCDMachineScriptExecutionError, "", machine.Name, fmt.Sprintf("%v", err))

				return errors.Wrapf(err, "Error while bootstrapping the machine [%s/%s]; unable to wait for post customization phase [%s]",
					vAppName, vm.VM.Name, phase)
			}
			log.Info(fmt.Sprintf("End: waiting for the bootstrapping phase [%s] to complete", phase))
		}
	}

	err = capvcdRdeManager.RdeManager.RemoveErrorByNameOrIdFromErrorSet(ctx, vcdsdk.ComponentCAPVCD, capisdk.VCDMachineScriptExecutionError, "", "")
	if err != nil {
		log.Error(err, "failed to remove VCDMachineScriptExecutionError from RDE", "rdeID", vcdCluster.Status.InfraId)
	}

	log.Info("Successfully bootstrapped the machine")
	capvcdRdeManager.AddToEventSet(ctx, capisdk.InfraVmBootstrapped, "", machine.Name, "", skipRDEEventUpdates)

	if err = vm.Refresh(); err != nil {
		capvcdRdeManager.AddToErrorSet(ctx, capisdk.VCDMachineCreationError, "", machine.Name, fmt.Sprintf("Unable to refresh vm: %v", err))

		return errors.Wrapf(err, "Unexpected error after the machine [%s/%s] is bootstrapped; unable to refresh vm", vAppName, vm.VM.Name)
	}
	if err = vApp.Refresh(); err != nil {
		capvcdRdeManager.AddToErrorSet(ctx, capisdk.VCDMachineCreationError, "", machine.Name, fmt.Sprintf("Unable to refresh vApp: %v", err))

		return errors.Wrapf(err, "Unexpected error after the machine [%s/%s] is bootstrapped; unable to refresh vapp", vAppName, vm.VM.Name)
	}

	err = capvcdRdeManager.RdeManager.RemoveErrorByNameOrIdFromErrorSet(ctx, vcdsdk.ComponentCAPVCD, capisdk.VCDMachineCreationError, "", machine.Name)
	if err != nil {
		log.Error(err, "failed to remove VCDMachineCreationError from RDE", "rdeID", vcdCluster.Status.InfraId)
	}
	return nil
}

// getOVDCDetailsForMachine gets OVDC name and OVDC network name for the machine.
//
//	Also updates the failure domain for vcdMachine object
func (r *VCDMachineReconciler) getOVDCDetailsForMachine(ctx context.Context, machine *clusterv1.Machine,
	vcdMachine *infrav1beta3.VCDMachine, vcdCluster *infrav1beta3.VCDCluster) (string, string, error) {

	log := ctrl.LoggerFrom(ctx, "machine", machine.Name, "cluster", vcdCluster.Name)

	switch vcdCluster.Spec.MultiZoneSpec.ZoneTopology {
	case infrav1beta3.SingleZone:
		return vcdCluster.Spec.Ovdc, vcdCluster.Spec.OvdcNetwork, nil
	case infrav1beta3.DCGroup:
		failureDomainName := ""
		failureDomainNames := vcdMachine.Spec.FailureDomain
		if failureDomainNames == nil {
			failureDomainNames = machine.Spec.FailureDomain
		}
		if failureDomainNames != nil {
			failureDomainName = *failureDomainNames
		} else {
			// This is not stable and has many other deficiencies, but this approach is okay since we don't need
			// a cryptographically strong random value.
			failureDomainName = vcdCluster.Spec.MultiZoneSpec.Zones[rand.Intn(len(vcdCluster.Spec.MultiZoneSpec.Zones))].Name
			// Also update into vcdMachine data structure
			vcdMachine.Spec.FailureDomain = &failureDomainName
		}

		zoneSpec, ok := vcdCluster.Status.FailureDomains[failureDomainName]
		if !ok {
			err := fmt.Errorf("unknown failure domain name [%s]", failureDomainName)
			log.Error(err, "user specified failureDomain for the machine is not listed in vcdCluster.status.failureDomains")
			return "", "", err
		}

		ovdcName, ok := zoneSpec.Attributes["OVDCName"]
		if !ok {
			return "", "", fmt.Errorf("unable to find [OVDCName] in zone spec")
		}
		ovdcNetworkName, ok := zoneSpec.Attributes["OVDCNetworkName"]
		if !ok {
			return "", "", fmt.Errorf("unable to find [OVDCNetworkName] in zone spec")
		}
		return ovdcName, ovdcNetworkName, nil

	default:
		return "", "", fmt.Errorf("unknown zoneTopology [%s]", vcdCluster.Spec.MultiZoneSpec.ZoneTopology)
	}
}

func GetNodePoolName(ctx context.Context, cli client.Client, vcdCluster *infrav1beta3.VCDCluster,
	machine *clusterv1.Machine) (string, error) {

	machineList := &clusterv1.MachineList{}
	clusterName, ok := vcdCluster.GetLabels()[clusterv1.ClusterNameLabel]
	if !ok {
		return "", fmt.Errorf("unable to get cluster name from the vcdCluster object [%s]", vcdCluster.Name)
	}

	machineListLabels := map[string]string{clusterv1.ClusterNameLabel: clusterName}
	if err := cli.List(ctx, machineList, client.InNamespace(vcdCluster.Namespace),
		client.MatchingLabels(machineListLabels)); err != nil {
		return "", fmt.Errorf("error getting MachineList object for cluster [%s]: [%v]", vcdCluster.Name, err)
	}

	for _, machineListItem := range machineList.Items {
		if machine.Name == machineListItem.Name {
			if kcpNameLabel, ok := machineListItem.Labels[clusterv1.MachineControlPlaneNameLabel]; ok {
				return kcpNameLabel, nil
			} else {
				if machineDeploymentNameLabel, ok := machineListItem.Labels[clusterv1.MachineDeploymentNameLabel]; ok {
					return machineDeploymentNameLabel, nil
				}
			}
		}
	}

	return "", fmt.Errorf("unable to find machine deployment for cluster [%s], machine [%s]",
		vcdCluster.Name, machine.Name)
}

func CreateFullVAppName(ctx context.Context, cli client.Client, ovdcID string,
	vcdCluster *infrav1beta3.VCDCluster, machine *clusterv1.Machine) (string, error) {

	switch vcdCluster.Spec.MultiZoneSpec.ZoneTopology {
	case "":
		return vcdCluster.Name, nil

	case infrav1beta3.DCGroup:
		machineDeploymentName, err := GetNodePoolName(ctx, cli, vcdCluster, machine)
		if err != nil {
			return "", fmt.Errorf("unable to get MachineDeployment name from cluster [%s], machine [%s]: [%v]",
				vcdCluster.Name, machine.Name, err)
		}
		vAppPrefix, err := capvcdutil.CreateVAppNamePrefix(vcdCluster.Name, ovdcID)
		if err != nil {
			return "", fmt.Errorf("unable to get vApp name from cluster name [%s], machine [%s]: [%v]",
				vcdCluster.Name, machine.Name, err)
		}
		return fmt.Sprintf("%s_%s", vAppPrefix, machineDeploymentName), nil

	default:
		return "", fmt.Errorf("encountered unknown zonetype while creating vApp Name for cluster [%s]",
			vcdCluster.Name)
	}
}

func (r *VCDMachineReconciler) reconcileVAppCreation(ctx context.Context, vcdClient *vcdsdk.Client,
	machineName string, vcdCluster *infrav1beta3.VCDCluster,
	vAppName string, ovdcNetworkName string, skipRDEEventUpdates bool) (ctrl.Result, error) {

	log := ctrl.LoggerFrom(ctx, "machine", machineName, "cluster", vcdCluster.Name, "vAppName", vAppName)
	capvcdRdeManager := capisdk.NewCapvcdRdeManager(vcdClient, vcdCluster.Status.InfraId)
	rdeManager := vcdsdk.NewRDEManager(vcdClient, vcdCluster.Status.InfraId, capisdk.StatusComponentNameCAPVCD,
		release.Version)
	vdcManager, err := vcdsdk.NewVDCManager(vcdClient, vcdClient.ClusterOrgName, vcdClient.ClusterOVDCName)
	if err != nil {
		capvcdRdeManager.AddToErrorSet(ctx, capisdk.VCDClusterError, "", vcdCluster.Name,
			fmt.Sprintf("failed to get vdcManager: [%v]", err))
		return ctrl.Result{}, errors.Wrapf(err,
			"Error creating vdc manager to reconcile vcd infrastructure for cluster [%s]", vcdCluster.Name)
	}
	metadataMap := map[string]string{
		CapvcdInfraId: vcdCluster.Status.InfraId,
	}
	if vdcManager.Vdc == nil {
		capvcdRdeManager.AddToErrorSet(ctx, capisdk.VCDClusterError, "",
			vcdCluster.Name, fmt.Sprintf("%v", err))
		return ctrl.Result{}, errors.Errorf("no Vdc created with vdc manager name [%s]", vdcManager.Client.ClusterOVDCName)
	}
	if err = capvcdRdeManager.RdeManager.RemoveErrorByNameOrIdFromErrorSet(ctx, vcdsdk.ComponentCAPVCD,
		capisdk.VCDClusterError, "", ""); err != nil {
		log.Error(err, "failed to remove VCDClusterError from RDE", "rdeID", vcdCluster.Status.InfraId)
	}

	_, err = vdcManager.Vdc.GetVAppByName(vAppName, true)
	if err != nil && err == govcd.ErrorEntityNotFound {
		vcdCluster.Status.VAppMetadataUpdated = false
	}

	clusterVApp, err := vdcManager.GetOrCreateVApp(vAppName, ovdcNetworkName)
	if err != nil {
		capvcdRdeManager.AddToErrorSet(ctx, capisdk.VCDClusterVappCreationError, "", vAppName,
			fmt.Sprintf("%v", err))
		return ctrl.Result{}, errors.Wrapf(err, "Error creating Infra vApp for the cluster [%s]: [%v]",
			vcdCluster.Name, err)
	}
	if clusterVApp == nil || clusterVApp.VApp == nil {
		capvcdRdeManager.AddToErrorSet(ctx, capisdk.VCDClusterVappCreationError, "",
			vcdCluster.Name, fmt.Sprintf("%v", err))
		return ctrl.Result{}, errors.Wrapf(err, "found nil value for VApp [%s]", vAppName)
	}

	// AMK: TODO: this is likely not needed since the resourceset will get added later.
	//if !strings.HasPrefix(vcdCluster.Status.InfraId, NoRdePrefix) {
	//	if err := r.reconcileRDE(ctx, cluster, vcdCluster, vcdClient, clusterVApp.VApp.ID, true); err != nil {
	//		log.Error(err, "failed to add VApp ID to RDE", "rdeID", vcdCluster.Status.InfraId,
	//			"vappID", clusterVApp.VApp.ID)
	//		return ctrl.Result{}, errors.Wrapf(err, "failed to update RDE [%s] with VApp ID [%s]: [%v]",
	//			vcdCluster.Status.InfraId, clusterVApp.VApp.ID, err)
	//	}
	//	log.Info("successfully updated external ID of RDE with VApp ID", "infraID", vcdCluster.Status.InfraId,
	//		"vAppID", clusterVApp.VApp.ID)
	//}

	if metadataMap != nil && !vcdCluster.Status.VAppMetadataUpdated {
		if err := vdcManager.AddMetadataToVApp(vAppName, metadataMap); err != nil {
			capvcdRdeManager.AddToErrorSet(ctx, capisdk.VCDClusterError, "", vAppName,
				fmt.Sprintf("failed to add metadata into vApp [%s]: [%v]", vcdCluster.Name, err))
			return ctrl.Result{}, fmt.Errorf("unable to add metadata [%s] to vApp [%s]: [%v]", metadataMap,
				vAppName, err)
		}

		// The following requires a patch to the vcdCluster object.
		// TODO: evaluate if VCDCluster.VAppMetadataUpdated is required
		vcdCluster.Status.VAppMetadataUpdated = true
	}

	// Add VApp to VCDResourceSet
	err = rdeManager.AddToVCDResourceSet(ctx, vcdsdk.ComponentCAPVCD, VCDResourceVApp,
		vAppName, clusterVApp.VApp.ID, nil)
	if err != nil {
		capvcdRdeManager.AddToErrorSet(ctx, capisdk.RdeError, "", vAppName,
			fmt.Sprintf("failed to add VCD Resource [%s] of type [%s] from VCDResourceSet of RDE [%s]: [%v]",
				vAppName, VcdResourceTypeVM, vcdCluster.Status.InfraId, err))
		return ctrl.Result{}, errors.Wrapf(err,
			"failed to add resource [%s] of type [%s] to VCDResourceSet of RDE [%s]: [%v]",
			vAppName, VCDResourceVApp, vcdCluster.Status.InfraId, err)
	}

	capvcdRdeManager.AddToEventSet(ctx, capisdk.InfraVappAvailable, clusterVApp.VApp.ID, "",
		"", skipRDEEventUpdates)

	err = capvcdRdeManager.RdeManager.RemoveErrorByNameOrIdFromErrorSet(ctx, vcdsdk.ComponentCAPVCD,
		capisdk.VCDClusterVappCreationError, clusterVApp.VApp.ID, vAppName)
	if err != nil {
		log.Error(err, "failed to remove VCDClusterVappCreationError from RDE",
			"rdeID", vcdCluster.Status.InfraId)
	}

	return ctrl.Result{}, nil
}

func (r *VCDMachineReconciler) reconcileVM(
	ctx context.Context, vcdClient *vcdsdk.Client, vdcManager *vcdsdk.VdcManager,
	vApp *govcd.VApp, machine *clusterv1.Machine, vcdMachine *infrav1beta3.VCDMachine,
	vmName string, ovdcNetworkName string,
	vcdCluster *infrav1beta3.VCDCluster) (res ctrl.Result, vm *govcd.VM, machineAddress string, retErr error) {

	if vApp == nil || vApp.VApp == nil {
		return ctrl.Result{}, nil, "", errors.New("reconcileVM is called with nil vApp")
	}

	vAppName := vApp.VApp.Name
	log := ctrl.LoggerFrom(ctx, "machine", machine.Name, "cluster", vcdCluster.Name, "vAppName", vAppName)
	capvcdRdeManager := capisdk.NewCapvcdRdeManager(vcdClient, vcdCluster.Status.InfraId)

	vmExists := true
	vm, err := vApp.GetVMByName(vmName, true)
	if err != nil && err != govcd.ErrorEntityNotFound {
		capvcdRdeManager.AddToErrorSet(ctx, capisdk.VCDMachineCreationError, "",
			machine.Name, fmt.Sprintf("%v", err))
		return ctrl.Result{}, nil, "", errors.Wrapf(err,
			"Error provisioning infrastructure for the machine; unable to query for VM [%s] in vApp [%s]",
			machine.Name, vAppName)
	} else if err == govcd.ErrorEntityNotFound {
		vmExists = false
	}
	if !vmExists {
		log.Info("Adding infra VM for the machine")
		var task govcd.Task
		var err error
		if r.Params.UseNormalVms {
			log.Info("Using normal VM")
			// By default vcloud director supports 2 cloud-init datasources - OVF and Vmware.
			// In standard distros cloud-init checks OVF datasource first.
			// CAPSVCD only passes arguments to Vmware cloud-init, so we need to modify cloud-init datasource order and reboot node to apply cloud-init changes.
			guestCustScript := `#!/usr/bin/env bash
cat > /etc/cloud/cloud.cfg.d/98-cse-vmware-datasource.cfg <<EOF
datasource_list: [ "VMware" ]
EOF
shutdown -r now
`
			task, err = vdcManager.AddNewVM(vmName, vAppName,
				vcdMachine.Spec.Catalog, vcdMachine.Spec.Template, vcdMachine.Spec.PlacementPolicy,
				vcdMachine.Spec.SizingPolicy, vcdMachine.Spec.StorageProfile, guestCustScript)
		} else {
			log.Info("Using TGK VM")
			// vcda-4391 fixed
			task, err = vdcManager.AddNewTkgVM(vmName, vAppName,
				vcdMachine.Spec.Catalog, vcdMachine.Spec.Template, vcdMachine.Spec.PlacementPolicy,
				vcdMachine.Spec.SizingPolicy, vcdMachine.Spec.StorageProfile)
		}
		if err != nil {
			capvcdRdeManager.AddToErrorSet(ctx, capisdk.VCDMachineCreationError, "", machine.Name,
				fmt.Sprintf("%v", err))
			return ctrl.Result{}, nil, "", errors.Wrapf(err,
				"Error provisioning infrastructure for the machine; unable to create VM [%s] in vApp [%s]",
				machine.Name, vAppName)
		}
		if err = task.WaitTaskCompletion(); err != nil {
			capvcdRdeManager.AddToErrorSet(ctx, capisdk.VCDMachineCreationError, "", machine.Name,
				fmt.Sprintf("%v", err))
			return ctrl.Result{}, nil, "", errors.Wrapf(err,
				"Error provisioning infrastructure for the machine; unable to wait for task [%v] for create VM [%s] in vApp [%s]",
				task, machine.Name, vAppName)
		}

		vm, err = vApp.GetVMByName(vmName, true)
		if err != nil {
			capvcdRdeManager.AddToErrorSet(ctx, capisdk.VCDMachineCreationError, "",
				machine.Name, fmt.Sprintf("%v", err))
			return ctrl.Result{}, nil, "", errors.Wrapf(err,
				"Error provisioning infrastructure for the machine; unable to find newly created VM [%s] in vApp [%s]",
				vm.VM.Name, vAppName)
		}
		if vm == nil || vm.VM == nil {
			capvcdRdeManager.AddToErrorSet(ctx, capisdk.VCDMachineCreationError, "",
				machine.Name, fmt.Sprintf("%v", err))
			return ctrl.Result{}, nil, "", errors.Wrapf(err, "Obtained nil VM after creating VM [%s/%s]",
				vAppName, machine.Name)
		}

		// NOTE: VMs are not added to VCDResourceSet intentionally as the VMs can be obtained from the VApp and
		// 	VCDResourceSet can get bloated with VMs if the cluster contains a large number of worker nodes
	}

	// in some cases we want to resize disk before starting VM
	resizeHardDisk := func(vm *govcd.VM, vcdMachine *infrav1beta3.VCDMachine) (res ctrl.Result, retErr error) {
		// only resize hard disk if the user has requested so by specifying such in the VCDMachineTemplate spec
		// check isn't strictly required as we ensure that specified number is larger than what's in the template and left
		// empty this will just be 0. However, this makes it clear from a standpoint of inspecting the code what we are doing
		if !vcdMachine.Spec.DiskSize.IsZero() {
			// go-vcd expects value in MB (2^10 = 1024 * 1024 bytes), so we scale it as such
			diskSize, ok := vcdMachine.Spec.DiskSize.AsInt64()
			if !ok {
				capvcdRdeManager.AddToErrorSet(ctx, capisdk.VCDMachineCreationError,
					"", machine.Name, fmt.Sprintf("%v", err))
				return ctrl.Result{},
					fmt.Errorf("error while provisioning the infrastructure VM for the machine [%s] of the cluster [%s]; "+
						"failed to parse disk size quantity [%s]", vm.VM.Name, vApp.VApp.Name, vcdMachine.Spec.DiskSize.String())
			}
			diskSize = int64(math.Floor(float64(diskSize) / float64(Mebibyte)))
			diskSettings := vm.VM.VmSpecSection.DiskSection.DiskSettings
			// if the specified disk size is less than what is defined in the template, then we ignore the field
			if len(diskSettings) != 0 && diskSettings[0].SizeMb < diskSize {
				log.Info(
					fmt.Sprintf("resizing hard disk on VM for machine [%s] of cluster [%s]; resizing from [%dMB] to [%dMB]",
						vmName, vAppName, diskSettings[0].SizeMb, diskSize))

				diskSettings[0].SizeMb = diskSize
				vm.VM.VmSpecSection.DiskSection.DiskSettings = diskSettings

				if _, err = vm.UpdateInternalDisks(vm.VM.VmSpecSection); err != nil {
					capvcdRdeManager.AddToErrorSet(ctx, capisdk.VCDMachineCreationError, "",
						machine.Name, fmt.Sprintf("%v", err))
					return ctrl.Result{},
						errors.Wrapf(err, "Error while provisioning the infrastructure VM for the machine [%s] "+
							"of the cluster [%s]; failed to resize hard disk", vm.VM.Name, vApp.VApp.Name)
				}
			}
			if err = capvcdRdeManager.RdeManager.RemoveErrorByNameOrIdFromErrorSet(ctx, vcdsdk.ComponentCAPVCD,
				capisdk.VCDMachineCreationError, "", ""); err != nil {
				log.Error(err, "failed to remove VCDMachineCreationError from RDE")
			}
		}

		return ctrl.Result{}, nil
	}

	if r.Params.ResizeDiskBeforeNetworkReconciliation {
		log.Info("Resize hard disk before starting VM")
		res, err := resizeHardDisk(vm, vcdMachine)
		if err != nil {
			return res, nil, "", errors.Wrapf(err, "Cannot resize hard disk")
		}
	}

	desiredNetworks := []string{ovdcNetworkName}
	if vcdMachine.Spec.ExtraOvdcNetworks != nil {
		desiredNetworks = append([]string{ovdcNetworkName}, vcdMachine.Spec.ExtraOvdcNetworks...)
	}
	if err = r.reconcileVMNetworks(vdcManager, vApp, vm, desiredNetworks); err != nil {
		log.Error(err, "Error while attaching networks to vApp and VMs")
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil, "", nil
	}

	// checks before setting address in machine status
	if vm.VM == nil {
		log.Error(nil, fmt.Sprintf("Requeuing...; vm.VM should not be nil: [%#v]", vm))
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil, "", nil
	}
	if vm.VM.NetworkConnectionSection == nil || len(vm.VM.NetworkConnectionSection.NetworkConnection) == 0 {
		log.Error(nil, fmt.Sprintf("Requeuing...; network connection section was not found for vm [%s(%s)]: [%#v]", vm.VM.Name, vm.VM.ID, vm.VM))
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil, "", nil
	}

	primaryNetwork := getPrimaryNetwork(vm.VM)
	if primaryNetwork == nil {
		log.Error(nil, fmt.Sprintf(
			"Requeuing...; failed to get existing network connection information for vm [%s(%s)]: [%#v]. "+
				"NetworkConnection[0] should not be nil",
			vm.VM.Name, vm.VM.ID, vm.VM.NetworkConnectionSection))
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil, "", nil
	}

	if primaryNetwork.IPAddress == "" {
		log.Error(nil,
			fmt.Sprintf("Requeuing...; NetworkConnection[0] IP Address should not be empty for vm [%s(%s)]: [%#v]",
				vm.VM.Name, vm.VM.ID, *vm.VM.NetworkConnectionSection.NetworkConnection[0]))
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil, "", nil
	}

	// set address in machine status
	machineAddress = primaryNetwork.IPAddress
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

	if !r.Params.ResizeDiskBeforeNetworkReconciliation {
		log.Info("Resize hard disk after starting VM")
		res, err := resizeHardDisk(vm, vcdMachine)
		if err != nil {
			return res, nil, "", errors.Wrapf(err, "Cannot resize hard disk")
		}
	}

	return ctrl.Result{}, vm, machineAddress, nil
}

func (r *VCDMachineReconciler) reconcileLBPool(ctx context.Context, machine *clusterv1.Machine, machineAddress string,
	vcdCluster *infrav1beta3.VCDCluster, vcdClient *vcdsdk.Client, gateway *vcdsdk.GatewayManager) error {

	log := ctrl.LoggerFrom(ctx, "cluster", vcdCluster.Name, "machine", machine.Name)
	capvcdRdeManager := capisdk.NewCapvcdRdeManager(vcdClient, vcdCluster.Status.InfraId)

	virtualServiceName := capisdk.GetVirtualServiceNameUsingPrefix(
		capisdk.GetVirtualServiceNamePrefix(vcdCluster.Name, vcdCluster.Status.InfraId), "tcp")
	lbPoolName := capisdk.GetLoadBalancerPoolNameUsingPrefix(
		capisdk.GetLoadBalancerPoolNamePrefix(vcdCluster.Name, vcdCluster.Status.InfraId), "tcp")
	lbPoolRef, err := gateway.GetLoadBalancerPool(ctx, lbPoolName)
	if err != nil {
		capvcdRdeManager.AddToErrorSet(ctx, capisdk.LoadBalancerError, "", machine.Name,
			fmt.Sprintf("Error retrieving/updating load balancer pool [%s]: %v", lbPoolName, err))
		return fmt.Errorf("unable to retrieve/update load balancer pool [%s] for the "+
			"control plane machine [%s] of the cluster [%s]: [%v]", lbPoolName, machine.Name, vcdCluster.Name, err)
	}
	controlPlaneIPs, err := gateway.GetLoadBalancerPoolMemberIPs(ctx, lbPoolRef)
	if err != nil {
		capvcdRdeManager.AddToErrorSet(ctx, capisdk.LoadBalancerError,
			"", machine.Name, fmt.Sprintf("Error retrieving/updating lpool members [%s]: %v", lbPoolName, err))
		return fmt.Errorf("unable to retrieve/update load balancer pool members [%s] for the "+
			"control plane machine [%s] of the cluster [%s]: [%v]", lbPoolName, machine.Name, vcdCluster.Name, err)
	}

	err = capvcdRdeManager.RdeManager.RemoveErrorByNameOrIdFromErrorSet(ctx, vcdsdk.ComponentCAPVCD,
		capisdk.LoadBalancerError, "", "")
	if err != nil {
		log.Error(err, "failed to remove LoadBalancerError from RDE",
			"rdeID", vcdCluster.Status.InfraId)
	}

	updatedIPs := append(controlPlaneIPs, machineAddress)
	updatedUniqueIPs := cpiutil.NewSet(updatedIPs).GetElements()
	resourcesAllocated := &cpiutil.AllocatedResourcesMap{}
	var oneArm *vcdsdk.OneArm = nil
	if vcdCluster.Spec.LoadBalancerConfigSpec.UseOneArm {
		oneArm = &OneArmDefault
	}

	// At this point the vcdCluster.Spec.ControlPlaneEndpoint should have been set correctly.
	// We are not using externalIp=vcdCluster.Spec.ControlPlaneEndpoint.Host because of a possible race between which controller picks ups according to the spec.
	// 1. If vcdcluster controller picks up vcdCluster.Spec.ControlPlaneEndpoint.Host, there is no issue as it will retrieve it from existing VCD VirtualService.
	// 2. If vcdmachine controller picks up first, then it would update the LB according to vcdCluster.Spec.ControlPlaneEndpoint.Host.
	// We are deciding to pass externalIp="" in this case, as UpdateVirtualService() would see it's an empty string, so it would just update the VS Object with what's already present.
	// Users should not be updating control plane IP after it has been created, so this is not a valid use case.
	// TODO: CAFV-143 In the the future, ideally we should add ControlPlaneEndpoint.Host, ControlPlaneEndpoint.Port into VCDClusterStatus, and pass externalIp=vcdCluster.Status.ControlPlaneEndpoint.Host instead
	_, err = gateway.UpdateLoadBalancer(ctx, lbPoolName, virtualServiceName, updatedUniqueIPs,
		"", int32(vcdCluster.Spec.ControlPlaneEndpoint.Port), int32(vcdCluster.Spec.ControlPlaneEndpoint.Port),
		oneArm, !vcdCluster.Spec.LoadBalancerConfigSpec.UseOneArm, "TCP", resourcesAllocated)
	if err != nil {
		capvcdRdeManager.AddToErrorSet(ctx, capisdk.VCDMachineCreationError, "",
			machine.Name, fmt.Sprintf("%v", err))
		return fmt.Errorf("unable to update LB pool [%s] for the control plane machine [%s] of the cluster [%s]: [%v]",
			lbPoolName, machine.Name, vcdCluster.Name, err)
	}
	log.Info("Updated the load balancer pool with the control plane machine IP",
		"lbpool", lbPoolName)

	return nil
}

func (r *VCDMachineReconciler) reconcileNormal(ctx context.Context, cluster *clusterv1.Cluster,
	machine *clusterv1.Machine, vcdMachine *infrav1beta3.VCDMachine, vcdCluster *infrav1beta3.VCDCluster) (res ctrl.Result, retErr error) {

	log := ctrl.LoggerFrom(ctx, "machine", machine.Name, "cluster", vcdCluster.Name)

	// To avoid spamming RDEs with updates, only update the RDE with events when machine creation is ongoing
	skipRDEEventUpdates := machine.Status.BootstrapReady

	ovdcName, ovdcNetworkName, err := r.getOVDCDetailsForMachine(ctx, machine, vcdMachine, vcdCluster)
	if err != nil {
		log.Error(err, "Unable to get OVDC details of machine")
		return ctrl.Result{}, errors.Wrapf(err, "unable to get OVDC details of machine [%s]", vcdMachine.Name)
	}

	// create new logger with OVDC and Machine names
	log = ctrl.LoggerFrom(ctx, "cluster", vcdCluster.Name, "ovdc", ovdcName, "machine", machine.Name)
	log.Info("Starting VCDMachine reconciliation")

	if vcdMachine.Spec.ProviderID != nil && vcdMachine.Status.ProviderID != nil {
		vcdMachine.Status.Ready = true
		conditions.MarkTrue(vcdMachine, ContainerProvisionedCondition)

		if checkIfMachineNodeIsUnhealthy(machine) {
			// Create the workload client only if the machine is unhealthy because we would have to add an event to the RDE.
			// Else there is no need to login to VCD
			vcdClient, err := loginVCD(ctx, r.Client, vcdCluster, ovdcName, true)
			// close all idle connections when reconciliation is done
			defer func() {
				if vcdClient != nil && vcdClient.VCDClient != nil {
					vcdClient.VCDClient.Client.Http.CloseIdleConnections()
					log.Info(fmt.Sprintf("closed connection to the http client [%#v]", vcdClient.VCDClient.Client.Http))
				}
			}()
			if err != nil {
				log.Error(err, "error occurred while logging in to VCD")
				return ctrl.Result{}, errors.Wrapf(err, "error occurred while logging in to VCD: [%v]", err)
			}

			capvcdRdeManager := capisdk.NewCapvcdRdeManager(vcdClient, vcdCluster.Status.InfraId)
			if conditions.IsFalse(machine, clusterv1.MachineHealthCheckSucceededCondition) {
				capvcdRdeManager.AddToEventSet(ctx, capisdk.NodeHealthCheckFailed, getVMIDFromProviderID(vcdMachine.Status.ProviderID), machine.Name, conditions.GetMessage(machine, clusterv1.MachineHealthCheckSucceededCondition), false)
			}
			capvcdRdeManager.AddToEventSet(ctx, capisdk.NodeUnhealthy, getVMIDFromProviderID(vcdMachine.Status.ProviderID), machine.Name, conditions.GetMessage(machine, clusterv1.MachineNodeHealthyCondition), false)
		}

		return ctrl.Result{}, nil
	}

	vcdClient, err := loginVCD(ctx, r.Client, vcdCluster, ovdcName, true)

	// close all idle connections when reconciliation is done
	defer func() {
		if vcdClient != nil && vcdClient.VCDClient != nil {
			vcdClient.VCDClient.Client.Http.CloseIdleConnections()
			log.Info(fmt.Sprintf("closed connection to the http client [%#v]", vcdClient.VCDClient.Client.Http))
		}
	}()
	if err != nil {
		log.Error(err, "error occurred while logging in to VCD")
		return ctrl.Result{}, errors.Wrapf(err, "error occurred while logging in to VCD: [%v]", err)
	}

	capvcdRdeManager := capisdk.NewCapvcdRdeManager(vcdClient, vcdCluster.Status.InfraId)
	if conditions.IsFalse(machine, clusterv1.MachineHealthCheckSucceededCondition) {
		capvcdRdeManager.AddToEventSet(ctx, capisdk.NodeHealthCheckFailed, getVMIDFromProviderID(vcdMachine.Status.ProviderID), machine.Name, conditions.GetMessage(machine, clusterv1.MachineHealthCheckSucceededCondition), false)
	}
	if checkIfMachineNodeIsUnhealthy(machine) {
		capvcdRdeManager.AddToEventSet(ctx, capisdk.NodeUnhealthy, getVMIDFromProviderID(vcdMachine.Status.ProviderID), machine.Name, conditions.GetMessage(machine, clusterv1.MachineNodeHealthyCondition), false)
	}

	patchHelper, err := patch.NewHelper(vcdMachine, r.Client)
	if err != nil {
		capvcdRdeManager.AddToErrorSet(ctx, capisdk.CAPVCDObjectPatchError, "", machine.Name, fmt.Sprintf("%v", err))
		return ctrl.Result{}, errors.Wrapf(err, "Error patching VCDMachine [%s] of cluster [%s]", vcdMachine.Name, vcdCluster.Name)
	}

	if !conditions.Has(vcdMachine, BootstrapExecSucceededCondition) {
		conditions.MarkFalse(vcdMachine, BootstrapExecSucceededCondition,
			BootstrappingReason, clusterv1.ConditionSeverityInfo, "")
		if err := patchVCDMachine(ctx, patchHelper, vcdMachine); err != nil {
			capvcdRdeManager.AddToErrorSet(ctx, capisdk.CAPVCDObjectPatchError, "", machine.Name, fmt.Sprintf("%v", err))
			return ctrl.Result{}, errors.Wrapf(err, "Error patching VCDMachine [%s] of cluster [%s]", vcdMachine.Name, vcdCluster.Name)
		}
	}
	err = capvcdRdeManager.RdeManager.RemoveErrorByNameOrIdFromErrorSet(ctx, vcdsdk.ComponentCAPVCD, capisdk.CAPVCDObjectPatchError, "", machine.Name)
	if err != nil {
		log.Error(err, "failed to remove CAPVCDObjectPatchError from RDE", "rdeID", vcdCluster.Status.InfraId)
	}
	vdcManager, err := vcdsdk.NewVDCManager(vcdClient, vcdClient.ClusterOrgName,
		vcdClient.ClusterOVDCName)
	if err != nil {
		capvcdRdeManager.AddToErrorSet(ctx, capisdk.VCDMachineError, "", machine.Name, fmt.Sprintf("%v", err))

		return ctrl.Result{}, errors.Wrapf(err, "failed to create a vdc manager object when reconciling machine [%s]", vcdMachine.Name)
	}
	err = capvcdRdeManager.RdeManager.RemoveErrorByNameOrIdFromErrorSet(ctx, vcdsdk.ComponentCAPVCD, capisdk.VCDMachineError, "", machine.Name)
	if err != nil {
		log.Error(err, "failed to remove VCDMachineError from RDE", "rdeID", vcdCluster.Status.InfraId)
	}

	// Create the vApp if it doesn't already exist. In the multi-AZ case, this has to be done in the machine controller
	// since new zones could be added dynamically. In the non-AZ case, it can be done in the vcdCluster controller. However,
	// we do it in one place for simplicity.
	// TODO: should we add a field in VCDMachine to store the VApp name used for the machine ?
	vAppName, err := CreateFullVAppName(ctx, r.Client, vdcManager.Vdc.Vdc.ID, vcdCluster, machine)
	log.Info(fmt.Sprintf("Using VApp name [%s] for the machine [%s]", vAppName, machine.Name))

	if err != nil {
		log.Error(err, "error occurred while creating vApp name prefix")
		return ctrl.Result{}, errors.Wrapf(err, "error creating vApp name prefix for the vcdMachine [%s]",
			vcdMachine.Name)
	}
	log.Info(fmt.Sprintf("Using VApp name [%s] for the machine [%s]", vAppName, machine.Name))

	result, err := r.reconcileVAppCreation(ctx, vcdClient, machine.Name, vcdCluster, vAppName, ovdcNetworkName, false)
	if err != nil {
		log.Error(err, "failed to reconcile vApp", "vAppName", vAppName)
		return result, errors.Wrapf(err, "unable to reconcile vApp [%s] for cluster [%s]", vAppName, vcdCluster.Name)
	}

	vApp, err := vdcManager.Vdc.GetVAppByName(vAppName, true)
	if err != nil {
		capvcdRdeManager.AddToErrorSet(ctx, capisdk.VCDClusterVappCreationError, "", machine.Name, fmt.Sprintf("%v", err))
		return ctrl.Result{}, errors.Wrapf(err,
			"Error provisioning infrastructure VApp for the machine [%s] of the cluster [%s]",
			machine.Name, vcdCluster.Name)
	}
	err = capvcdRdeManager.RdeManager.RemoveErrorByNameOrIdFromErrorSet(ctx, vcdsdk.ComponentCAPVCD, capisdk.VCDClusterVappCreationError, "", "")
	if err != nil {
		log.Error(err, "failed to remove VCDClusterVappCreationError from RDE", "rdeID", vcdCluster.Status.InfraId)
	}

	vmName, err := getVMName(machine, vcdMachine, log)
	if err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "unable to get VM [%s] by name for cluster [%s]",
			machine.Name, vcdCluster.Name)
	}
	log.Info(fmt.Sprintf("Using VM name [%s] in VApp [%s] for the machine [%s]", vmName, vAppName, machine.Name))

	result, vm, machineAddress, err := r.reconcileVM(ctx, vcdClient, vdcManager, vApp, machine, vcdMachine,
		vmName, ovdcNetworkName, vcdCluster)
	if err != nil {
		capvcdRdeManager.AddToErrorSet(ctx, capisdk.VCDMachineError, "", machine.Name, fmt.Sprintf("%v", err))
		return result, errors.Wrapf(err, "unable to provision infrastructure for VM [%s/%s] in ovdc[%s] with network [%s]",
			vAppName, vmName, ovdcName, ovdcNetworkName)
	} else if result.Requeue || result.RequeueAfter > 0 {
		log.Info("Re queuing the request",
			"result.Requeue", result.Requeue, "result.RequeueAfter", result.RequeueAfter.String())
		return result, nil
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
	conditions.MarkTrue(vcdMachine, ContainerProvisionedCondition)

	bootstrapData, bootstrapFormat, isInitialControlPlane, isResizedControlPlane, err := r.reconcileNodeSetupScripts(
		ctx, vcdClient, machine, cluster, vcdMachine, vcdCluster, vAppName, vmName, skipRDEEventUpdates)

	gateway, err := vcdsdk.NewGatewayManager(ctx, vcdClient, ovdcNetworkName, vcdCluster.Spec.LoadBalancerConfigSpec.VipSubnet, ovdcName)
	if err != nil {
		capvcdRdeManager.AddToErrorSet(ctx, capisdk.VCDMachineCreationError, "", machine.Name, fmt.Sprintf("%v", err))

		return ctrl.Result{}, errors.Wrapf(err, "failed to create gateway manager object while reconciling machine [%s]", vcdMachine.Name)
	}

	// TODO (UserSpecifiedEdge topology): update gateway reference for the `gateway` object

	// Update loadbalancer pool with the IP of the control plane node as a new member.
	// Note that this must be done before booting on the VM!
	if isInitialControlPlane {
		if err := r.reconcileLBPool(ctx, machine, machineAddress, vcdCluster, vcdClient, gateway); err != nil {
			return ctrl.Result{}, errors.Wrapf(err, "unable to add machine address [%s] into LB Pool for the "+
				"control plane machine [%s] of the cluster [%s]", machineAddress, machine.Name, vcdCluster.Name)
		}
	}

	err = r.reconcileVMBootstrap(ctx, vcdClient, vdcManager, vApp, vm, vmName, bootstrapData, bootstrapFormat, vcdCluster, machine,
		isInitialControlPlane, isResizedControlPlane, skipRDEEventUpdates)
	if err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "failed to bootstrap VM [%s/%s]", vAppName, vmName)
	}

	// Update load-balancer pool with the IP of the control plane node as a new member.
	// For joining nodes the LB Pool should be updated after the VM has joined.
	if isResizedControlPlane {
		if err := r.reconcileLBPool(ctx, machine, machineAddress, vcdCluster, vcdClient, gateway); err != nil {
			return ctrl.Result{}, errors.Wrapf(err, "unable to add machine address [%s] into LB Pool for the "+
				"control plane machine [%s] of the cluster [%s]", machineAddress, machine.Name, vcdCluster.Name)
		}
	}

	vcdMachine.Spec.Bootstrapped = true
	conditions.MarkTrue(vcdMachine, BootstrapExecSucceededCondition)
	// Set ProviderID so the Cluster API Machine Controller can pull it
	providerID := fmt.Sprintf("%s://%s", infrav1beta3.VCDProviderID, vm.VM.ID)
	vcdMachine.Spec.ProviderID = &providerID
	vcdMachine.Status.Ready = true
	vcdMachine.Status.Template = vcdMachine.Spec.Template
	vcdMachine.Status.ProviderID = vcdMachine.Spec.ProviderID
	vcdMachine.Status.SizingPolicy = vcdMachine.Spec.SizingPolicy
	vcdMachine.Status.PlacementPolicy = vcdMachine.Spec.PlacementPolicy
	vcdMachine.Status.NvidiaGPUEnabled = vcdMachine.Spec.EnableNvidiaGPU
	vcdMachine.Status.FailureDomain = vcdMachine.Spec.FailureDomain
	conditions.MarkTrue(vcdMachine, ContainerProvisionedCondition)
	return ctrl.Result{}, nil
}

// generateNetworkInitializationScriptForIgnition creates the bash script that will create the networkd units stored in metadata
// and consumed by ignition
func generateNetworkInitializationScriptForIgnition(networkConnection *types.NetworkConnectionSection, vdcManager *vcdsdk.VdcManager) (string, error) {
	ignitionNetworkInitTemplate, err := template.New("ignition_network_init_script_template").Parse(ignitionNetworkInitScriptTemplate)
	if err != nil {
		return "", errors.Wrapf(err, "Error parsing ignitionNetworkInitScriptTemplate [%s]", ignitionNetworkInitTemplate.Name())
	}

	var sectionInputConfigs []IgnitionNetworkInitScriptSectionInput
	for _, network := range networkConnection.NetworkConnection {
		// Process NIC network properties and subnet CIDR
		orgVdcNetwork, err := vdcManager.Vdc.GetOrgVdcNetworkByName(network.Network, true)
		if err != nil {
			return "", err
		}

		ipScope := orgVdcNetwork.OrgVDCNetwork.Configuration.IPScopes.IPScope[0]
		netmask := net.ParseIP(ipScope.Netmask)
		netmaskCidr, _ := net.IPMask(netmask.To4()).Size()

		sectionInputConfigs = append(sectionInputConfigs, IgnitionNetworkInitScriptSectionInput{
			Primary:     network.NetworkConnectionIndex == networkConnection.PrimaryNetworkConnectionIndex,
			Network:     network.Network,
			IPAddress:   network.IPAddress,
			MACAddress:  network.MACAddress,
			NetmaskCidr: netmaskCidr,
			Gateway:     ipScope.Gateway,
			DNS1:        ipScope.DNS1,
			DNS2:        ipScope.DNS2,
		})
	}

	buff := bytes.Buffer{}
	if err = ignitionNetworkInitTemplate.Execute(&buff, sectionInputConfigs); err != nil {
		return "", errors.Wrapf(err, "Error rendering ignition network init template: [%s]", ignitionNetworkInitTemplate.Name())
	}
	return buff.String(), nil
}

func getVMName(machine *clusterv1.Machine, vcdMachine *infrav1beta3.VCDMachine, log logr.Logger) (string, error) {
	if vcdMachine.Spec.VmNamingTemplate == "" {
		return machine.Name, nil
	}

	vmNameTemplate, err := template.New("vmname").
		Funcs(sprig.TxtFuncMap()).
		Parse(vcdMachine.Spec.VmNamingTemplate)
	if err != nil {
		log.Error(err, "Error while parsing VmNamingTemplate of VCDMachine")
		return "", errors.Wrapf(err, "Error while parsing VmNamingTemplate of VCDMachine")
	}

	buf := new(bytes.Buffer)
	err = vmNameTemplate.Execute(buf, map[string]interface{}{
		"machine":    machine,
		"vcdMachine": vcdMachine,
	})
	if err != nil {
		log.Error(err, "Error while generating VM Name by using VmNamingTemplate of VCDMachine")
		return "", errors.Wrapf(err, "Error while generating VM Name by using VmNamingTemplate of VCDMachine")
	}

	return buf.String(), nil
}

// getPrimaryNetwork returns the primary network based on vm.NetworkConnectionSection.PrimaryNetworkConnectionIndex
// It is not possible to assume vm.NetworkConnectionSection.NetworkConnection[0] is the primary network when there are
// multiple networks attached to the VM.
func getPrimaryNetwork(vm *types.Vm) *types.NetworkConnection {
	for _, network := range vm.NetworkConnectionSection.NetworkConnection {
		if network.NetworkConnectionIndex == vm.NetworkConnectionSection.PrimaryNetworkConnectionIndex {
			return network
		}
	}

	return nil
}

// reconcileVMNetworks ensures that desired networks are attached to VMs
// networks[0] refers the primary network
func (r *VCDMachineReconciler) reconcileVMNetworks(vdcManager *vcdsdk.VdcManager, vApp *govcd.VApp, vm *govcd.VM, networks []string) error {
	connections, err := vm.GetNetworkConnectionSection()
	if err != nil {
		return errors.Wrapf(err, "Failed to get attached networks to VM")
	}

	desiredConnectionArray := make([]*types.NetworkConnection, len(networks))

	for index, ovdcNetwork := range networks {
		err = ensureNetworkIsAttachedToVApp(vdcManager, vApp, ovdcNetwork)
		if err != nil {
			return errors.Wrapf(err, "Error ensuring network [%s] is attached to vApp", ovdcNetwork)
		}

		desiredConnectionArray[index] = getNetworkConnection(connections, ovdcNetwork, r.Params.DefaultNetworkModeForNewVM)
	}

	if !containsTheSameElements(connections.NetworkConnection, desiredConnectionArray) {
		connections.NetworkConnection = desiredConnectionArray
		// update connection indexes for deterministic reconcilation
		connections.PrimaryNetworkConnectionIndex = 0
		for index, connection := range connections.NetworkConnection {
			connection.NetworkConnectionIndex = index
		}

		err = vm.UpdateNetworkConnectionSection(connections)
		if err != nil {
			return errors.Wrapf(err, "failed to update networks of VM")
		}
		// update vm.VM object for the rest of the flow, especially for getPrimaryNetwork function
		vm.VM.NetworkConnectionSection = connections
	}

	return nil
}

// containsTheSameElements checks all elements in the two array are the same regardless of order
func containsTheSameElements(array1 []*types.NetworkConnection, array2 []*types.NetworkConnection) bool {
	if len(array1) != len(array2) {
		return false
	}

OUTER:
	for _, element1 := range array1 {
		for _, element2 := range array2 {
			if reflect.DeepEqual(element1, element2) {
				continue OUTER
			}
		}

		return false
	}

	return true
}

func getNetworkConnection(connections *types.NetworkConnectionSection, ovdcNetwork string, defaultMode string) *types.NetworkConnection {

	for _, existingConnection := range connections.NetworkConnection {
		if existingConnection.Network == ovdcNetwork {
			return existingConnection
		}
	}

	return &types.NetworkConnection{
		Network:                 ovdcNetwork,
		NeedsCustomization:      false,
		IsConnected:             true,
		IPAddressAllocationMode: defaultMode,
		NetworkAdapterType:      "VMXNET3",
	}
}

func ensureNetworkIsAttachedToVApp(vdcManager *vcdsdk.VdcManager, vApp *govcd.VApp, ovdcNetworkName string) error {
	for _, vAppNetwork := range vApp.VApp.NetworkConfigSection.NetworkNames() {
		if vAppNetwork == ovdcNetworkName {
			return nil
		}
	}

	ovdcNetwork, err := vdcManager.Vdc.GetOrgVdcNetworkByName(ovdcNetworkName, true)
	if err != nil {
		return fmt.Errorf("unable to get ovdc network [%s]: [%v]", ovdcNetworkName, err)
	}

	_, err = vApp.AddOrgNetwork(&govcd.VappNetworkSettings{}, ovdcNetwork.OrgVDCNetwork, false)
	if err != nil {
		return fmt.Errorf("unable to add ovdc network [%v] to vApp [%s]: [%v]",
			ovdcNetwork, vApp.VApp.Name, err)
	}

	return nil
}

func (r *VCDMachineReconciler) getBootstrapData(ctx context.Context, machine *clusterv1.Machine) (string, string, error) {
	log := ctrl.LoggerFrom(ctx)
	if machine.Spec.Bootstrap.DataSecretName == nil {
		return "", "", errors.New("error retrieving bootstrap data: linked Machine's bootstrap.dataSecretName is nil")
	}

	s := &corev1.Secret{}
	key := client.ObjectKey{Namespace: machine.GetNamespace(), Name: *machine.Spec.Bootstrap.DataSecretName}
	if err := r.Client.Get(ctx, key, s); err != nil {
		return "", "", errors.Wrapf(err,
			"failed to retrieve bootstrap data secret for VCDMachine %s/%s",
			machine.GetNamespace(), machine.GetName())
	}

	value, ok := s.Data["value"]
	if !ok {
		return "", "", errors.New("error retrieving bootstrap data: secret value key is missing")
	}

	log.V(2).Info(fmt.Sprintf("Auto-generated bootstrap script: [%s]", string(value)))

	format, ok := s.Data["format"]
	if !ok {
		return "", "", errors.New("error retrieving bootstrap data: secret format key is missing")
	}

	log.Info(fmt.Sprintf("Auto-generated bootstrap format: [%s] script: [%s]", string(format), string(value)))

	return string(format), string(value), nil
}

func (r *VCDMachineReconciler) reconcileDelete(ctx context.Context, machine *clusterv1.Machine,
	vcdMachine *infrav1beta3.VCDMachine, vcdCluster *infrav1beta3.VCDCluster) (ctrl.Result, error) {
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
		controllerutil.RemoveFinalizer(vcdMachine, infrav1beta3.MachineFinalizer)
		return ctrl.Result{}, nil
	}

	ovdcName, ovdcNetworkName, err := r.getOVDCDetailsForMachine(ctx, machine, vcdMachine, vcdCluster)
	if err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "unable to get OVDC details of machine [%s]", vcdMachine.Name)
	}

	vcdClient, err := loginVCD(ctx, r.Client, vcdCluster, ovdcName, true)

	// close all idle connections when reconciliation is done
	defer func() {
		if vcdClient != nil && vcdClient.VCDClient != nil {
			vcdClient.VCDClient.Client.Http.CloseIdleConnections()
			log.Info(fmt.Sprintf("closed connection to the http client [%#v]", vcdClient.VCDClient.Client.Http))
		}
	}()
	if err != nil {
		log.Error(err, "error occurred while logging in to VCD")
		return ctrl.Result{}, errors.Wrapf(err, "error occurred while logging in to VCD: [%v]", err)
	}

	capvcdRdeManager := capisdk.NewCapvcdRdeManager(vcdClient, vcdCluster.Status.InfraId)
	if err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "Unable to create VCD client to reconcile infrastructure for the Machine [%s]", machine.Name)
	}

	if util.IsControlPlaneMachine(machine) {
		gateway, err := vcdsdk.NewGatewayManager(ctx, vcdClient, ovdcNetworkName,
			vcdCluster.Spec.LoadBalancerConfigSpec.VipSubnet, vcdClient.ClusterOVDCName)
		if err != nil {
			capvcdRdeManager.AddToErrorSet(ctx, capisdk.VCDMachineCreationError, "", machine.Name, fmt.Sprintf("%v", err))
			return ctrl.Result{}, errors.Wrapf(err, "failed to create gateway manager object while reconciling machine [%s]", vcdMachine.Name)
		}
		// remove the address from the lbpool
		log.Info("Deleting the control plane IP from the load balancer pool")
		lbPoolName := capisdk.GetLoadBalancerPoolNameUsingPrefix(
			capisdk.GetLoadBalancerPoolNamePrefix(vcdCluster.Name, vcdCluster.Status.InfraId), "tcp")
		virtualServiceName := capisdk.GetVirtualServiceNameUsingPrefix(
			capisdk.GetVirtualServiceNamePrefix(vcdCluster.Name, vcdCluster.Status.InfraId), "tcp")
		lbPoolRef, err := gateway.GetLoadBalancerPool(ctx, lbPoolName)
		if err != nil && err != govcd.ErrorEntityNotFound {
			capvcdRdeManager.AddToErrorSet(ctx, capisdk.LoadBalancerError, "", machine.Name, fmt.Sprintf("%v", err))

			return ctrl.Result{}, errors.Wrapf(err, "Error while deleting the infra resources of the machine [%s/%s]; failed to get load balancer pool [%s]", vcdCluster.Name, vcdMachine.Name, lbPoolName)
		}
		// Do not try to update the load balancer if lbPool is not found
		if err != govcd.ErrorEntityNotFound {
			controlPlaneIPs, err := gateway.GetLoadBalancerPoolMemberIPs(ctx, lbPoolRef)
			if err != nil {
				capvcdRdeManager.AddToErrorSet(ctx, capisdk.LoadBalancerError, "", machine.Name, fmt.Sprintf("%v", err))

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
					break
				}
			}
			resourcesAllocated := &cpiutil.AllocatedResourcesMap{}
			var oneArm *vcdsdk.OneArm = nil
			if vcdCluster.Spec.LoadBalancerConfigSpec.UseOneArm {
				oneArm = &OneArmDefault
			}

			// At this point the vcdCluster.Spec.ControlPlaneEndpoint should have been set correctly.
			// At this point the vcdCluster.Spec.ControlPlaneEndpoint should have been set correctly.
			// We are not using externalIp=vcdCluster.Spec.ControlPlaneEndpoint.Host because of a possible race between which controller picks ups according to the spec.
			// 1. If vcdcluster controller picks up vcdCluster.Spec.ControlPlaneEndpoint.Host, there is no issue as it will retrieve it from existing VCD VirtualService.
			// 2. If vcdmachine controller picks up first, then it would update the LB according to vcdCluster.Spec.ControlPlaneEndpoint.Host.
			// Hence, we are deciding to pass externalIp="" in this case, as UpdateVirtualService() would see it's an empty string, so it would just update the VS Object with what's already present.
			// Users should not be updating control plane IP after it has been created, so this is not a valid use case.
			// TODO: CAFV-143 - In the the future, ideally we should add ControlPlaneEndpoint.Host, ControlPlaneEndpoint.Port into VCDClusterStatus, and pass externalIp=vcdCluster.Status.ControlPlaneEndpoint.Host instead
			_, err = gateway.UpdateLoadBalancer(ctx, lbPoolName, virtualServiceName, updatedIPs,
				"", int32(vcdCluster.Spec.ControlPlaneEndpoint.Port), int32(vcdCluster.Spec.ControlPlaneEndpoint.Port),
				oneArm, !vcdCluster.Spec.LoadBalancerConfigSpec.UseOneArm, "TCP", resourcesAllocated)
			if err != nil {
				capvcdRdeManager.AddToErrorSet(ctx, capisdk.LoadBalancerError, "", machine.Name, fmt.Sprintf("%v", err))

				return ctrl.Result{}, errors.Wrapf(err,
					"Error while deleting the infra resources of the machine [%s/%s]; error deleting the control plane from the load balancer pool [%s]",
					vcdCluster.Name, vcdMachine.Name, lbPoolName)
			}
		}
		err = capvcdRdeManager.RdeManager.RemoveErrorByNameOrIdFromErrorSet(ctx, vcdsdk.ComponentCAPVCD, capisdk.LoadBalancerError, "", "")
		if err != nil {
			log.Error(err, "failed to remove LoadBalancerError from RDE", "rdeID", vcdCluster.Status.InfraId)
		}
	}

	vdcManager, err := vcdsdk.NewVDCManager(vcdClient, vcdClient.ClusterOrgName,
		vcdClient.ClusterOVDCName)
	if err != nil {
		capvcdRdeManager.AddToErrorSet(ctx, capisdk.VCDMachineError, "", machine.Name, fmt.Sprintf("failed to get vdcmanager: %v", err))

		return ctrl.Result{}, errors.Wrapf(err, "failed to create a vdc manager object when reconciling machine [%s]", vcdMachine.Name)
	}

	err = capvcdRdeManager.RdeManager.RemoveErrorByNameOrIdFromErrorSet(ctx, vcdsdk.ComponentCAPVCD, capisdk.VCDMachineError, "", machine.Name)
	if err != nil {
		log.Error(err, "failed to remove VCDMachineError from RDE", "rdeID", vcdCluster.Status.InfraId)
	}

	// get the vApp
	vAppName, err := CreateFullVAppName(ctx, r.Client, vdcManager.Vdc.Vdc.ID, vcdCluster, machine)
	if err != nil {
		log.Error(err, "error while creating vApp name", "vcdClusterName", vcdCluster.Name,
			"ovdcID", vdcManager.Vdc.Vdc.ID)
		return ctrl.Result{}, errors.Wrapf(err, "error creating vApp name using vcdClusterName [%s] and ovdcID [%s]",
			vcdCluster.Name, vdcManager.Vdc.Vdc.ID)
	}
	log.Info(fmt.Sprintf("Using VApp name [%s] for the machine [%s]", vAppName, machine.Name))

	vApp, err := vdcManager.Vdc.GetVAppByName(vAppName, true)
	if err != nil {
		if err == govcd.ErrorEntityNotFound {
			log.Error(err, "Error while deleting the machine; vApp not found")
		} else {
			capvcdRdeManager.AddToErrorSet(ctx, capisdk.VCDClusterVappCreationError, "", machine.Name, fmt.Sprintf("%v", err))

			return ctrl.Result{}, errors.Wrapf(err, "Error while deleting the machine [%s/%s]; failed to find vapp by name", vAppName, machine.Name)
		}
	}
	if vApp != nil {
		// Delete the VM if and only if rdeId (matches) present in the vApp
		// TODO: remove usages of VCDCluster.Status.VAppMetadataUpdated because VApp Metadata will be updated in VCDMachine controller but
		//   we don't updated the VCDCluster object from within VCDMachine controller.
		//if !vcdCluster.Status.VAppMetadataUpdated {
		//	updatedErr := capvcdRdeManager.AddToErrorSet(ctx, capisdk.VCDMachineError, "", machine.Name, fmt.Sprintf("rdeId is not presented in the vApp [%s]: %v", vcdCluster.Name, err))
		//	if updatedErr != nil {
		//		log.Error(updatedErr, "failed to add VCDMachineError into RDE", "rdeID", vcdCluster.Status.InfraId)
		//	}
		//	return ctrl.Result{}, errors.Errorf("Error occurred during the machine deletion; Metadata not found in vApp")
		//}
		metadataInfraId, err := vdcManager.GetMetadataByKey(vApp, CapvcdInfraId)
		if err != nil {
			capvcdRdeManager.AddToErrorSet(ctx, capisdk.VCDMachineError, "", machine.Name, fmt.Sprintf("failed to get metadata by key [%s]: %v", CapvcdInfraId, err))

			return ctrl.Result{}, errors.Errorf("Error occurred during fetching metadata in vApp")
		}
		// checking the metadata value and vcdCluster.Status.InfraId are equal or not
		if metadataInfraId != vcdCluster.Status.InfraId {
			capvcdRdeManager.AddToErrorSet(ctx, capisdk.VCDMachineError, "", machine.Name, fmt.Sprintf("%v", err))

			return ctrl.Result{}, errors.Wrapf(err,
				"Error occurred during the machine deletion; failed to delete vApp [%s]", vcdCluster.Name)
		}
		// Removed error: VCDClusterVappCreationError VCDMachineError
		err = capvcdRdeManager.RdeManager.RemoveErrorByNameOrIdFromErrorSet(ctx, vcdsdk.ComponentCAPVCD, capisdk.VCDClusterVappCreationError, "", "")
		if err != nil {
			log.Error(err, "failed to remove VCDClusterVappCreationError from RDE", "rdeID", vcdCluster.Status.InfraId)
		}
		err = capvcdRdeManager.RdeManager.RemoveErrorByNameOrIdFromErrorSet(ctx, vcdsdk.ComponentCAPVCD, capisdk.VCDMachineError, "", "")
		if err != nil {
			log.Error(err, "failed to remove VCDMachineError from RDE", "rdeID", vcdCluster.Status.InfraId)
		}
		// delete the vm
		vmName, err := getVMName(machine, vcdMachine, log)
		if err != nil {
			return ctrl.Result{}, err
		}
		vm, err := vApp.GetVMByName(vmName, true)
		if err != nil {
			if err == govcd.ErrorEntityNotFound {
				log.Error(err, "Error while deleting the machine; VM  not found")
			} else {
				capvcdRdeManager.AddToErrorSet(ctx, capisdk.VCDMachineDeletionError, "", machine.Name, fmt.Sprintf("%v", err))

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
						capvcdRdeManager.AddToErrorSet(ctx, capisdk.VCDMachineDeletionError, "", machine.Name, fmt.Sprintf("%v", err))

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
						capvcdRdeManager.AddToErrorSet(ctx, capisdk.VCDMachineDeletionError, "", machine.Name, fmt.Sprintf("%v", err))

						return ctrl.Result{}, fmt.Errorf("error waiting for task completion after reconfiguring vm: [%v]", err)
					}
				}
			}

			// in any case try to delete the machine
			log.Info("Deleting the infra VM of the machine")
			if err := vm.Delete(); err != nil {
				capvcdRdeManager.AddToErrorSet(ctx, capisdk.VCDMachineDeletionError, "", machine.Name, fmt.Sprintf("%v", err))

				return ctrl.Result{}, errors.Wrapf(err, "error deleting the machine [%s/%s]", vAppName, vm.VM.Name)
			}
		}
		log.Info("Successfully deleted infra resources of the machine")
		capvcdRdeManager.AddToEventSet(ctx, capisdk.InfraVmDeleted, "", machine.Name, "", true)

		err = capvcdRdeManager.RdeManager.RemoveErrorByNameOrIdFromErrorSet(ctx, vcdsdk.ComponentCAPVCD, capisdk.VCDMachineDeletionError, "", machine.Name)
		if err != nil {
			log.Error(err, "failed to remove VCDMachineDeletionError from RDE", "rdeID", vcdCluster.Status.InfraId)
		}
	}
	// Remove VM from VCDResourceSet of RDE
	rdeManager := vcdsdk.NewRDEManager(vcdClient, vcdCluster.Status.InfraId, capisdk.StatusComponentNameCAPVCD, release.Version)
	err = rdeManager.RemoveFromVCDResourceSet(ctx, vcdsdk.ComponentCAPVCD, VcdResourceTypeVM, machine.Name)
	if err != nil {
		log.Error(fmt.Errorf("error occurred when removing VM [%s] from VCD resource set: [%v]", machine.Name, err),
			"failed to remove VM from VCD resource set")
		capvcdRdeManager.AddToErrorSet(ctx, capisdk.RdeError, "", machine.Name,
			fmt.Sprintf("failed to delete VCD Resource [%s] of type [%s] from VCDResourceSet of RDE [%s]: [%v]",
				machine.Name, VcdResourceTypeVM, vcdCluster.Status.InfraId, err))

		// VCD has a bug where RDE update to VCD resource set may fail. Although VCD resource set in the RDE may
		// contain outdated information, the cluster creation won't be blocked.
	}
	err = capvcdRdeManager.RdeManager.RemoveErrorByNameOrIdFromErrorSet(ctx, vcdsdk.ComponentCAPVCD, capisdk.RdeError, "", machine.Name)
	if err != nil {
		log.Error(err, "failed to remove RdeError from RDE", "rdeID", vcdCluster.Status.InfraId)
	}

	controllerutil.RemoveFinalizer(vcdMachine, infrav1beta3.MachineFinalizer)
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *VCDMachineReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager,
	options controller.Options) error {
	clusterToVCDMachines, err := util.ClusterToObjectsMapper(mgr.GetClient(),
		&infrav1beta3.VCDMachineList{}, mgr.GetScheme())
	if err != nil {
		return err
	}

	c, err := ctrl.NewControllerManagedBy(mgr).
		For(&infrav1beta3.VCDMachine{}).
		WithOptions(options).
		WithEventFilter(predicates.ResourceNotPaused(ctrl.LoggerFrom(ctx))).
		Watches(
			&clusterv1.Machine{},
			handler.EnqueueRequestsFromMapFunc(util.MachineToInfrastructureMapFunc(
				infrav1beta3.GroupVersion.WithKind("VCDMachine")),
			),
		).
		Watches(
			&infrav1beta3.VCDCluster{},
			handler.EnqueueRequestsFromMapFunc(
				r.VCDClusterToVCDMachines,
			),
		).
		Build(r)
	if err != nil {
		return fmt.Errorf("unable to build controller for VCDMachine: [%v]", err)
	}

	if err = c.Watch(
		source.Kind(mgr.GetCache(), &clusterv1.Cluster{}),
		handler.EnqueueRequestsFromMapFunc(clusterToVCDMachines),
		predicates.ClusterUnpausedAndInfrastructureReady(ctrl.LoggerFrom(ctx)),
	); err != nil {
		return fmt.Errorf("unable to add a watch for ready clusters: [%v]", err)
	}

	return nil
}

// VCDClusterToVCDMachines is a handler.ToRequestsFunc to be used to enqueue
// requests for reconciliation of VCDMachines.
func (r *VCDMachineReconciler) VCDClusterToVCDMachines(_ context.Context, o client.Object) []reconcile.Request {
	var result []reconcile.Request
	c, ok := o.(*infrav1beta3.VCDCluster)
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

	labels := map[string]string{clusterv1.ClusterNameLabel: cluster.Name}
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
		result = append(result, reconcile.Request{
			NamespacedName: name,
		})
	}

	return result
}

func (r *VCDMachineReconciler) hasCloudInitExecutionFailedBefore(vcdClient *vcdsdk.Client, vm *govcd.VM) (bool, error) {
	vdcManager, err := vcdsdk.NewVDCManager(vcdClient, vcdClient.ClusterOrgName,
		vcdClient.ClusterOVDCName)
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
