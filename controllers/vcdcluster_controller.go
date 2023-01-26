/*
   Copyright 2021 VMware, Inc.
   SPDX-License-Identifier: Apache-2.0
*/

package controllers

import (
	"context"
	_ "embed"
	"fmt"
	rdeType "github.com/vmware/cluster-api-provider-cloud-director/pkg/vcdtypes/rde_type_1_2_0"
	"net/http"
	"os"
	"reflect"
	"strings"
	"time"

	"github.com/antihax/optional"
	"github.com/blang/semver"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	vcdsdkutil "github.com/vmware/cloud-provider-for-cloud-director/pkg/util"
	"github.com/vmware/cloud-provider-for-cloud-director/pkg/vcdsdk"
	swagger "github.com/vmware/cloud-provider-for-cloud-director/pkg/vcdswaggerclient"
	infrav1 "github.com/vmware/cluster-api-provider-cloud-director/api/v1beta2"
	"github.com/vmware/cluster-api-provider-cloud-director/pkg/capisdk"
	vcdutil "github.com/vmware/cluster-api-provider-cloud-director/pkg/util"
	"github.com/vmware/cluster-api-provider-cloud-director/release"
	"github.com/vmware/go-vcloud-director/v2/govcd"
	"github.com/vmware/go-vcloud-director/v2/types/v56"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	kcpv1 "sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1beta1"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/conditions"
	kcfg "sigs.k8s.io/cluster-api/util/kubeconfig"
	"sigs.k8s.io/cluster-api/util/patch"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	EnvSkipRDE = "CAPVCD_SKIP_RDE"

	RDEStatusResolved = "RESOLVED"
	VCDLocationHeader = "Location"

	ClusterApiStatusPhaseReady    = "Ready"
	ClusterApiStatusPhaseNotReady = "Not Ready"
	CapvcdInfraId                 = "CapvcdInfraId"

	NoRdePrefix     = `NO_RDE_`
	VCDResourceVApp = "VApp"

	TcpPort = 6443
)

var (
	CAPVCDEntityTypeID = fmt.Sprintf("urn:vcloud:type:%s:%s:%s", capisdk.CAPVCDTypeVendor, capisdk.CAPVCDTypeNss, rdeType.CapvcdRDETypeVersion)
	OneArmDefault      = vcdsdk.OneArm{
		StartIP: "192.168.8.2",
		EndIP:   "192.168.8.100",
	}
	SkipRDE = vcdutil.Str2Bool(os.Getenv(EnvSkipRDE))
)

// VCDClusterReconciler reconciles a VCDCluster object
type VCDClusterReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=vcdclusters,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=vcdclusters/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=vcdclusters/finalizers,verbs=update
//+kubebuilder:rbac:groups=cluster.x-k8s.io,resources=clusters;clusters/status,verbs=get;list;watch
//+kubebuilder:rbac:groups=cluster.x-k8s.io,resources=machines;machines/status,verbs=get;list;watch
//+kubebuilder:rbac:groups="",resources=secrets;,verbs=get;list;watch
//+kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch
//+kubebuilder:rbac:groups="",resources=events,verbs=get;list;watch;create;update;patch
//+kubebuilder:rbac:groups=addons.cluster.x-k8s.io,resources=clusterresourcesetbindings,verbs=get;list;watch
//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=vcdmachines,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=vcdmachines/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=vcdmachines/finalizers,verbs=update
//+kubebuilder:rbac:groups=controlplane.cluster.x-k8s.io,resources=kubeadmcontrolplanes,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=vcdmachinetemplates,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=cluster.x-k8s.io,resources=machinedeployments,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=bootstrap.cluster.x-k8s.io,resources=kubeadmconfigtemplates,verbs=get;list;watch;create;update;patch;delete

