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

package controllers

import (
	"context"
	"fmt"
	"github.com/pkg/errors"
	infrav1 "github.com/vmware/cluster-api-provider-cloud-director/api/v1alpha4"
	"github.com/vmware/cluster-api-provider-cloud-director/pkg/vcdclient"
	"github.com/vmware/go-vcloud-director/v2/govcd"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/klog"
	"os/exec"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha4"
	"sigs.k8s.io/cluster-api/test/infrastructure/docker/cloudinit"
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
	"sigs.k8s.io/kind/pkg/cluster/constants"
	"strings"
	"time"
)

// VCDMachineReconciler reconciles a VCDMachine object
type VCDMachineReconciler struct {
	client.Client
	//Scheme    *runtime.Scheme
	VcdClient *vcdclient.Client
}

//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=vcdmachines,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=vcdmachines/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=vcdmachines/finalizers,verbs=update
func (r *VCDMachineReconciler) Reconcile(ctx context.Context, req ctrl.Request) (_ ctrl.Result, rerr error) {
	log := ctrl.LoggerFrom(ctx)

	// your logic here
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

	// Fetch the VCD Cluster.
	vcdCluster := &infrav1.VCDCluster{}
	vcdClusterName := client.ObjectKey{
		Namespace: vcdMachine.Namespace,
		Name:      cluster.Spec.InfrastructureRef.Name,
	}
	if err := r.Client.Get(ctx, vcdClusterName, vcdCluster); err != nil {
		log.Info("VCDCluster is not available yet")
		return ctrl.Result{}, nil
	}

	log = log.WithValues("vcd-cluster", vcdCluster.Name)

	// Initialize the patch helper
	patchHelper, err := patch.NewHelper(vcdMachine, r)
	if err != nil {
		return ctrl.Result{}, err
	}
	// Always attempt to Patch the VCDMachine object and status after each reconciliation.
	defer func() {
		if err := patchVCDMachine(ctx, patchHelper, vcdMachine); err != nil {
			log.Error(err, "failed to patch VCDMachine")
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

	// Check if the infrastructure is ready, otherwise return and wait for the cluster object to be updated
	if !cluster.Status.InfrastructureReady {
		log.Info("Waiting for VCDCluster Controller to create cluster infrastructure")
		conditions.MarkFalse(vcdMachine, infrav1.ContainerProvisionedCondition,
			infrav1.WaitingForClusterInfrastructureReason, clusterv1.ConditionSeverityInfo, "")
		return ctrl.Result{}, nil
	}

	// Handle deleted machines
	if !vcdMachine.ObjectMeta.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, cluster, machine, vcdMachine, vcdCluster)
	}

	// Handle non-deleted machines
	return r.reconcileNormal(ctx, cluster, machine, vcdMachine, vcdCluster)
}

func patchVCDMachine(ctx context.Context, patchHelper *patch.Helper, vcdMachine *infrav1.VCDMachine) error {
	conditions.SetSummary(vcdMachine,
		conditions.WithConditions(
			infrav1.ContainerProvisionedCondition,
			infrav1.BootstrapExecSucceededCondition,
		),
		conditions.WithStepCounterIf(vcdMachine.ObjectMeta.DeletionTimestamp.IsZero()),
	)

	return patchHelper.Patch(
		ctx,
		vcdMachine,
		patch.WithOwnedConditions{Conditions: []clusterv1.ConditionType{
			clusterv1.ReadyCondition,
			infrav1.ContainerProvisionedCondition,
			infrav1.BootstrapExecSucceededCondition,
		}},
	)
}

func (r *VCDMachineReconciler) reconcileNormal(ctx context.Context, cluster *clusterv1.Cluster,
	machine *clusterv1.Machine, vcdMachine *infrav1.VCDMachine, vcdCluster *infrav1.VCDCluster) (res ctrl.Result, retErr error) {

	log := ctrl.LoggerFrom(ctx)

	if vcdMachine.Spec.ProviderID != nil {
		vcdMachine.Status.Ready = true
		conditions.MarkTrue(vcdMachine, infrav1.ContainerProvisionedCondition)
		return ctrl.Result{}, nil
	}

	if machine.Spec.Bootstrap.DataSecretName == nil {
		if !util.IsControlPlaneMachine(machine) && !conditions.IsTrue(cluster,
			clusterv1.ControlPlaneInitializedCondition) {

			log.Info("Waiting for the control plane to be initialized")
			conditions.MarkFalse(vcdMachine, infrav1.ContainerProvisionedCondition,
				clusterv1.WaitingForControlPlaneAvailableReason, clusterv1.ConditionSeverityInfo, "")
			return ctrl.Result{}, nil
		}

		log.Info("Waiting for the Bootstrap provider controller to set bootstrap data")
		conditions.MarkFalse(vcdMachine, infrav1.ContainerProvisionedCondition,
			infrav1.WaitingForBootstrapDataReason, clusterv1.ConditionSeverityInfo, "")
		return ctrl.Result{}, nil
	}

	role := constants.WorkerNodeRoleValue
	if util.IsControlPlaneMachine(machine) {
		role = constants.ControlPlaneNodeRoleValue
	}
	klog.Infof("Role of machine is [%s]\n", role)

	patchHelper, err := patch.NewHelper(vcdMachine, r.Client)
	if err != nil {
		return ctrl.Result{}, err
	}
	conditions.MarkTrue(vcdMachine, infrav1.ContainerProvisionedCondition)

	if !conditions.Has(vcdMachine, infrav1.BootstrapExecSucceededCondition) {
		conditions.MarkFalse(vcdMachine, infrav1.BootstrapExecSucceededCondition,
			infrav1.BootstrappingReason, clusterv1.ConditionSeverityInfo, "")
		if err := patchVCDMachine(ctx, patchHelper, vcdMachine); err != nil {
			return ctrl.Result{}, errors.Wrap(err, "failed to patch VCDMachine")
		}
	}
	defaultVdcName := r.VcdClient.VcdAuthConfig.VDC
	defaultOrgName := r.VcdClient.VcdAuthConfig.Org
	vdcManager := vcdclient.VdcManager{
		VdcName: defaultVdcName,
		OrgName: defaultOrgName,
		Client:  r.VcdClient,
		Vdc:     r.VcdClient.Vdc,
	}
	// Replace the default org and ovdc with the user specified inputs.
	if vcdCluster.Spec.Ovdc != "" && vcdCluster.Spec.Ovdc != defaultVdcName {
		vdcManager.VdcName = vcdCluster.Spec.Ovdc
		err := vdcManager.CacheVdcDetails()
		if err != nil {
			return ctrl.Result{}, errors.Wrapf(err, "unable to cache vdc details [%s]: [%v]", vcdCluster.Spec.Ovdc, err)
		}
	}

	// Create the vApp if not existing yet
	vAppName := cluster.Name
	vApp, err := vdcManager.GetOrCreateVApp(vAppName, r.VcdClient.NetworkName)
	if err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "unable to create vApp for new cluster")
	}

	guestCustScript := ""
	if !vcdMachine.Spec.Bootstrapped {
		guestCustScriptTemplate := `
#!/usr/bin/env bash -x
echo "Command called with arguments [$@] at time $(date)" >> /root/output.txt
if [ -f "/.guest-customization-post-reboot-pending" ]
then
	echo "Reboot pending, hence will do nothing."  >> /root/output.txt
elif [ "$1" = "postcustomization" ]
then
	hostnamectl set-hostname %s
	echo 'net.ipv6.conf.all.disable_ipv6 = 1' >> /etc/sysctl.conf
  	echo 'net.ipv6.conf.default.disable_ipv6 = 1' >> /etc/sysctl.conf
  	echo 'net.ipv6.conf.lo.disable_ipv6 = 1' >> /etc/sysctl.conf
  	sudo sysctl -p

  	# also remove ipv6 localhost entry from /etc/hosts
  	sed -i 's/::1/127.0.0.1/g' /etc/hosts || true

	\rm -f /etc/containerd/config.toml || true
	systemctl restart containerd
	systemctl restart kubelet
    \rm -rf /etc/kubernetes/*
    for image in "coredns" "etcd" "kube-proxy" "kube-apiserver" "kube-controller-manager" "kube-scheduler"
      do
        image_ref=$(ctr -n=k8s.io image list | cut -d" " -f1 | grep $image)
        ref_path=$(echo $image_ref | sed 's/:.*//')
        new_tag_version=$(echo $image_ref | sed 's/.*://' | sed 's/_/-/')
        ctr -n=k8s.io image tag $image_ref $ref_path:$new_tag_version
      done

	%s
else
	echo "Skipping script since postcustomization is not involved." >> /root/output.txt
fi
exit 0
`
		bootstrapJinjaScript, err := r.getBootstrapData(ctx, machine)
		if err != nil {
			return ctrl.Result{}, errors.Wrapf(err, "unable to get bootstrap data for machine [%s]",
				machine.Name)
		}

		bootstrapShellScript, err := JinjaToShell(bootstrapJinjaScript)
		if err != nil {
			return ctrl.Result{}, errors.Wrapf(err,
				"unable to convert bootstrap jinja script to shell script [%s]", bootstrapJinjaScript)
		}

		guestCustScript = fmt.Sprintf(guestCustScriptTemplate, machine.Name, bootstrapShellScript)
	}
	klog.Infof("Guest Customization Script: [%s]", guestCustScript)

	vmExists := true
	vm, err := vApp.GetVMByName(machine.Name, true)
	if err != nil && err != govcd.ErrorEntityNotFound {
		return ctrl.Result{}, errors.Wrapf(err, "unable to query for VM [%s] in vApp [%s]",
			vm.VM.Name, vAppName)
	} else if err == govcd.ErrorEntityNotFound {
		vmExists = false
	}
	if !vmExists {
		err = vdcManager.AddNewVM(machine.Name, vApp.VApp.Name, 1,
			vcdMachine.Spec.Catalog, vcdMachine.Spec.Template,
			vcdclient.CSEPlacementPolicy, vcdMachine.Spec.ComputePolicy, guestCustScript)
		if err != nil {
			return ctrl.Result{}, errors.Wrapf(err, "unable to create VM [%s] in vApp [%s]",
				machine.Name, vApp.VApp.Name)
		}
		vm, err = vApp.GetVMByName(machine.Name, true)
		if err != nil {
			return ctrl.Result{}, errors.Wrapf(err, "unable to find newly created VM [%s] in vApp [%s]",
				vm.VM.Name, vAppName)
		}
	}

	// set address in machine status
	if vm.VM == nil ||
		vm.VM.NetworkConnectionSection == nil ||
		len(vm.VM.NetworkConnectionSection.NetworkConnection) == 0 ||
		vm.VM.NetworkConnectionSection.NetworkConnection[0] == nil ||
		vm.VM.NetworkConnectionSection.NetworkConnection[0].IPAddress == "" {

		klog.Errorf("Failed to get the machine address of vm [%#v]", vm)
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}
	machineAddress := vm.VM.NetworkConnectionSection.NetworkConnection[0].IPAddress
	gateway := &vcdclient.GatewayManager{
		NetworkName:        r.VcdClient.NetworkName,
		Client:             r.VcdClient,
		GatewayRef:         r.VcdClient.GatewayRef,
		NetworkBackingType: r.VcdClient.NetworkBackingType,
	}
	// Replace the default ovdc network with the user specified input.
	if vcdCluster.Spec.OvdcNetwork != "" && vcdCluster.Spec.OvdcNetwork != r.VcdClient.NetworkName {
		gateway.NetworkName = vcdCluster.Spec.OvdcNetwork
		err := gateway.CacheGatewayDetails(ctx)
		if err != nil {
			return ctrl.Result{}, errors.Wrapf(err, "unable to cache ovdc network details [%s]: [%v]", vcdCluster.Spec.OvdcNetwork, err)
		}
	}

	// update dnat rule to point to control plane node
	// update loadbalancer pool with the IP of the control plane node as a new member.
	if util.IsControlPlaneMachine(machine) {
		lbPoolName := cluster.Name + "-" + vcdCluster.Status.ClusterRDEId + "-tcp"
		lbPoolRef, err := gateway.GetLoadBalancerPool(ctx, lbPoolName)
		if err != nil {
			return ctrl.Result{}, errors.Wrapf(err, "Failed to get load balancer pool by name [%s]: [%v]", lbPoolName, err)
		}
		controlPlaneIPs, err := gateway.GetLoadBalancerPoolMemberIPs(ctx, lbPoolRef)
		if err != nil {
			return ctrl.Result{}, errors.Wrapf(err,
				"unexpected error retrieving the controlplane members from the load balancer pool [%s] in gateway [%s]: [%v]",
				lbPoolName, r.VcdClient.GatewayRef.Name, err)
		}

		updatedIPs := append(controlPlaneIPs, machineAddress)
		err = gateway.UpdateLoadBalancer(ctx, lbPoolName, updatedIPs, int32(6443))
		if err != nil {
			return ctrl.Result{}, errors.Wrapf(err,
				"unexpected error while updating the controlplane load balancer pool [%s] in gateway [%s]: [%v]",
				lbPoolName, r.VcdClient.GatewayRef.Name, err)
		}
	}

	// TODO: Make it more deterministic by checking for pre-customization status and then rebooting
	// Hack: Waiting for 30s so that pre-customization phase is complete.
	time.Sleep(30 * time.Second)
	// Reboot needed so that guest customization script can be executed
	err = r.VcdClient.RebootVm(vm)
	if err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "failed to reboot vm [%s]", vm.VM.Name)
	}

	// ensure that VM is booted and in the right state before marking bootstrapped
	if err = vdcManager.WaitForGuestScriptCompletion(machine.Name, vAppName); err != nil {
		return ctrl.Result{}, errors.Wrapf(err,
			"unable to wait for guest cust script completion in vm [%s]", machine.Name)
	}

	// TODO(Aniruddha): enable disk uuid
	vcdMachine.Spec.Bootstrapped = true
	conditions.MarkTrue(vcdMachine, infrav1.BootstrapExecSucceededCondition)

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

	// Set ProviderID so the Cluster API Machine Controller can pull it
	providerID := fmt.Sprintf("%s://%s", infrav1.VCDProviderID, vm.VM.ID)
	vcdMachine.Spec.ProviderID = &providerID
	vcdMachine.Status.Ready = true
	conditions.MarkTrue(vcdMachine, infrav1.ContainerProvisionedCondition)

	return ctrl.Result{}, nil
}