func (r *VCDClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (_ ctrl.Result, rerr error) {
	log := ctrl.LoggerFrom(ctx)
	// Fetch the VCDCluster instance
	vcdCluster := &infrav1.VCDCluster{}
	// remove the trailing '/'
	vcdCluster.Spec.Site = strings.TrimRight(vcdCluster.Spec.Site, "/")
	if err := r.Client.Get(ctx, req.NamespacedName, vcdCluster); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	clusterBeingDeleted := !vcdCluster.DeletionTimestamp.IsZero()

	// Fetch the Cluster.
	cluster, err := util.GetOwnerCluster(ctx, r.Client, vcdCluster.ObjectMeta)
	if err != nil {
		return ctrl.Result{}, err
	}
	if cluster == nil {
		log.Info("Waiting for Cluster Controller to set OwnerRef on VCDCluster")
		if !clusterBeingDeleted {
			return ctrl.Result{}, nil
		}

		log.Info("Continuing to delete cluster since DeletionTimestamp is set")
	}
	patchHelper, err := patch.NewHelper(vcdCluster, r.Client)
	if err != nil {
		return ctrl.Result{}, err
	}
	defer func() {
		if err := patchVCDCluster(ctx, patchHelper, vcdCluster); err != nil {
			log.Error(err, "Failed to patch VCDCluster")
			if rerr == nil {
				rerr = err
			}
		}
		log.V(3).Info("Cleanly patched VCD cluster.", "infra ID", vcdCluster.Status.InfraId)
	}()

	if !controllerutil.ContainsFinalizer(vcdCluster, infrav1.ClusterFinalizer) {
		controllerutil.AddFinalizer(vcdCluster, infrav1.ClusterFinalizer)
		return ctrl.Result{}, nil
	}

	if clusterBeingDeleted {
		return r.reconcileDelete(ctx, vcdCluster)
	}

	return r.reconcileNormal(ctx, cluster, vcdCluster)
}

func patchVCDCluster(ctx context.Context, patchHelper *patch.Helper, vcdCluster *infrav1.VCDCluster) error {
	conditions.SetSummary(vcdCluster,
		conditions.WithConditions(
			LoadBalancerAvailableCondition,
		),
		conditions.WithStepCounterIf(vcdCluster.ObjectMeta.DeletionTimestamp.IsZero()),
	)

	return patchHelper.Patch(
		ctx,
		vcdCluster,
		patch.WithOwnedConditions{Conditions: []clusterv1.ConditionType{
			clusterv1.ReadyCondition,
			LoadBalancerAvailableCondition,
		}},
	)
}

func addLBResourcesToVCDResourceSet(ctx context.Context, rdeManager *vcdsdk.RDEManager, resourcesAllocated *vcdsdkutil.AllocatedResourcesMap, externalIP string) error {
	for _, key := range []string{vcdsdk.VcdResourceDNATRule, vcdsdk.VcdResourceVirtualService,
		vcdsdk.VcdResourceLoadBalancerPool, vcdsdk.VcdResourceAppPortProfile} {
		if values := resourcesAllocated.Get(key); values != nil {
			for _, value := range values {
				additionalDetails := make(map[string]interface{})
				if key == vcdsdk.VcdResourceVirtualService {
					additionalDetails = map[string]interface{}{
						"virtualIP": externalIP,
					}
				}
				err := rdeManager.AddToVCDResourceSet(ctx, vcdsdk.ComponentCAPVCD, key,
					value.Name, value.Id, additionalDetails)
				if err != nil {
					return fmt.Errorf("failed to add resource [%s] of type [%s] to VCDResourceSet of RDE [%s]: [%v]",
						value.Name, key, rdeManager.ClusterID, err)
				}
			}
		}
	}
	return nil
}

// On VCDCluster reconciliation, we either create a new Infra ID or use an existing Infra ID from the VCDCluster object.
// The values for infra ID is not expected to change. rdeVersionInUse is not expected to change too unless the CAPVCD is being upgraded and the new
// CAPVCD version makes use of a higher RDE version.
// Derived values for infraID and rdeVersionInUse are essentially the final computed values - i.e either created or picked from the VCDCluster object.
// validateDerivedRDEProperties makes sure the infra ID and the RDE version in-use doesn't change to unexpected values over different reconciliations.
func validateDerivedRDEProperties(vcdCluster *infrav1.VCDCluster, infraID string, rdeVersionInUse string) error {
	// If the RDEVersionInUse is NO_RDE_ then there RDEVersionInUse cannot change
	// If the RDEVersionInUse is a semantic version, RDEVersionInUse can only be upgraded to a higher version
	// InfraID is not expected to change
	if infraID == "" || rdeVersionInUse == "" {
		return fmt.Errorf("empty values derived for infraID or RDEVersionInUse - InfraID: [%s], RDEVersionInUse: [%s]",
			infraID, rdeVersionInUse)
	}
	if vcdCluster.Spec.RDEId != "" && vcdCluster.Spec.RDEId != infraID {
		return fmt.Errorf("derived infraID [%s] is different from the infraID in VCDCluster spec [%s]",
			infraID, vcdCluster.Spec.RDEId)
	}
	if vcdCluster.Status.InfraId != "" && vcdCluster.Status.InfraId != infraID {
		// infra ID cannot change
		return fmt.Errorf("derived infraID [%s] is different from the infraID in VCDCluster status [%s]",
			infraID, vcdCluster.Status.InfraId)
	}
	if vcdCluster.Status.RdeVersionInUse != "" && vcdCluster.Status.RdeVersionInUse != NoRdePrefix {
		statusRdeVersion, err := semver.New(vcdCluster.Status.RdeVersionInUse)
		if err != nil {
			return fmt.Errorf("invalid RDE version [%s] in VCDCluster status", vcdCluster.Status.RdeVersionInUse)
		}
		newRdeVersion, err := semver.New(rdeVersionInUse)
		if err != nil {
			return fmt.Errorf("invalid RDE verison [%s] derived", rdeVersionInUse)
		}
		if newRdeVersion.LT(*statusRdeVersion) {
			return fmt.Errorf("derived RDE version [%s] is lesser than RDE version in VCDCluster status [%s]",
				rdeVersionInUse, vcdCluster.Status.RdeVersionInUse)
		}
	}

	// don't validate rde version in use if it is empty. Empty value for vcdCluster.Status.RdeVersionInUse may mean
	// that vcdCluster status has not been updated with RdeVersionInUse
	if vcdCluster.Status.RdeVersionInUse == "" {
		return nil
	}

	// If vcdCluster.Status.RdeVersionInUse and the derived rdeVersionInUse are both NO_RDE_ or if both are not NO_RDE_
	// then don't return error
	if (vcdCluster.Status.RdeVersionInUse == NoRdePrefix && rdeVersionInUse == NoRdePrefix) ||
		(vcdCluster.Status.RdeVersionInUse != NoRdePrefix && rdeVersionInUse != NoRdePrefix) {
		return nil
	}

	// rdeVersionInUse cannot change from a valid semantic version (valid RDE) to NO_RDE_ (RDE creation skipped). This can occur
	// when the there is an update to VCDCluster object from an external source, some misconfiguration or a bug in the logic to obtain infraID/rdeVersionInUse.
	return fmt.Errorf("stored and derived RDE versions mismatched for the cluster [%s]: status says [%s] while derived version says [%s]",
		vcdCluster.Name, vcdCluster.Status.RdeVersionInUse, rdeVersionInUse)
}

// TODO: Remove uncommented code when decision to only keep capi.yaml as part of RDE spec is finalized
func (r *VCDClusterReconciler) constructCapvcdRDE(ctx context.Context, cluster *clusterv1.Cluster,
	vcdCluster *infrav1.VCDCluster, vdc *types.Vdc, vcdOrg *types.Org) (*swagger.DefinedEntity, error) {

	if vdc == nil {
		return nil, fmt.Errorf("VDC cannot be nil")
	}
	if vcdOrg == nil {
		return nil, fmt.Errorf("org cannot be nil")
	}
	kcpList, err := getAllKubeadmControlPlaneForCluster(ctx, r.Client, *cluster)
	if err != nil {
		return nil, fmt.Errorf("error getting KubeadmControlPlane objects for cluster [%s]: [%v]", vcdCluster.Name, err)
	}

	// we assume that there is only one kcp object for a cluster.
	// TODO: need to update the logic for multiple kcp objects in the cluster
	kubernetesVersion := ""
	for _, kcp := range kcpList.Items {
		kubernetesVersion = kcp.Spec.Version
	}

	mdList, err := getAllMachineDeploymentsForCluster(ctx, r.Client, *cluster)
	if err != nil {
		return nil, fmt.Errorf("error getting all machine deployment objects for the cluster [%s]: [%v]", vcdCluster.Name, err)
	}
	ready, err := hasClusterReconciledToDesiredK8Version(ctx, r.Client, vcdCluster.Name, kcpList, mdList, kubernetesVersion)
	if err != nil {
		return nil, fmt.Errorf("error occurred while determining the value for the ready flag for cluster [%s]: [%v]", vcdCluster.Name, err)
	}

	orgList := []rdeType.Org{
		rdeType.Org{
			Name: vcdOrg.Name,
			ID:   vcdOrg.ID,
		},
	}
	ovdcList := []rdeType.Ovdc{
		rdeType.Ovdc{
			Name:        vdc.Name,
			ID:          vdc.ID,
			OvdcNetwork: vcdCluster.Spec.OvdcNetwork,
		},
	}

	rde := &swagger.DefinedEntity{
		EntityType: CAPVCDEntityTypeID,
		Name:       vcdCluster.Name,
	}
	capvcdEntity := rdeType.CAPVCDEntity{
		Kind:       capisdk.CAPVCDClusterKind,
		ApiVersion: capisdk.CAPVCDClusterEntityApiVersion,
		Metadata: rdeType.Metadata{
			Name: vcdCluster.Name,
			Org:  vcdOrg.Name,
			Vdc:  vdc.Name,
			Site: vcdCluster.Spec.Site,
		},
		Spec: rdeType.CAPVCDSpec{},
		Status: rdeType.Status{
			CAPVCDStatus: rdeType.CAPVCDStatus{
				Phase: ClusterApiStatusPhaseNotReady,
				// TODO: Discuss with sahithi if "kubernetes" needs to be removed from the RDE.
				Kubernetes: kubernetesVersion,
				ClusterAPIStatus: rdeType.ClusterApiStatus{
					Phase:        "",
					ApiEndpoints: []rdeType.ApiEndpoints{},
				},
				NodePool:               nil,
				CapvcdVersion:          release.CAPVCDVersion,
				UseAsManagementCluster: vcdCluster.Spec.UseAsManagementCluster,
				K8sNetwork: rdeType.K8sNetwork{
					Pods: rdeType.Pods{
						CidrBlocks: cluster.Spec.ClusterNetwork.Pods.CIDRBlocks,
					},
					Services: rdeType.Services{
						CidrBlocks: cluster.Spec.ClusterNetwork.Services.CIDRBlocks,
					},
				},
				ParentUID: vcdCluster.Spec.ParentUID,
				VcdProperties: rdeType.VCDProperties{
					Site: vcdCluster.Spec.Site,
					Org:  orgList,
					Ovdc: ovdcList,
				},
				CapiStatusYaml:             "",
				ClusterResourceSetBindings: nil,
				CreatedByVersion:           release.CAPVCDVersion,
				Upgrade: rdeType.Upgrade{
					Current: &rdeType.K8sInfo{
						K8sVersion: kubernetesVersion,
						TkgVersion: getTKGVersion(cluster),
					},
					Previous: nil,
					Ready:    ready,
				},
			},
		},
	}

	// convert CAPVCDEntity to map[string]interface{} type
	capvcdEntityMap, err := vcdutil.ConvertCAPVCDEntityToMap(&capvcdEntity)
	if err != nil {
		return nil, fmt.Errorf("failed to convert CAPVCD entity to Map: [%v]", err)
	}

	rde.Entity = capvcdEntityMap
	return rde, nil
}

func (r *VCDClusterReconciler) constructAndCreateRDEFromCluster(ctx context.Context, workloadVCDClient *vcdsdk.Client, cluster *clusterv1.Cluster, vcdCluster *infrav1.VCDCluster) (string, error) {
	log := ctrl.LoggerFrom(ctx)

	org, err := workloadVCDClient.VCDClient.GetOrgByName(vcdCluster.Spec.Org)
	if err != nil {
		return "", fmt.Errorf("failed to get org by name [%s]", vcdCluster.Spec.Org)
	}
	if org == nil || org.Org == nil {
		return "", fmt.Errorf("found nil org when getting org by name [%s]", vcdCluster.Spec.Org)
	}
	rde, err := r.constructCapvcdRDE(ctx, cluster, vcdCluster, workloadVCDClient.VDC.Vdc, org.Org)
	if err != nil {
		return "", fmt.Errorf("error occurred while constructing RDE payload for the cluster [%s]: [%v]", vcdCluster.Name, err)
	}
	resp, err := workloadVCDClient.APIClient.DefinedEntityApi.CreateDefinedEntity(ctx, *rde,
		rde.EntityType, org.Org.ID, nil)
	if err != nil {
		return "", fmt.Errorf("error occurred during RDE creation for the cluster [%s]: [%v]", vcdCluster.Name, err)
	}
	if resp.StatusCode != http.StatusAccepted {
		return "", fmt.Errorf("error occurred during RDE creation for the cluster [%s]", vcdCluster.Name)
	}
	taskURL := resp.Header.Get(VCDLocationHeader)
	task := govcd.NewTask(&workloadVCDClient.VCDClient.Client)
	task.Task.HREF = taskURL
	err = task.Refresh()
	if err != nil {
		return "", fmt.Errorf("error occurred during RDE creation for the cluster [%s]; error refreshing task: [%s]", vcdCluster.Name, task.Task.HREF)
	}
	rdeID := task.Task.Owner.ID
	log.Info("Created defined entity for cluster", "InfraId", rdeID)
	return rdeID, nil
}

func (r *VCDClusterReconciler) reconcileRDE(ctx context.Context, cluster *clusterv1.Cluster,
	vcdCluster *infrav1.VCDCluster, workloadVCDClient *vcdsdk.Client, vappID string, updateExternalID bool) error {
	log := ctrl.LoggerFrom(ctx)

	org, err := workloadVCDClient.VCDClient.GetOrgByName(vcdCluster.Spec.Org)
	if err != nil {
		return fmt.Errorf("failed to get org by name [%s]", vcdCluster.Spec.Org)
	}
	if org == nil || org.Org == nil {
		return fmt.Errorf("found nil org when getting org by name [%s]", vcdCluster.Spec.Org)
	}
	capvcdRdeManager := capisdk.NewCapvcdRdeManager(workloadVCDClient, vcdCluster.Status.InfraId)
	_, capvcdSpec, capvcdMetadata, capvcdStatus, err := capvcdRdeManager.GetCAPVCDEntity(ctx, vcdCluster.Status.InfraId)
	if err != nil {
		return fmt.Errorf("failed to get RDE with ID [%s] for cluster [%s]: [%v]", vcdCluster.Status.InfraId, vcdCluster.Name, err)
	}

	// TODO(VCDA-3107): Should we be updating org and vdc information here.
	metadataPatch := make(map[string]interface{})
	if org.Org.Name != capvcdMetadata.Org {
		metadataPatch["Org"] = org.Org.Name
	}

	vdc := vcdCluster.Spec.Ovdc
	if vdc != capvcdMetadata.Vdc {
		metadataPatch["Vdc"] = capvcdMetadata.Vdc
	}

	if capvcdMetadata.Site != vcdCluster.Spec.Site {
		metadataPatch["Site"] = vcdCluster.Spec.Site
	}

	specPatch := make(map[string]interface{})
	kcpList, err := getAllKubeadmControlPlaneForCluster(ctx, r.Client, *cluster)
	if err != nil {
		return fmt.Errorf("error getting all KubeadmControlPlane objects for cluster [%s]: [%v]", vcdCluster.Name, err)
	}

	mdList, err := getAllMachineDeploymentsForCluster(ctx, r.Client, *cluster)
	if err != nil {
		return fmt.Errorf("error getting all MachineDeployment objects for cluster [%s]: [%v]", vcdCluster.Name, err)
	}

	kubernetesSpecVersion := ""
	var kcpObj *kcpv1.KubeadmControlPlane
	// we assume that there is only one kcp object for a cluster.
	// TODO: need to update the logic for multiple kcp objects in the cluster
	if len(kcpList.Items) > 0 {
		kcpObj = &kcpList.Items[0]
		kubernetesSpecVersion = kcpObj.Spec.Version // for RDE updates, consider only the first kcp object
	}
	tkgVersion := getTKGVersion(cluster)
	crsBindingList, err := getAllCRSBindingForCluster(ctx, r.Client, *cluster)
	if err != nil {
		// this is fundamentally not a mandatory field
		log.Error(err, "error getting ClusterResourceBindings for cluster")
	}

	// UI can create CAPVCD clusters in future which can populate capiYaml in RDE.Spec, so we only want to populate if capiYaml is empty
	if capvcdSpec.CapiYaml == "" {
		capiYaml, err := getCapiYaml(ctx, r.Client, *cluster, *vcdCluster)
		if err != nil {
			log.Error(err,
				"error during RDE reconciliation: failed to construct capi yaml from kubernetes resources of cluster")
		} else {
			specPatch["CapiYaml"] = capiYaml
		}
	}

	// Updating status portion of the RDE in the following code
	capvcdStatusPatch := make(map[string]interface{})
	if capvcdStatus.Phase != cluster.Status.Phase {
		capvcdStatusPatch["Phase"] = cluster.Status.Phase
	}

	upgradeObject := capvcdStatus.Upgrade
	var ready bool
	ready, err = hasClusterReconciledToDesiredK8Version(ctx, r.Client, vcdCluster.Name, kcpList, mdList, kubernetesSpecVersion)
	if err != nil {
		return fmt.Errorf("failed to determine the value for ready flag for upgrades for cluster [%s(%s)]: [%v]",
			vcdCluster.Name, vcdCluster.Status.InfraId, err)
	}

	if kcpObj != nil {
		if upgradeObject.Current == nil {
			upgradeObject = rdeType.Upgrade{
				Current: &rdeType.K8sInfo{
					K8sVersion: kubernetesSpecVersion,
					TkgVersion: tkgVersion,
				},
				Previous: nil,
				Ready:    ready,
			}
		} else {
			if kcpObj.Spec.Version != capvcdStatus.Upgrade.Current.K8sVersion {

				upgradeObject.Previous = upgradeObject.Current
				upgradeObject.Current = &rdeType.K8sInfo{
					K8sVersion: kubernetesSpecVersion,
					TkgVersion: tkgVersion,
				}
			}
			upgradeObject.Ready = ready
		}
	}

	log.V(4).Info("upgrade section of the RDE", "previous", capvcdStatus.Upgrade, "current", upgradeObject)

	if !reflect.DeepEqual(upgradeObject, capvcdStatus.Upgrade) {
		capvcdStatusPatch["Upgrade"] = upgradeObject
	}

	// TODO: Delete "kubernetes" string in RDE. Discuss with Sahithi
	if capvcdStatus.Kubernetes != kubernetesSpecVersion {
		capvcdStatusPatch["Kubernetes"] = kubernetesSpecVersion
	}

	if capvcdStatus.Uid != vcdCluster.Status.InfraId {
		capvcdStatusPatch["Uid"] = vcdCluster.Status.InfraId
	}
	if capvcdStatus.Phase != cluster.Status.Phase {
		capvcdStatusPatch["Phase"] = cluster.Status.Phase
	}
	if capvcdStatus.ParentUID != vcdCluster.Status.ParentUID {
		capvcdStatusPatch["ParentUID"] = vcdCluster.Status.ParentUID
	}
	if capvcdStatus.UseAsManagementCluster != vcdCluster.Status.UseAsManagementCluster {
		capvcdStatusPatch["UseAsManagementCluster"] = vcdCluster.Status.UseAsManagementCluster
	}
	// fill CAPIStatusYaml
	capiStatusYaml, err := getCapiStatusYaml(ctx, r.Client, *cluster, *vcdCluster)
	if err != nil {
		log.Error(err, "failed to populate capiStatusYaml in RDE", "rdeID", vcdCluster.Status.InfraId)
	}
	if capvcdStatus.CapiStatusYaml != capiStatusYaml {
		capvcdStatusPatch["CapiStatusYaml"] = capiStatusYaml
	}

	pods := rdeType.Pods{
		CidrBlocks: cluster.Spec.ClusterNetwork.Pods.CIDRBlocks,
	}
	if !reflect.DeepEqual(capvcdStatus.K8sNetwork.Pods, pods) {
		capvcdStatusPatch["K8sNetwork.Pods"] = pods
	}

	if crsBindingList != nil {
		clusterResourceSetBindings := make([]rdeType.ClusterResourceSetBinding, 0)
		for _, crsBinding := range crsBindingList.Items {
			for _, crsSpecBindings := range crsBinding.Spec.Bindings {
				for _, resource := range crsSpecBindings.Resources {
					clusterResourceSetBindings = append(clusterResourceSetBindings,
						rdeType.ClusterResourceSetBinding{
							ClusterResourceSetName: crsSpecBindings.ClusterResourceSetName,
							Kind:                   resource.Kind,
							Name:                   resource.Name,
							Applied:                resource.Applied,
							LastAppliedTime:        resource.LastAppliedTime.Time.String(),
						})
				}
			}
		}
		if clusterResourceSetBindings != nil {
			if !reflect.DeepEqual(capvcdStatus.ClusterResourceSetBindings, clusterResourceSetBindings) {
				capvcdStatusPatch["ClusterResourceSetBindings"] = clusterResourceSetBindings
			}
		}
	}

	services := rdeType.Services{
		CidrBlocks: cluster.Spec.ClusterNetwork.Services.CIDRBlocks,
	}
	if !reflect.DeepEqual(capvcdStatus.K8sNetwork.Services, services) {
		capvcdStatusPatch["K8sNetwork.Services"] = services
	}

	clusterApiStatusPhase := ClusterApiStatusPhaseNotReady
	if cluster.Status.ControlPlaneReady {
		clusterApiStatusPhase = ClusterApiStatusPhaseReady
	}
	controlPlanePort := vcdCluster.Spec.ControlPlaneEndpoint.Port
	if controlPlanePort == 0 {
		controlPlanePort = TcpPort
	}
	clusterApiStatus := rdeType.ClusterApiStatus{
		Phase: clusterApiStatusPhase,
		ApiEndpoints: []rdeType.ApiEndpoints{
			{
				Host: vcdCluster.Spec.ControlPlaneEndpoint.Host,
				Port: int32(controlPlanePort),
			},
		},
	}
	if !reflect.DeepEqual(clusterApiStatus, capvcdStatus.ClusterAPIStatus) {
		capvcdStatusPatch["ClusterAPIStatus"] = clusterApiStatus
	}

	// update node status. Needed to remove stray nodes which were already deleted
	nodePoolList, err := getNodePoolList(ctx, r.Client, *cluster)
	if err != nil {
		klog.Errorf("failed to get node pool list from cluster [%s]: [%v]", cluster.Name, err)
	}
	if !reflect.DeepEqual(nodePoolList, capvcdStatus.NodePool) {
		capvcdStatusPatch["NodePool"] = nodePoolList
	}

	ovdcList := []rdeType.Ovdc{
		rdeType.Ovdc{
			Name:        workloadVCDClient.VDC.Vdc.Name,
			ID:          workloadVCDClient.VDC.Vdc.Name,
			OvdcNetwork: vcdCluster.Spec.OvdcNetwork,
		},
	}
	orgList := []rdeType.Org{
		rdeType.Org{
			Name: org.Org.Name,
			ID:   org.Org.ID,
		},
	}
	vcdResources := rdeType.VCDProperties{
		Site: vcdCluster.Spec.Site,
		Org:  orgList,
		Ovdc: ovdcList,
	}
	if !reflect.DeepEqual(vcdResources, capvcdStatus.VcdProperties) {
		capvcdStatusPatch["VcdProperties"] = vcdResources
	}

	obj := client.ObjectKey{
		Namespace: cluster.Namespace,
		Name:      cluster.Name,
	}
	kubeConfigBytes, err := kcfg.FromSecret(ctx, r.Client, obj)
	if err != nil {
		log.Error(err, "failed to update RDE private section with kubeconfig")
	} else {
		if !reflect.DeepEqual(string(kubeConfigBytes), capvcdStatus.Private.KubeConfig) {
			capvcdStatusPatch["Private.KubeConfig"] = string(kubeConfigBytes)
		}
	}
	if release.CAPVCDVersion != capvcdStatus.CapvcdVersion {
		capvcdStatusPatch["CapvcdVersion"] = release.CAPVCDVersion
	}

	updatedRDE, err := capvcdRdeManager.PatchRDE(ctx, specPatch, metadataPatch, capvcdStatusPatch, vcdCluster.Status.InfraId, vappID, updateExternalID)
	if err != nil {
		return fmt.Errorf("failed to update defined entity with ID [%s] for cluster [%s]: [%v]", vcdCluster.Status.InfraId, vcdCluster.Name, err)
	}

	if updatedRDE.State != RDEStatusResolved {
		// try to resolve the defined entity
		entityState, resp, err := workloadVCDClient.APIClient.DefinedEntityApi.ResolveDefinedEntity(ctx, updatedRDE.Id, org.Org.ID)
		if err != nil {
			return fmt.Errorf("failed to resolve defined entity with ID [%s] for cluster [%s]", vcdCluster.Status.InfraId, vcdCluster.Name)
		}
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("error while resolving defined entity with ID [%s] for cluster [%s] with message: [%s]", vcdCluster.Status.InfraId, vcdCluster.Name, entityState.Message)
		}
		if entityState.State != RDEStatusResolved {
			return fmt.Errorf("defined entity resolution failed for RDE with ID [%s] for cluster [%s] with message: [%s]", vcdCluster.Status.InfraId, vcdCluster.Name, entityState.Message)
		}
		log.Info("Resolved defined entity of cluster", "InfraId", vcdCluster.Status.InfraId)
	}
	return nil

}