func (r *VCDMachineReconciler) getBootstrapData(ctx context.Context, machine *clusterv1.Machine) (string, error) {
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

	klog.Infof("Init script: [%s]", string(value))

	return string(value), nil
}

func (r *VCDMachineReconciler) reconcileDelete(ctx context.Context, cluster *clusterv1.Cluster, machine *clusterv1.Machine,
	vcdMachine *infrav1.VCDMachine, vcdCluster *infrav1.VCDCluster) (ctrl.Result, error) {

	patchHelper, err := patch.NewHelper(vcdMachine, r.Client)
	if err != nil {
		return ctrl.Result{}, err
	}

	conditions.MarkFalse(vcdMachine, infrav1.ContainerProvisionedCondition,
		clusterv1.DeletingReason, clusterv1.ConditionSeverityInfo, "")
	if err := patchVCDMachine(ctx, patchHelper, vcdMachine); err != nil {
		return ctrl.Result{}, errors.Wrap(err, "failed to patch VCDMachine")
	}
	gateway := &vcdclient.GatewayManager{
		NetworkName:        r.VcdClient.NetworkName,
		Client:             r.VcdClient,
		GatewayRef:         r.VcdClient.GatewayRef,
		NetworkBackingType: r.VcdClient.NetworkBackingType,
	}
	// Replace the default ovdc network with the user specified input.
	if vcdCluster.Spec.OvdcNetwork != "" && vcdCluster.Spec.OvdcNetwork != r.VcdClient.NetworkName {
		gateway.NetworkName = vcdCluster.Spec.OvdcNetwork
		err := gateway.CacheGatewayDetails(ctx)
		if err != nil {
			return ctrl.Result{}, errors.Wrapf(err, "unable to cache ovdc network details [%s]: [%v]", vcdCluster.Spec.OvdcNetwork, err)
		}
	}
	if util.IsControlPlaneMachine(machine) {
		// remove the address from the lbpool
		lbPoolName := cluster.Name + "-" + r.VcdClient.ClusterID + "-tcp"
		lbPoolRef, err := gateway.GetLoadBalancerPool(ctx, lbPoolName)
		if err != nil && err != govcd.ErrorEntityNotFound {
			return ctrl.Result{}, errors.Wrapf(err, "failed to get load balancer pool [%s]: [%v]", lbPoolName, err)
		}
		// Do not try to update the load balancer if lbPool is not found
		if err != govcd.ErrorEntityNotFound {
			controlPlaneIPs, err := gateway.GetLoadBalancerPoolMemberIPs(ctx, lbPoolRef)
			if err != nil {
				return ctrl.Result{}, errors.Wrapf(err,
					"unexpected error retrieving the controlplane members from the load balancer pool [%s] in gateway [%s]: [%v]",
					lbPoolName, r.VcdClient.GatewayRef.Name, err)
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
			err = gateway.UpdateLoadBalancer(ctx, lbPoolName, updatedIPs, int32(6443))
			if err != nil {
				return ctrl.Result{}, errors.Wrapf(err,
					"unexpected error while updating the controlplane load balancer pool [%s] in gateway [%s]: [%v]",
					lbPoolName, r.VcdClient.GatewayRef.Name, err)
			}
		}
	}
	defaultVdcName := r.VcdClient.VcdAuthConfig.VDC
	defaultOrgName := r.VcdClient.VcdAuthConfig.Org
	vdcManager := vcdclient.VdcManager{
		VdcName: defaultVdcName,
		OrgName: defaultOrgName,
		Client:  r.VcdClient,
		Vdc:     r.VcdClient.Vdc,
	}
	// Replace the default org and ovdc with the user specified inputs.
	if vcdCluster.Spec.Ovdc != "" && vcdCluster.Spec.Ovdc != defaultVdcName {
		vdcManager.VdcName = vcdCluster.Spec.Ovdc
		err := vdcManager.CacheVdcDetails()
		if err != nil {
			return ctrl.Result{}, errors.Wrapf(err, "unable to cache vdc details [%s]: [%v]", vcdCluster.Spec.Ovdc, err)
		}
	}

	// get the vApp
	vAppName := cluster.Name
	vApp, err := vdcManager.Vdc.GetVAppByName(vAppName, true)
	if err != nil && err != govcd.ErrorEntityNotFound {
		if err == govcd.ErrorEntityNotFound {
			klog.Infof("vApp by name [%s] not found.", vAppName)
		} else {
			return ctrl.Result{}, errors.Wrapf(err, "failed to find vapp by name [%s].", vAppName)
		}
	}
	if vApp != nil {
		// delete the vm
		vm, err := vApp.GetVMByName(machine.Name, true)
		if err != nil {
			if err == govcd.ErrorEntityNotFound {
				klog.Infof("VM by name [%s] not found in vApp [%s].", machine.Name, vAppName)
			} else {
				return ctrl.Result{}, errors.Wrapf(err, "unable to check if vm [%s] exists in vapp [%s]",
					machine.Name, vAppName)
			}
		}
		if vm != nil {
			// delete the machine
			if err := vm.Delete(); err != nil {
				return ctrl.Result{}, errors.Wrapf(err, "failed to delete VCDMachine [%s]", vm.VM.Name)
			}
		}
		klog.Infof("successfully deleted VM [%s]", machine.Name)
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
		panic(fmt.Sprintf("Expected a VCDCluster but got a %T", o))
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

// JinjaToShell converts from jinja to shell
func JinjaToShell(jinjaScript string) (string, error) {
	commands, err := cloudinit.Commands([]byte(jinjaScript))
	if err != nil {
		klog.Errorf("unable to parse jinja config [%s]", jinjaScript)
		return "", errors.Wrap(err, "failed to join a control plane node with kubeadm")
	}

	shellScript := make([]string, len(commands))
	for idx, command := range commands {
		cmd := exec.Command(command.Cmd, command.Args...)
		shellScript[idx] = strings.TrimPrefix(
			strings.TrimSpace(
				strings.Join(cmd.Args[:], " "),
			),
			"/bin/sh -c ",
		)
		if command.Stdin != "" {
			shellScript[idx] = fmt.Sprintf("echo -n '%s' | %s", command.Stdin, shellScript[idx])
		}
	}

	return strings.Join(shellScript, "\n"), nil
}