func (r *VCDClusterReconciler) reconcileNormal(ctx context.Context, cluster *clusterv1.Cluster,
	vcdCluster *infrav1.VCDCluster) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)

	// To avoid spamming RDEs with updates, only update the RDE with events when machine creation is ongoing
	skipRDEEventUpdates := clusterv1.ClusterPhase(cluster.Status.Phase) == clusterv1.ClusterPhaseProvisioned

	userCreds, err := getUserCredentialsForCluster(ctx, r.Client, vcdCluster.Spec.UserCredentialsContext)
	if err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "Error getting client credentials to reconcile Cluster [%s] infrastructure", vcdCluster.Name)
	}
	workloadVCDClient, err := vcdsdk.NewVCDClientFromSecrets(vcdCluster.Spec.Site, vcdCluster.Spec.Org,
		vcdCluster.Spec.Ovdc, vcdCluster.Spec.Org, userCreds.Username, userCreds.Password, userCreds.RefreshToken, true, true)
	if err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "Error creating VCD client to reconcile Cluster [%s] infrastructure", vcdCluster.Name)
	}
	// General note on RDE operations, always ensure CAPVCD cluster reconciliation progress
	//is not affected by any RDE operation failures.
	infraID := vcdCluster.Status.InfraId
	capvcdRdeManager := capisdk.NewCapvcdRdeManager(workloadVCDClient, infraID)

	gateway, err := vcdsdk.NewGatewayManager(ctx, workloadVCDClient, vcdCluster.Spec.OvdcNetwork, vcdCluster.Spec.LoadBalancerConfigSpec.VipSubnet)
	if err != nil {
		updatedErr := capvcdRdeManager.AddToErrorSet(ctx, capisdk.VCDClusterError, "", vcdCluster.Name, fmt.Sprintf("failed to create gateway manager: [%v]", err))
		if updatedErr != nil {
			log.Error(updatedErr, "failed to add VCDClusterError into RDE", "rdeID", vcdCluster.Status.InfraId)
		}
		return ctrl.Result{}, errors.Wrapf(err, "failed to create gateway manager using the workload client to reconcile cluster [%s]", vcdCluster.Name)
	}
	err = capvcdRdeManager.RdeManager.RemoveErrorByNameOrIdFromErrorSet(ctx, vcdsdk.ComponentCAPVCD, capisdk.VCDClusterError, "", vcdCluster.Name)
	if err != nil {
		log.Error(err, "failed to remove VCDClusterError from RDE", "rdeID", vcdCluster.Status.InfraId)
	}
	specInfraID := vcdCluster.Spec.RDEId

	if infraID == "" {
		infraID = specInfraID
	}

	// creating RDEs for the clusters -
	// 1. clusters already created will have NO_RDE_ prefix in the infra ID. We should make sure that the cluster can co-exist
	// 2. Clusters which are newly created should check for CAPVCD_SKIP_RDE environment variable to determine if an RDE should be created for the cluster

	// NOTE: If CAPVCD_SKIP_RDE is not set, CAPVCD will error out if there is any error in RDE creation
	rdeVersionInUse := vcdCluster.Status.RdeVersionInUse
	if infraID == "" {
		// Create RDE for the cluster or generate a NO_RDE infra ID for the cluster.
		if !SkipRDE {
			// Create an RDE for the cluster. If RDE creation results in a failure, error out cluster creation.
			// check rights for RDE creation and create an RDE
			if !capvcdRdeManager.IsCapvcdEntityTypeRegistered(rdeType.CapvcdRDETypeVersion) {
				updatedErr := capvcdRdeManager.AddToErrorSet(ctx, capisdk.RdeError, "", vcdCluster.Name, fmt.Sprintf("CapvcdCluster entity type not registered or capvcdCluster rights missing from the user's role: [%v]", err))
				if updatedErr != nil {
					log.Error(updatedErr, "failed to add RdeError into RDE", "rdeID", vcdCluster.Status.InfraId)
				}
				return ctrl.Result{}, errors.Wrapf(errors.New("capvcdCluster entity type not registered or capvcdCluster rights missing from the user's role"),
					"cluster create issued with executeWithoutRDE=[%v] but unable to create capvcdCluster entity at version [%s]",
					SkipRDE, rdeType.CapvcdRDETypeVersion)
			}
			// create RDE
			nameFilter := &swagger.DefinedEntityApiGetDefinedEntitiesByEntityTypeOpts{
				Filter: optional.NewString(fmt.Sprintf("name==%s", vcdCluster.Name)),
			}
			org, err := workloadVCDClient.VCDClient.GetOrgByName(workloadVCDClient.ClusterOrgName)
			if err != nil {
				return ctrl.Result{}, errors.Wrapf(errors.New("failed to get org by name"), "error getting org by name for org [%s]: [%v]", workloadVCDClient.ClusterOrgName, err)
			}
			if org == nil || org.Org == nil {
				return ctrl.Result{}, errors.Wrapf(errors.New("invalid org ref obtained"),
					"obtained nil org when getting org by name [%s]", workloadVCDClient.ClusterOrgName)
			}
			// the following api call will return an empty list if there are no entities with the same name as the cluster
			definedEntities, resp, err := workloadVCDClient.APIClient.DefinedEntityApi.GetDefinedEntitiesByEntityType(ctx,
				capisdk.CAPVCDTypeVendor, capisdk.CAPVCDTypeNss, capisdk.CAPVCDEntityTypeDefaultMajorVersion, org.Org.ID, 1, 25, nameFilter)
			if err != nil {
				updatedErr := capvcdRdeManager.AddToErrorSet(ctx, capisdk.RdeError, "", vcdCluster.Name, fmt.Sprintf("Error fetching RDE: [%v]", err))
				if updatedErr != nil {
					log.Error(updatedErr, "failed to add RdeError into RDE", "rdeID", vcdCluster.Status.InfraId)
				}
				log.Error(err, "Error while checking if RDE is already present for the cluster",
					"entityTypeId", CAPVCDEntityTypeID)
				return ctrl.Result{}, errors.Wrapf(err, "Error while checking if RDE is already present for the cluster with entity type ID [%s]",
					CAPVCDEntityTypeID)
			}
			if resp == nil {
				msg := fmt.Sprintf("Error while checking if RDE for the cluster [%s] is already present for the cluster; "+
					"obtained an empty response for get defined entity call for the cluster", vcdCluster.Name)
				log.Error(nil, msg)
				updatedErr := capvcdRdeManager.AddToErrorSet(ctx, capisdk.RdeError, "", vcdCluster.Name, msg)
				if updatedErr != nil {
					log.Error(updatedErr, "failed to add RdeError into RDE", "rdeID", vcdCluster.Status.InfraId)
				}
				return ctrl.Result{}, errors.Wrapf(fmt.Errorf(msg), msg)

			} else if resp.StatusCode != http.StatusOK {
				msg := fmt.Sprintf("Invalid status code [%d] while checking if RDE is already present for the cluster using the entityTYpeID [%s]",
					resp.StatusCode, CAPVCDEntityTypeID)
				log.Error(nil, msg)
				updatedErr := capvcdRdeManager.AddToErrorSet(ctx, capisdk.RdeError, "", vcdCluster.Name, msg)
				if updatedErr != nil {
					log.Error(updatedErr, "failed to add RdeError into RDE", "rdeID", vcdCluster.Status.InfraId)
				}
				return ctrl.Result{}, errors.Wrapf(err, msg)
			}
			err = capvcdRdeManager.RdeManager.RemoveErrorByNameOrIdFromErrorSet(ctx, vcdsdk.ComponentCAPVCD, capisdk.RdeError, "", "")
			if err != nil {
				log.Error(err, "failed to remove RdeError from RDE", "rdeID", infraID)
			}
			// create an RDE with the same name as the vcdCluster object if there are no RDEs with the same name as the vcdCluster object
			if err == nil && resp != nil && resp.StatusCode == http.StatusOK {
				if len(definedEntities.Values) == 0 {
					rdeID, err := r.constructAndCreateRDEFromCluster(ctx, workloadVCDClient, cluster, vcdCluster)
					if err != nil {
						log.Error(err, "Error creating RDE for the cluster")
						return ctrl.Result{}, errors.Wrapf(err, "error creating RDE for the cluster %s", vcdCluster.Name)
					}
					infraID = rdeID
				} else {
					log.Info("RDE for the cluster is already present; skipping RDE creation", "InfraId",
						definedEntities.Values[0].Id)
					infraID = definedEntities.Values[0].Id
				}
			}
			rdeVersionInUse = rdeType.CapvcdRDETypeVersion
		} else {
			// If there is no RDE ID specified (or) created for any reason, self-generate one and use.
			// We need UUIDs to single-instance cleanly in the Virtual Services etc.
			noRDEID := NoRdePrefix + uuid.New().String()
			log.Info("Error retrieving InfraId. Hence using a self-generated UUID", "UUID", noRDEID)
			infraID = noRDEID
			rdeVersionInUse = NoRdePrefix
		}

	} else {
		log.V(3).Info("Reusing already available InfraID", "infraID", infraID)
		if strings.HasPrefix(infraID, NoRdePrefix) {
			log.V(3).Info("Infra ID has NO_RDE_ prefix. Using RdeVersionInUse -", "RdeVersionInUse", NoRdePrefix)
			rdeVersionInUse = NoRdePrefix
		} else {
			_, rdeVersion, err := capvcdRdeManager.GetRDEVersion(ctx, infraID)
			if err != nil {
				return ctrl.Result{}, errors.Wrapf(err,
					"\"Unexpected error retrieving RDE [%s] for the cluster [%s]", infraID, vcdCluster.Name)
			}
			// update the RdeVersionInUse with the entity type version of the rde.
			rdeVersionInUse = rdeVersion

			// update the createdByVersion if not present already
			if err := capvcdRdeManager.CheckForEmptyRDEAndUpdateCreatedByVersions(ctx, infraID); err != nil {
				return ctrl.Result{}, errors.Wrapf(err, "Failed to update RDE [%s] with created by version", infraID)
			}
		}

		// upgrade RDE if necessary
		if !strings.Contains(infraID, NoRdePrefix) && vcdCluster.Status.RdeVersionInUse != "" &&
			vcdCluster.Status.RdeVersionInUse != rdeType.CapvcdRDETypeVersion {
			capvcdRdeManager := capisdk.NewCapvcdRdeManager(workloadVCDClient, infraID)
			log.Info("Upgrading RDE", "rdeID", infraID,
				"targetRDEVersion", rdeType.CapvcdRDETypeVersion)
			_, err = capvcdRdeManager.ConvertToLatestRDEVersionFormat(ctx, infraID)
			if err != nil {
				log.Error(err, "failed to upgrade RDE", "rdeID", infraID,
					"sourceVersion", vcdCluster.Status.RdeVersionInUse,
					"targetVersion", rdeType.CapvcdRDETypeVersion)
				err1 := capvcdRdeManager.AddToErrorSet(ctx, capisdk.RdeError, "", vcdCluster.Name, fmt.Sprintf("RDE upgrade failed: [%v]", err))
				if err1 != nil {
					log.Error(err1, "failed to add RdeError (RDE upgrade failed) ", "rdeID", infraID)
				}
				return ctrl.Result{}, errors.Wrapf(err, "failed to upgrade RDE [%s]", infraID)
			}
			// calling reconcileRDE here to avoid delay in updating the RDE contents
			if err = r.reconcileRDE(ctx, cluster, vcdCluster, workloadVCDClient, "", false); err != nil {
				// TODO: can we recover the RDE to a proper state if RDE fails to reconcile?
				log.Error(err, "failed to reconcile RDE after upgrading RDE", "rdeID", infraID)
				err1 := capvcdRdeManager.AddToErrorSet(ctx, capisdk.RdeError, "", vcdCluster.Name, fmt.Sprintf("failed to reconcile RDE after upgrading RDE: [%v]", err))
				if err1 != nil {
					log.Error(err1, "failed to add RdeError (RDE upgrade failed) ", "rdeID", infraID)
				}
				return ctrl.Result{}, errors.Wrapf(err, "failed to reconcile RDE after upgrading RDE [%s]", infraID)
			}
			err = capvcdRdeManager.AddToEventSet(ctx, capisdk.RdeUpgraded, infraID, "", "", skipRDEEventUpdates)
			if err != nil {
				log.Error(err, "failed to add RDE-upgrade event (RDE upgraded successfully) ", "rdeID", infraID)
			}
			err = capvcdRdeManager.RdeManager.RemoveErrorByNameOrIdFromErrorSet(ctx, vcdsdk.ComponentCAPVCD, capisdk.RdeError, "", "")
			if err != nil {
				log.Error(err, "failed to remove RdeError (RDE upgraded successfully) ", "rdeID", infraID)
			}
			rdeVersionInUse = rdeType.CapvcdRDETypeVersion
		}
	}

	err = capvcdRdeManager.AddToEventSet(ctx, capisdk.RdeAvailable, infraID, "", "", skipRDEEventUpdates)
	if err != nil {
		log.Error(err, "failed to add RdeAvailable event", "rdeID", infraID)
	}

	if err := validateDerivedRDEProperties(vcdCluster, infraID, rdeVersionInUse); err != nil {
		log.Error(err, "Error validating derived infraID and RDE version")
		return ctrl.Result{}, errors.Wrapf(err, "error validating derived infraID and RDE version with VCDCluster status")
	}

	if vcdCluster.Status.InfraId == "" || vcdCluster.Status.RdeVersionInUse != rdeVersionInUse {
		// update the status
		log.Info("updating vcdCluster with the following data", "vcdCluster.Status.InfraId", infraID, "vcdCluster.Status.RdeVersionInUse", rdeVersionInUse)

		oldVCDCluster := vcdCluster.DeepCopy()
		vcdCluster.Status.InfraId = infraID
		vcdCluster.Status.RdeVersionInUse = rdeVersionInUse
		if err := r.Status().Patch(ctx, vcdCluster, client.MergeFrom(oldVCDCluster)); err != nil {
			updatedErr := capvcdRdeManager.AddToErrorSet(ctx, capisdk.CAPVCDObjectPatchError, "", vcdCluster.Name, fmt.Sprintf("failed to patch vcdcluster: [%v]", err))
			if updatedErr != nil {
				log.Error(updatedErr, "failed to add CAPVCDObjectPatchError into RDE", "rdeID", vcdCluster.Status.InfraId)
			}
			return ctrl.Result{}, errors.Wrapf(err,
				"unable to patch status of vcdCluster [%s] with InfraID [%s], RDEVersion [%s]",
				vcdCluster.Name, infraID, rdeType.CapvcdRDETypeVersion)
		}
		err = capvcdRdeManager.RdeManager.RemoveErrorByNameOrIdFromErrorSet(ctx, vcdsdk.ComponentCAPVCD, capisdk.CAPVCDObjectPatchError, "", vcdCluster.Name)
		if err != nil {
			log.Error(err, "failed to remove CAPVCDObjectPatchError from RDE", "rdeID", vcdCluster.Status.InfraId)
		}
	}
	// spec.RDEId should be populated because clusterctl move operation erases the status section of the VCDCluster object.
	// This can cause issues for a cluster which has no RDE because the auto-generated infraID will be lost.
	vcdCluster.Spec.RDEId = infraID

	rdeManager := vcdsdk.NewRDEManager(workloadVCDClient, vcdCluster.Status.InfraId,
		capisdk.StatusComponentNameCAPVCD, release.CAPVCDVersion)
	// After InfraId has been set, we can update site, org, ovdcNetwork, parentUid, useAsManagementCluster
	// proxyConfigSpec loadBalancerConfigSpec for vcdCluster status
	vcdCluster.Status.Site = vcdCluster.Spec.Site
	vcdCluster.Status.Org = vcdCluster.Spec.Org
	vcdCluster.Status.Ovdc = vcdCluster.Spec.Ovdc
	vcdCluster.Status.OvdcNetwork = vcdCluster.Spec.OvdcNetwork
	vcdCluster.Status.UseAsManagementCluster = vcdCluster.Spec.UseAsManagementCluster
	vcdCluster.Status.ParentUID = vcdCluster.Spec.ParentUID
	vcdCluster.Status.ProxyConfig = vcdCluster.Spec.ProxyConfigSpec
	vcdCluster.Status.LoadBalancerConfig = vcdCluster.Spec.LoadBalancerConfigSpec

	// create load balancer for the cluster. Only one-arm load balancer is fully tested.
	virtualServiceNamePrefix := capisdk.GetVirtualServiceNamePrefix(vcdCluster.Name, vcdCluster.Status.InfraId)
	lbPoolNamePrefix := capisdk.GetLoadBalancerPoolNamePrefix(vcdCluster.Name, vcdCluster.Status.InfraId)

	var oneArm *vcdsdk.OneArm = nil
	if vcdCluster.Spec.LoadBalancerConfigSpec.UseOneArm {
		oneArm = &OneArmDefault
	}
	var resourcesAllocated *vcdsdkutil.AllocatedResourcesMap
	controlPlaneNodeIP, resourcesAllocated, err := gateway.GetLoadBalancer(ctx,
		fmt.Sprintf("%s-tcp", virtualServiceNamePrefix), fmt.Sprintf("%s-tcp", lbPoolNamePrefix), oneArm)

	// TODO: ideally we should get this port from the GetLoadBalancer function
	controlPlanePort := TcpPort

	//TODO: Sahithi: Check if error is really because of missing virtual service.
	// In any other error cases, force create the new load balancer with the original control plane endpoint
	// (if already present). Do not overwrite the existing control plane endpoint with a new endpoint.
	var virtualServiceHref string
	if err != nil || controlPlaneNodeIP == "" {
		if vsError, ok := err.(*vcdsdk.VirtualServicePendingError); ok {
			log.Info("Error getting load balancer. Virtual Service is still pending",
				"virtualServiceName", vsError.VirtualServiceName, "error", err)
			return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
		}

		if vcdCluster.Spec.ControlPlaneEndpoint.Host != "" {
			controlPlanePort := vcdCluster.Spec.ControlPlaneEndpoint.Port
			log.Info("Creating load balancer for the cluster at user-specified endpoint",
				"host", vcdCluster.Spec.ControlPlaneEndpoint.Host, "port", controlPlanePort)
		} else {
			log.Info("Creating load balancer for the cluster")
		}

		resourcesAllocated = &vcdsdkutil.AllocatedResourcesMap{}
		// here we set enableVirtualServiceSharedIP to ensure that we don't use a DNAT rule. The variable is possibly
		// badly named. Though the user-facing name is good, the internal variable name could be better.
		controlPlaneNodeIP, err = gateway.CreateLoadBalancer(ctx, virtualServiceNamePrefix, lbPoolNamePrefix,
			[]string{}, []vcdsdk.PortDetails{
				{
					Protocol:     "TCP",
					PortSuffix:   "tcp",
					ExternalPort: int32(controlPlanePort),
					InternalPort: int32(controlPlanePort),
				},
			}, oneArm, !vcdCluster.Spec.LoadBalancerConfigSpec.UseOneArm,
			nil, vcdCluster.Spec.ControlPlaneEndpoint.Host, resourcesAllocated)
		if err != nil {
			updatedErr := capvcdRdeManager.AddToErrorSet(ctx, capisdk.LoadBalancerError, "", "",
				fmt.Sprintf("failed to create load balancer for the cluster [%s(%s)]: [%v]",
					vcdCluster.Name, vcdCluster.Status.InfraId, err))
			if updatedErr != nil {
				log.Error(updatedErr, "failed to add LoadBalancerError into RDE", "rdeID", vcdCluster.Status.InfraId)
			}
			return ctrl.Result{}, errors.Wrapf(err, "failed to create load balancer for the cluster [%s(%s)]: [%v]",
				vcdCluster.Name, vcdCluster.Status.InfraId, err)
		}

		// Update VCDResourceSet even if the creation has failed since we may have partially
		// created set of resources
		if err = addLBResourcesToVCDResourceSet(ctx, rdeManager, resourcesAllocated, controlPlaneNodeIP); err != nil {
			updatedErr := capvcdRdeManager.AddToErrorSet(ctx, capisdk.RdeError, "", vcdCluster.Name,
				fmt.Sprintf("failed to add VCD Resource [%s] of type [%s] from VCDResourceSet of RDE [%s]: [%v]",
					vcdCluster.Name, VcdResourceTypeVM, vcdCluster.Status.InfraId, err))
			if updatedErr != nil {
				log.Error(updatedErr, "failed to add RdeError into RDE", "rdeID", vcdCluster.Status.InfraId)
			}
			return ctrl.Result{}, errors.Wrapf(err, "failed to add load balancer resources to RDE [%s]", infraID)
		}
		if err = capvcdRdeManager.RdeManager.RemoveErrorByNameOrIdFromErrorSet(ctx, vcdsdk.ComponentCAPVCD, capisdk.RdeError, "", ""); err != nil {
			log.Error(err, "failed to remove RdeError ", "rdeID", infraID)
		}

		if len(resourcesAllocated.Get(vcdsdk.VcdResourceVirtualService)) > 0 {
			virtualServiceHref = resourcesAllocated.Get(vcdsdk.VcdResourceVirtualService)[0].Id
		}

		if err != nil {
			if vsError, ok := err.(*vcdsdk.VirtualServicePendingError); ok {
				log.Info("Error creating load balancer for cluster. Virtual Service is still pending",
					"virtualServiceName", vsError.VirtualServiceName, "error", err)
				err1 := capvcdRdeManager.AddToErrorSet(ctx, capisdk.LoadBalancerPending, virtualServiceHref, "", fmt.Sprintf("Error creating load balancer: [%v]", err))
				if err1 != nil {
					log.Error(err1, "failed to add LoadBalancerPending into RDE", "rdeID", infraID)
				}
				return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
			}
			if err = capvcdRdeManager.RdeManager.RemoveErrorByNameOrIdFromErrorSet(ctx, vcdsdk.ComponentCAPVCD, capisdk.LoadBalancerError, "", ""); err != nil {
				log.Error(err, "failed to remove LoadBalancerError ", "rdeID", infraID)
			}
			return ctrl.Result{}, errors.Wrapf(err,
				"Error creating create load balancer [%s] for the cluster [%s]: [%v]",
				virtualServiceNamePrefix, vcdCluster.Name, err)
		}
		log.Info("Resources Allocated in creation of load balancer",
			"resourcesAllocated", resourcesAllocated)
	}

	if err = addLBResourcesToVCDResourceSet(ctx, rdeManager, resourcesAllocated, controlPlaneNodeIP); err != nil {
		updatedErr := capvcdRdeManager.AddToErrorSet(ctx, capisdk.RdeError, "", vcdCluster.Name,
			fmt.Sprintf("failed to add VCD Resource [%s] of type [%s] from VCDResourceSet of RDE [%s]: [%v]",
				vcdCluster.Name, VcdResourceTypeVM, vcdCluster.Status.InfraId, err))
		if updatedErr != nil {
			log.Error(updatedErr, "failed to add RdeError (LBResources) into RDE", "rdeID", vcdCluster.Status.InfraId)
		}
		return ctrl.Result{}, errors.Wrapf(err, "failed to add load balancer resources to RDE [%s]", infraID)
	}
	if err = capvcdRdeManager.RdeManager.RemoveErrorByNameOrIdFromErrorSet(ctx, vcdsdk.ComponentCAPVCD, capisdk.RdeError, "", ""); err != nil {
		log.Error(err, "failed to remove RdeError from RDE", "rdeID", infraID)
	}

	if len(resourcesAllocated.Get(vcdsdk.VcdResourceVirtualService)) > 0 {
		virtualServiceHref = resourcesAllocated.Get(vcdsdk.VcdResourceVirtualService)[0].Id
	}

	vcdCluster.Spec.ControlPlaneEndpoint = infrav1.APIEndpoint{
		Host: controlPlaneNodeIP,
		Port: controlPlanePort,
	}
	log.Info(fmt.Sprintf("Control plane endpoint for the cluster is [%s]", controlPlaneNodeIP))

	err = capvcdRdeManager.AddToEventSet(ctx, capisdk.LoadBalancerAvailable, virtualServiceHref, "", "", skipRDEEventUpdates)
	if err != nil {
		log.Error(err, "failed to add LoadBalancerAvailable event into RDE", "rdeID", infraID)
	}
	err = capvcdRdeManager.RdeManager.RemoveErrorByNameOrIdFromErrorSet(ctx, vcdsdk.ComponentCAPVCD, capisdk.LoadBalancerPending, "", "")
	if err != nil {
		log.Error(err, "failed to remove LoadBalancerPending error (RDE upgraded successfully) ", "rdeID", infraID)
	}

	if !strings.HasPrefix(vcdCluster.Status.InfraId, NoRdePrefix) {
		org, err := workloadVCDClient.VCDClient.GetOrgByName(workloadVCDClient.ClusterOrgName)
		if err != nil {
			return ctrl.Result{}, errors.Wrapf(errors.New("failed to get org by name"), "error getting org by name for org [%s]: [%v]", workloadVCDClient.ClusterOrgName, err)
		}
		if org == nil || org.Org == nil {
			return ctrl.Result{}, errors.Wrapf(errors.New("invalid org ref obtained"),
				"obtained nil org when getting org by name [%s]", workloadVCDClient.ClusterOrgName)
		}
		_, resp, _, err := workloadVCDClient.APIClient.DefinedEntityApi.GetDefinedEntity(ctx, vcdCluster.Status.InfraId, org.Org.ID)
		if err == nil && resp != nil && resp.StatusCode == http.StatusOK {
			if err = r.reconcileRDE(ctx, cluster, vcdCluster, workloadVCDClient, "", false); err != nil {
				log.Error(err, "Error occurred during RDE reconciliation",
					"InfraId", vcdCluster.Status.InfraId)
			}
		} else {
			log.Error(err, "Unexpected error retrieving RDE for the cluster from VCD",
				"InfraId", vcdCluster.Status.InfraId)
			// Some additional checks to log non-sensitive content safely.
			if resp == nil {
				log.Error(nil, "Error retrieving RDE for the cluster from VCD; obtained an empty response",
					"InfraId", vcdCluster.Status.InfraId)
			} else if resp.StatusCode != http.StatusOK {
				log.Error(nil, "Error retrieving RDE for the cluster from VCD",
					"InfraId", vcdCluster.Status.InfraId)
			}
		}
	}

	// create VApp
	vdcManager, err := vcdsdk.NewVDCManager(workloadVCDClient, workloadVCDClient.ClusterOrgName,
		workloadVCDClient.ClusterOVDCName)
	if err != nil {
		updatedErr := capvcdRdeManager.AddToErrorSet(ctx, capisdk.VCDClusterError, "", vcdCluster.Name, fmt.Sprintf("failed to get vdcManager: [%v]", err))
		if updatedErr != nil {
			log.Error(updatedErr, "failed to add VCDClusterError into RDE", "rdeID", vcdCluster.Status.InfraId)
		}
		return ctrl.Result{}, errors.Wrapf(err,
			"Error creating vdc manager to to reconcile vcd infrastructure for cluster [%s]", vcdCluster.Name)
	}
	metadataMap := map[string]string{
		CapvcdInfraId: vcdCluster.Status.InfraId,
	}
	if vdcManager.Vdc == nil {
		updatedErr := capvcdRdeManager.AddToErrorSet(ctx, capisdk.VCDClusterError, "", vcdCluster.Name, fmt.Sprintf("%v", err))
		if updatedErr != nil {
			log.Error(updatedErr, "failed to add VCDClusterError into RDE", "rdeID", vcdCluster.Status.InfraId)
		}
		return ctrl.Result{}, errors.Errorf("no Vdc created with vdc manager name [%s]", vdcManager.Client.ClusterOVDCName)
	}
	if err = capvcdRdeManager.RdeManager.RemoveErrorByNameOrIdFromErrorSet(ctx, vcdsdk.ComponentCAPVCD, capisdk.VCDClusterError, "", ""); err != nil {
		log.Error(err, "failed to remove VCDClusterError from RDE", "rdeID", infraID)
	}
	_, err = vdcManager.Vdc.GetVAppByName(vcdCluster.Name, true)
	if err != nil && err == govcd.ErrorEntityNotFound {
		vcdCluster.Status.VAppMetadataUpdated = false
	}

	clusterVApp, err := vdcManager.GetOrCreateVApp(vcdCluster.Name, vcdCluster.Spec.OvdcNetwork)
	if err != nil {
		err1 := capvcdRdeManager.AddToErrorSet(ctx, capisdk.VCDClusterVappCreationError, "", vcdCluster.Name, fmt.Sprintf("%v", err))
		if err1 != nil {
			log.Error(err1, "failed to add VCDClusterVappCreationError into RDE", "rdeID", infraID)
		}
		return ctrl.Result{}, errors.Wrapf(err, "Error creating Infra vApp for the cluster [%s]: [%v]", vcdCluster.Name, err)
	}
	if clusterVApp == nil || clusterVApp.VApp == nil {
		err1 := capvcdRdeManager.AddToErrorSet(ctx, capisdk.VCDClusterVappCreationError, "", vcdCluster.Name, fmt.Sprintf("%v", err))
		if err1 != nil {
			log.Error(err1, "failed to add VCDClusterVappCreationError into RDE", "rdeID", infraID)
		}
		return ctrl.Result{}, errors.Wrapf(err, "found nil value for VApp [%s]", vcdCluster.Name)
	}
	if !strings.HasPrefix(vcdCluster.Status.InfraId, NoRdePrefix) {
		if err := r.reconcileRDE(ctx, cluster, vcdCluster, workloadVCDClient, clusterVApp.VApp.ID, true); err != nil {
			log.Error(err, "failed to add VApp ID to RDE", "rdeID", infraID, "vappID", clusterVApp.VApp.ID)
			return ctrl.Result{}, errors.Wrapf(err, "failed to update RDE [%s] with VApp ID [%s]: [%v]", vcdCluster.Status.InfraId, clusterVApp.VApp.ID, err)
		}
		log.Info("successfully updated external ID of RDE with VApp ID", "infraID", infraID, "vAppID", clusterVApp.VApp.ID)
	}

	if metadataMap != nil && len(metadataMap) > 0 && !vcdCluster.Status.VAppMetadataUpdated {
		if err := vdcManager.AddMetadataToVApp(vcdCluster.Name, metadataMap); err != nil {
			err1 := capvcdRdeManager.AddToErrorSet(ctx, capisdk.VCDClusterError, "", vcdCluster.Name, fmt.Sprintf("failed to add metadata into vApp [%s]: [%v]", vcdCluster.Name, err))
			if err1 != nil {
				log.Error(err1, "failed to add VCDClusterError into RDE", "rdeID", infraID)
			}
			return ctrl.Result{}, fmt.Errorf("unable to add metadata [%s] to vApp [%s]: [%v]", metadataMap, vcdCluster.Name, err)
		}
		vcdCluster.Status.VAppMetadataUpdated = true
	}
	// Add VApp to VCDResourceSet
	err = rdeManager.AddToVCDResourceSet(ctx, vcdsdk.ComponentCAPVCD, VCDResourceVApp,
		vcdCluster.Name, clusterVApp.VApp.ID, nil)
	if err != nil {
		updatedErr := capvcdRdeManager.AddToErrorSet(ctx, capisdk.RdeError, "", vcdCluster.Name,
			fmt.Sprintf("failed to add VCD Resource [%s] of type [%s] from VCDResourceSet of RDE [%s]: [%v]",
				vcdCluster.Name, VcdResourceTypeVM, vcdCluster.Status.InfraId, err))
		if updatedErr != nil {
			log.Error(updatedErr, "failed to add RdeError into RDE", "rdeID", vcdCluster.Status.InfraId)
		}
		return ctrl.Result{}, errors.Wrapf(err, "failed to add resource [%s] of type [%s] to VCDResourceSet of RDE [%s]: [%v]",
			vcdCluster.Name, VCDResourceVApp, infraID, err)
	}
	err = capvcdRdeManager.AddToEventSet(ctx, capisdk.InfraVappAvailable, clusterVApp.VApp.ID, "", "", skipRDEEventUpdates)
	if err != nil {
		log.Error(err, "failed to add InfraVappAvailable event into RDE", "rdeID", infraID)
	}
	err = capvcdRdeManager.RdeManager.RemoveErrorByNameOrIdFromErrorSet(ctx, vcdsdk.ComponentCAPVCD, capisdk.VCDClusterVappCreationError, "", "")
	if err != nil {
		log.Error(err, "failed to remove VCDClusterVappCreationError from RDE", "rdeID", infraID)
	}

	// Update the vcdCluster resource with updated information
	// TODO Check if updating ovdcNetwork, Org and Vdc should be done somewhere earlier in the code.
	vcdCluster.Status.Ready = true
	conditions.MarkTrue(vcdCluster, LoadBalancerAvailableCondition)
	if cluster.Status.ControlPlaneReady {
		err = capvcdRdeManager.AddToEventSet(ctx, capisdk.ControlplaneReady, infraID, "", "", skipRDEEventUpdates)
		if err != nil {
			log.Error(err, "failed to add ControlPlaneReady event into RDE", "rdeID", infraID)
		}
	}

	return ctrl.Result{}, nil
}

func (r *VCDClusterReconciler) reconcileDelete(ctx context.Context,
	vcdCluster *infrav1.VCDCluster) (ctrl.Result, error) {

	log := ctrl.LoggerFrom(ctx)
	patchHelper, err := patch.NewHelper(vcdCluster, r.Client)
	if err != nil {
		return ctrl.Result{}, err
	}
	conditions.MarkFalse(vcdCluster, LoadBalancerAvailableCondition, clusterv1.DeletingReason,
		clusterv1.ConditionSeverityInfo, "")

	// restore vcdCluster status
	vcdCluster.Status.VAppMetadataUpdated = true
	if err := patchVCDCluster(ctx, patchHelper, vcdCluster); err != nil {
		return ctrl.Result{}, errors.Wrap(err, "Error occurred during cluster deletion; failed to patch VCDCluster")
	}

	userCreds, err := getUserCredentialsForCluster(ctx, r.Client, vcdCluster.Spec.UserCredentialsContext)
	if err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "Error getting client credentials to reconcile Cluster [%s] infrastructure", vcdCluster.Name)
	}
	workloadVCDClient, err := vcdsdk.NewVCDClientFromSecrets(vcdCluster.Spec.Site, vcdCluster.Spec.Org,
		vcdCluster.Spec.Ovdc, vcdCluster.Spec.Org, userCreds.Username, userCreds.Password, userCreds.RefreshToken, true, true)
	if err != nil {
		return ctrl.Result{}, errors.Wrapf(err,
			"Error occurred during cluster deletion; unable to create client for the workload cluster [%s]",
			vcdCluster.Name)
	}
	capvcdRdeManager := capisdk.NewCapvcdRdeManager(workloadVCDClient, vcdCluster.Status.InfraId)

	gateway, err := vcdsdk.NewGatewayManager(ctx, workloadVCDClient, vcdCluster.Spec.OvdcNetwork, vcdCluster.Spec.LoadBalancerConfigSpec.VipSubnet)
	if err != nil {
		updatedErr := capvcdRdeManager.AddToErrorSet(ctx, capisdk.VCDClusterError, "", vcdCluster.Name, fmt.Sprintf("failed to create new gateway manager: [%v]", err))
		if updatedErr != nil {
			log.Error(updatedErr, "failed to add VCDClusterError into RDE", "rdeID", vcdCluster.Status.InfraId)
		}
		return ctrl.Result{}, errors.Wrapf(err, "failed to create gateway manager using the workload client to reconcile cluster [%s]", vcdCluster.Name)
	}
	if err = capvcdRdeManager.RdeManager.RemoveErrorByNameOrIdFromErrorSet(ctx, vcdsdk.ComponentCAPVCD, capisdk.VCDClusterError, "", ""); err != nil {
		log.Error(err, "failed to remove VCDClusterError from RDE")
	}

	// Delete the load balancer components
	virtualServiceNamePrefix := capisdk.GetVirtualServiceNamePrefix(vcdCluster.Name, vcdCluster.Status.InfraId)
	lbPoolNamePrefix := capisdk.GetVirtualServiceNamePrefix(vcdCluster.Name, vcdCluster.Status.InfraId)

	controlPlanePort := vcdCluster.Spec.ControlPlaneEndpoint.Port
	if controlPlanePort == 0 {
		controlPlanePort = TcpPort
	}
	var oneArm *vcdsdk.OneArm = nil
	if vcdCluster.Spec.LoadBalancerConfigSpec.UseOneArm {
		oneArm = &OneArmDefault
	}
	resourcesAllocated := &vcdsdkutil.AllocatedResourcesMap{}
	_, err = gateway.DeleteLoadBalancer(ctx, virtualServiceNamePrefix, lbPoolNamePrefix,
		[]vcdsdk.PortDetails{
			{
				Protocol:     "TCP",
				PortSuffix:   "tcp",
				ExternalPort: int32(controlPlanePort),
				InternalPort: int32(controlPlanePort),
			},
		}, oneArm, resourcesAllocated)
	if err != nil {
		err1 := capvcdRdeManager.AddToErrorSet(ctx, capisdk.LoadBalancerError, "", virtualServiceNamePrefix, fmt.Sprintf("%v", err))
		if err1 != nil {
			log.Error(err1, "failed to add LoadBalancerError into RDE", "rdeID", vcdCluster.Status.InfraId)
		}
		return ctrl.Result{}, errors.Wrapf(err,
			"Error occurred during cluster [%s] deletion; unable to delete the load balancer [%s]: [%v]",
			vcdCluster.Name, virtualServiceNamePrefix, err)
	}
	log.Info("Deleted the load balancer components (virtual service, lb pool, dnat rule) of the cluster",
		"virtual service", virtualServiceNamePrefix, "lb pool", lbPoolNamePrefix)
	err = capvcdRdeManager.AddToEventSet(ctx, capisdk.LoadbalancerDeleted, virtualServiceNamePrefix, "", "", true)
	if err != nil {
		log.Error(err, "failed to add LoadBalancerDeleted event into RDE", "rdeID", vcdCluster.Status.InfraId)
	}
	err = capvcdRdeManager.RdeManager.RemoveErrorByNameOrIdFromErrorSet(ctx, vcdsdk.ComponentCAPVCD, capisdk.LoadBalancerError, "", "")
	if err != nil {
		log.Error(err, "failed to remove LoadBalancerError from RDE", "rdeID", vcdCluster.Status.InfraId)
	}

	vdcManager, err := vcdsdk.NewVDCManager(workloadVCDClient, workloadVCDClient.ClusterOrgName,
		workloadVCDClient.ClusterOVDCName)
	if err != nil {
		updatedErr := capvcdRdeManager.AddToErrorSet(ctx, capisdk.VCDClusterError, "", vcdCluster.Name, fmt.Sprintf("failed to get vdcManager: [%v]", err))
		if updatedErr != nil {
			log.Error(updatedErr, "failed to add VCDClusterError into RDE", "rdeID", vcdCluster.Status.InfraId)
		}
		return ctrl.Result{}, errors.Wrapf(err, "Error creating vdc manager to to reconcile vcd infrastructure for cluster [%s]", vcdCluster.Name)
	}
	if err = capvcdRdeManager.RdeManager.RemoveErrorByNameOrIdFromErrorSet(ctx, vcdsdk.ComponentCAPVCD, capisdk.VCDClusterError, "", vcdCluster.Name); err != nil {
		log.Error(err, "failed to remove VCDClusterError from RDE", "rdeID", vcdCluster.Status.InfraId)
	}
	// Delete vApp
	vApp, err := workloadVCDClient.VDC.GetVAppByName(vcdCluster.Name, true)
	if err != nil {
		log.Error(err, fmt.Sprintf("Error occurred during cluster deletion; vApp [%s] not found", vcdCluster.Name))
	}
	if vApp != nil {
		//Delete the vApp if and only if rdeId (matches) present in the vApp
		if !vcdCluster.Status.VAppMetadataUpdated {
			err1 := capvcdRdeManager.AddToErrorSet(ctx, capisdk.VCDClusterError, "", vcdCluster.Name, fmt.Sprintf("rdeId is not presented in vApp metadata"))
			if err1 != nil {
				log.Error(err1, "failed to add VCDClusterError into RDE", "rdeID", vcdCluster.Status.InfraId)
			}
			return ctrl.Result{}, errors.Errorf("Error occurred during cluster deletion; Field [VAppMetadataUpdated] is %t", vcdCluster.Status.VAppMetadataUpdated)
		}
		metadataInfraId, err := vdcManager.GetMetadataByKey(vApp, CapvcdInfraId)
		if err != nil {
			err1 := capvcdRdeManager.AddToErrorSet(ctx, capisdk.VCDClusterError, "", vcdCluster.Name, fmt.Sprintf("%v", err))
			if err1 != nil {
				log.Error(err1, "failed to add VCDClusterError into RDE", "rdeID", vcdCluster.Status.InfraId)
			}
			return ctrl.Result{}, errors.Errorf("Error occurred during fetching metadata in vApp")
		}
		// checking the metadata value and vcdCluster.Status.InfraId are equal or not
		if metadataInfraId != vcdCluster.Status.InfraId {
			err1 := capvcdRdeManager.AddToErrorSet(ctx, capisdk.VCDClusterError, "", vcdCluster.Name, fmt.Sprintf("%v", err))
			if err1 != nil {
				log.Error(err1, "failed to add VCDClusterError into RDE", "rdeID", vcdCluster.Status.InfraId)
			}
			return ctrl.Result{},
				errors.Errorf("error occurred during cluster deletion; failed to delete vApp [%s]",
					vcdCluster.Name)
		}
		err = capvcdRdeManager.RdeManager.RemoveErrorByNameOrIdFromErrorSet(ctx, vcdsdk.ComponentCAPVCD, capisdk.VCDClusterError, "", "")
		if err != nil {
			log.Error(err, "failed to remove VCDClusterError from RDE", "rdeID", vcdCluster.Status.InfraId)
		}
		if vApp.VApp.Children != nil {
			updatedErr := capvcdRdeManager.AddToErrorSet(ctx, capisdk.VCDClusterVappDeleteError, "", vcdCluster.Name, fmt.Sprintf(
				"Error occurred during cluster deletion; %d VMs detected in the vApp %s",
				len(vApp.VApp.Children.VM), vcdCluster.Name))
			if updatedErr != nil {
				log.Error(updatedErr, "failed to add VCDClusterVappDeleteError into RDE", "rdeID", vcdCluster.Status.InfraId)
			}
			return ctrl.Result{}, errors.Errorf(
				"Error occurred during cluster deletion; %d VMs detected in the vApp %s",
				len(vApp.VApp.Children.VM), vcdCluster.Name)
		} else {
			log.Info("Deleting vApp of the cluster", "vAppName", vcdCluster.Name)
			err = vdcManager.DeleteVApp(vcdCluster.Name)
			if err != nil {
				err1 := capvcdRdeManager.AddToErrorSet(ctx, capisdk.VCDClusterVappDeleteError, "", vcdCluster.Name, fmt.Sprintf("%v", err))
				if err1 != nil {
					log.Error(err1, "failed to add VCDClusterVappDeleteError into RDE", "rdeID", vcdCluster.Status.InfraId)
				}
				return ctrl.Result{}, errors.Wrapf(err,
					"Error occurred during cluster deletion; failed to delete vApp [%s]", vcdCluster.Name)
			}
			log.Info("Successfully deleted vApp of the cluster", "vAppName", vcdCluster.Name)
		}
	}
	// Remove vapp from VCDResourceSet in the RDE
	rdeManager := vcdsdk.NewRDEManager(workloadVCDClient, vcdCluster.Status.InfraId,
		capisdk.StatusComponentNameCAPVCD, release.CAPVCDVersion)
	err = rdeManager.RemoveFromVCDResourceSet(ctx, vcdsdk.ComponentCAPVCD, VCDResourceVApp, vcdCluster.Name)
	if err != nil {
		updatedErr := capvcdRdeManager.AddToErrorSet(ctx, capisdk.RdeError, "", vcdCluster.Name,
			fmt.Sprintf("failed to delete VCD Resource [%s] of type [%s] from VCDResourceSet of RDE [%s]: [%v]",
				vcdCluster.Name, VCDResourceVApp, vcdCluster.Status.InfraId, err))
		if updatedErr != nil {
			log.Error(updatedErr, "failed to add RdeError into RDE", "rdeID", vcdCluster.Status.InfraId)
		}
		return ctrl.Result{}, errors.Wrapf(err,
			"failed to delete VCD Resource [%s] of type [%s] from VCDResourceSet of RDE [%s]: [%v]",
			vcdCluster.Name, VCDResourceVApp, vcdCluster.Status.InfraId, err)
	}
	if err = capvcdRdeManager.RdeManager.RemoveErrorByNameOrIdFromErrorSet(ctx, vcdsdk.ComponentCAPVCD, capisdk.RdeError, "", vcdCluster.Name); err != nil {
		log.Error(err, "failed to remove RdeError from RDE", "rdeID", vcdCluster.Status.InfraId)
	}
	err = capvcdRdeManager.AddToEventSet(ctx, capisdk.VappDeleted, "", "", "", true)
	if err != nil {
		log.Error(err, "failed to add vAppDeleted event into RDE", "rdeID", vcdCluster.Status.InfraId)
	}
	err = capvcdRdeManager.RdeManager.RemoveErrorByNameOrIdFromErrorSet(ctx, vcdsdk.ComponentCAPVCD, capisdk.VCDClusterVappDeleteError, "", "")
	if err != nil {
		log.Error(err, "failed to remove vAppDeleteError from RDE", "rdeID", vcdCluster.Status.InfraId)
	}

	// TODO: If RDE deletion fails, should we throw an error during reconciliation?
	// Delete RDE
	if vcdCluster.Status.InfraId != "" && !strings.HasPrefix(vcdCluster.Status.InfraId, NoRdePrefix) {
		org, err := workloadVCDClient.VCDClient.GetOrgByName(workloadVCDClient.ClusterOrgName)
		if err != nil {
			return ctrl.Result{}, errors.Wrapf(errors.New("failed to get org by name"), "error getting org by name for org [%s]: [%v]", workloadVCDClient.ClusterOrgName, err)
		}
		if org == nil || org.Org == nil {
			return ctrl.Result{}, errors.Wrapf(errors.New("invalid org ref obtained"),
				"obtained nil org when getting org by name [%s]", workloadVCDClient.ClusterOrgName)
		}
		definedEntities, resp, err := workloadVCDClient.APIClient.DefinedEntityApi.GetDefinedEntitiesByEntityType(ctx,
			capisdk.CAPVCDTypeVendor, capisdk.CAPVCDTypeNss, rdeType.CapvcdRDETypeVersion, org.Org.ID, 1, 25,
			&swagger.DefinedEntityApiGetDefinedEntitiesByEntityTypeOpts{
				Filter: optional.NewString(fmt.Sprintf("id==%s", vcdCluster.Status.InfraId)),
			})
		if err != nil {
			err1 := capvcdRdeManager.AddToErrorSet(ctx, capisdk.RdeError, "", "", fmt.Sprintf("failed to get RDE [%s]: %v", vcdCluster.Status.InfraId, err))
			if err1 != nil {
				log.Error(err1, "failed to add RdeError into RDE", "rdeID", vcdCluster.Status.InfraId)
			}
			return ctrl.Result{}, errors.Wrapf(err, "Error occurred during RDE deletion; failed to fetch defined entities by entity type [%s] and ID [%s] for cluster [%s]", CAPVCDEntityTypeID, vcdCluster.Status.InfraId, vcdCluster.Name)
		}
		if resp.StatusCode != http.StatusOK {
			err1 := capvcdRdeManager.AddToErrorSet(ctx, capisdk.RdeError, "", "", fmt.Sprintf("Got wrong status code while fetching RDE [%s]: %v", vcdCluster.Status.InfraId, err))
			if err1 != nil {
				log.Error(err1, "failed to add RdeError into RDE", "rdeID", vcdCluster.Status.InfraId)
			}
			return ctrl.Result{}, errors.Errorf("Error occurred during RDE deletion; error while fetching defined entities by entity type [%s] and ID [%s] for cluster [%s]", CAPVCDEntityTypeID, vcdCluster.Status.InfraId, vcdCluster.Name)
		}
		if len(definedEntities.Values) > 0 {
			// resolve defined entity before deleting
			entityState, resp, err := workloadVCDClient.APIClient.DefinedEntityApi.ResolveDefinedEntity(ctx,
				vcdCluster.Status.InfraId, org.Org.ID)
			if err != nil {
				err1 := capvcdRdeManager.AddToErrorSet(ctx, capisdk.RdeError, "", "", fmt.Sprintf("failed to resolve entity: [%v]", err))
				if err1 != nil {
					log.Error(err1, "failed to add RdeError into RDE", "rdeID", vcdCluster.Status.InfraId)
				}
				return ctrl.Result{}, errors.Wrapf(err, "Error occurred during RDE deletion; error occurred while resolving defined entity [%s] with ID [%s] before deleting", vcdCluster.Name, vcdCluster.Status.InfraId)
			}
			if resp.StatusCode != http.StatusOK {
				log.Error(nil, "Error occurred during RDE deletion; failed to resolve RDE with ID [%s] for cluster [%s]: [%s]", vcdCluster.Status.InfraId, vcdCluster.Name, entityState.Message)
			}
			resp, err = workloadVCDClient.APIClient.DefinedEntityApi.DeleteDefinedEntity(ctx,
				vcdCluster.Status.InfraId, org.Org.ID, nil)
			if err != nil {
				err1 := capvcdRdeManager.AddToErrorSet(ctx, capisdk.RdeError, "", "", fmt.Sprintf("%v", err))
				if err1 != nil {
					log.Error(err1, "failed to add RdeError into RDE", "rdeID", vcdCluster.Status.InfraId)
				}
				return ctrl.Result{}, errors.Wrapf(err, "error occurred during RDE deletion; failed to execute delete defined entity call for RDE with ID [%s]", vcdCluster.Status.InfraId)
			}
			if resp.StatusCode != http.StatusNoContent {
				err1 := capvcdRdeManager.AddToErrorSet(ctx, capisdk.RdeError, "", "", fmt.Sprintf("%v", err))
				if err1 != nil {
					log.Error(err1, "failed to add RdeError into RDE", "rdeID", vcdCluster.Status.InfraId)
				}
				return ctrl.Result{}, errors.Errorf("Error occurred during RDE deletion; error deleting defined entity associated with the cluster. RDE id: [%s]", vcdCluster.Status.InfraId)
			}
			log.Info("Successfully deleted the (RDE) defined entity of the cluster")
		} else {
			log.Info("Attempted deleting the RDE, but corresponding defined entity is not found", "RDEId", vcdCluster.Status.InfraId)
		}
	}
	log.Info("Successfully deleted all the infra resources of the cluster")
	// Cluster is deleted so remove the finalizer.
	controllerutil.RemoveFinalizer(vcdCluster, infrav1.ClusterFinalizer)

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *VCDClusterReconciler) SetupWithManager(mgr ctrl.Manager, options controller.Options) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&infrav1.VCDCluster{}).
		WithOptions(options).
		Complete(r)
}
