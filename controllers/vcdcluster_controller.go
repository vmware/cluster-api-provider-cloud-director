/*
   Copyright 2021 VMware, Inc.
   SPDX-License-Identifier: Apache-2.0
*/

package controllers

import (
	"context"
	_ "embed"
	"fmt"
	"github.com/google/uuid"
	rdeType "github.com/vmware/cluster-api-provider-cloud-director/pkg/vcdtypes/rde_type_1_1_0"
	"net/http"
	"os"
	"reflect"
	"strings"
	"time"

	"github.com/antihax/optional"
	"github.com/blang/semver"
	"github.com/pkg/errors"
	vcdsdkutil "github.com/vmware/cloud-provider-for-cloud-director/pkg/util"
	"github.com/vmware/cloud-provider-for-cloud-director/pkg/vcdsdk"
	swagger "github.com/vmware/cloud-provider-for-cloud-director/pkg/vcdswaggerclient_37_2"
	infrav1beta3 "github.com/vmware/cluster-api-provider-cloud-director/api/v1beta3"
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

	RDEStatusResolved             = "RESOLVED"
	VCDLocationHeader             = "Location"
	ResourceTypeOvdc              = "ovdc"
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
	vcdCluster := &infrav1beta3.VCDCluster{}
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

	if !controllerutil.ContainsFinalizer(vcdCluster, infrav1beta3.ClusterFinalizer) {
		controllerutil.AddFinalizer(vcdCluster, infrav1beta3.ClusterFinalizer)
		return ctrl.Result{}, nil
	}

	if clusterBeingDeleted {
		return r.reconcileDelete(ctx, vcdCluster)
	}

	return r.reconcileNormal(ctx, cluster, vcdCluster)
}

func patchVCDCluster(ctx context.Context, patchHelper *patch.Helper, vcdCluster *infrav1beta3.VCDCluster) error {
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
// Derived values for infraID and rdeVersionInUse are essentially the final computed values - i.e. either created or picked from the VCDCluster object.
// validateDerivedRDEProperties makes sure the infra ID and the RDE version in-use doesn't change to unexpected values over different reconciliations.
func validateDerivedRDEProperties(vcdCluster *infrav1beta3.VCDCluster, infraID string, rdeVersionInUse string) error {
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
			return fmt.Errorf("invalid RDE version [%s] derived", rdeVersionInUse)
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

// updateClientWithVDC is to add the latest VDC into vcdClient.
// Reminder: Although vcdcluster provides array for vcdResourceMap[ovdc], vcdcluster should use only one OVDC in CAPVCD 1.1
func updateClientWithVDC(vcdCluster *infrav1beta3.VCDCluster, client *vcdsdk.Client, ovdcName string) error {
	log := ctrl.LoggerFrom(context.Background())
	orgName := vcdCluster.Spec.Org
	if vcdCluster.Status.VcdResourceMap.Ovdcs != nil && len(vcdCluster.Status.VcdResourceMap.Ovdcs) > 0 {
		NameChanged, newOvdc, err := checkIfOvdcNameChange(vcdCluster, client)
		if err != nil {
			return fmt.Errorf("error occurred while updating the client with VDC: [%v]", err)
		}
		if newOvdc == nil {
			return fmt.Errorf("error occurred while updating the client with VDC as the new ovdc is empty")
		}
		if NameChanged {
			ovdcName = newOvdc.Vdc.Name
			vcdCluster.Spec.Ovdc = newOvdc.Vdc.Name
			log.Info("updating vcdCluster with the following data", "vcdCluster.Status.VcdResourceMap[ovdc].ID", client.VDC.Vdc.ID, "vcdCluster.Status.VcdResourceMap[ovdc].Name", client.VDC.Vdc.Name)
		}
	}
	newOvdc, err := getOvdcByName(client, orgName, ovdcName)
	if err != nil {
		return fmt.Errorf("failed to get the ovdc by the name [%s]: [%v]", ovdcName, err)
	}
	client.VDC = newOvdc
	client.ClusterOVDCName = newOvdc.Vdc.Name

	return nil
}

func createVCDClientFromSecrets(ctx context.Context, client client.Client,
	vcdCluster *infrav1beta3.VCDCluster, ovdcName string) (*vcdsdk.Client, error) {
	userCreds, err := getUserCredentialsForCluster(ctx, client, vcdCluster.Spec.UserCredentialsContext)
	if err != nil {
		return nil, fmt.Errorf("error getting client credentials to reconcile Cluster [%s] infrastructure: [%v]",
			vcdCluster.Name, err)
	}
	vcdClient, err := vcdsdk.NewVCDClientFromSecrets(
		vcdCluster.Spec.Site,
		vcdCluster.Spec.Org,
		ovdcName,
		vcdCluster.Spec.Org,
		userCreds.Username,
		userCreds.Password,
		userCreds.RefreshToken,
		true,
		false)
	if err != nil {
		return nil, fmt.Errorf("error creating VCD client from secrets to reconcile Cluster [%s] infrastructure: [%v]", vcdCluster.Name, err)
	}

	if ovdcName != "" {
		err = updateClientWithVDC(vcdCluster, vcdClient, ovdcName)
		if err != nil {
			return nil, fmt.Errorf(
				"error updating VCD client with VDC [%s] to reconcile Cluster [%s] infrastructure: [%v]",
				ovdcName, vcdCluster.Name, err)
		}
	}

	return vcdClient, nil
}

// TODO: Remove uncommented code when decision to only keep capi.yaml as part of RDE spec is finalized
func (r *VCDClusterReconciler) constructCapvcdRDE(ctx context.Context, cluster *clusterv1.Cluster,
	vcdCluster *infrav1beta3.VCDCluster, vdc *types.Vdc, vcdOrg *types.Org) (*swagger.DefinedEntity, error) {

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
		{
			Name: vcdOrg.Name,
			ID:   vcdOrg.ID,
		},
	}
	ovdcList := []rdeType.Ovdc{
		{
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
				CapvcdVersion:          release.Version,
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
				CreatedByVersion:           release.Version,
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

func (r *VCDClusterReconciler) constructAndCreateRDEFromCluster(ctx context.Context, vcdClient *vcdsdk.Client,
	cluster *clusterv1.Cluster, vcdCluster *infrav1beta3.VCDCluster) (string, error) {

	log := ctrl.LoggerFrom(ctx)

	if vcdClient == nil {
		return "", fmt.Errorf("vcdClient is nil")
	}

	if vcdClient.VDC == nil || vcdClient.VDC.Vdc == nil {
		return "", fmt.Errorf("VDC client in vcdClient object is nil")
	}

	org, err := getOrgByName(vcdClient, vcdCluster.Spec.Org)
	if err != nil {
		return "", fmt.Errorf("error occurred while constructing RDE from cluster [%s]", vcdCluster.Status.InfraId)
	}
	if org == nil || org.Org == nil {
		return "", fmt.Errorf("unable to get the org by name [%s]", vcdCluster.Spec.Org)
	}

	rde, err := r.constructCapvcdRDE(ctx, cluster, vcdCluster, vcdClient.VDC.Vdc, org.Org)
	if err != nil {
		return "", fmt.Errorf("error occurred while constructing RDE payload for the cluster [%s]: [%v]", vcdCluster.Name, err)
	}
	resp, err := vcdClient.APIClient.DefinedEntityApi.CreateDefinedEntity(ctx, *rde,
		rde.EntityType, org.Org.ID, nil)
	if err != nil {
		return "", fmt.Errorf("error occurred during RDE creation for the cluster [%s]: [%v]", vcdCluster.Name, err)
	}
	if resp.StatusCode != http.StatusAccepted {
		return "", fmt.Errorf("error occurred during RDE creation for the cluster [%s]", vcdCluster.Name)
	}
	taskURL := resp.Header.Get(VCDLocationHeader)
	task := govcd.NewTask(&vcdClient.VCDClient.Client)
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
	vcdCluster *infrav1beta3.VCDCluster, vcdClient *vcdsdk.Client, vappID string, updateExternalID bool) error {
	log := ctrl.LoggerFrom(ctx)

	if vcdClient == nil {
		return fmt.Errorf("vcdClient is nil")
	}

	// skip RDE reconciliation if the Infra ID has NoRdePrefix
	if strings.HasPrefix(vcdCluster.Status.InfraId, NoRdePrefix) {
		// skip rde reconciliation
		log.Info("Skipping RDE reconciliation as cluster has no RDE",
			"InfraID", vcdCluster.Status.InfraId,
			"NoRDEPrefix", NoRdePrefix)
		return nil
	}

	org, err := vcdClient.VCDClient.GetOrgByName(vcdCluster.Spec.Org)
	if err != nil {
		return fmt.Errorf("failed to get org by name [%s]", vcdCluster.Spec.Org)
	}
	if org == nil || org.Org == nil {
		return fmt.Errorf("found nil org when getting org by name [%s]", vcdCluster.Spec.Org)
	}
	capvcdRdeManager := capisdk.NewCapvcdRdeManager(vcdClient, vcdCluster.Status.InfraId)
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

	vcdResources := rdeType.VCDProperties{
		Site: vcdCluster.Spec.Site,
		Org: []rdeType.Org{
			{
				Name: org.Org.Name,
				ID:   org.Org.ID,
			},
		},
		Ovdc: []rdeType.Ovdc{
			{
				Name:        vcdClient.VDC.Vdc.Name,
				ID:          vcdClient.VDC.Vdc.Name,
				OvdcNetwork: vcdCluster.Spec.OvdcNetwork,
			},
		},
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
	if release.Version != capvcdStatus.CapvcdVersion {
		capvcdStatusPatch["CapvcdVersion"] = release.Version
	}

	updatedRDE, err := capvcdRdeManager.PatchRDE(ctx, specPatch, metadataPatch, capvcdStatusPatch, vcdCluster.Status.InfraId, vappID, updateExternalID)
	if err != nil {
		return fmt.Errorf("failed to update defined entity with ID [%s] for cluster [%s]: [%v]", vcdCluster.Status.InfraId, vcdCluster.Name, err)
	}

	if updatedRDE.State != swagger.RDEStateResolved {
		// try to resolve the defined entity
		entityState, resp, err := vcdClient.APIClient.DefinedEntityApi.ResolveDefinedEntity(ctx, updatedRDE.Id, org.Org.ID)
		if err != nil {
			return fmt.Errorf("failed to resolve defined entity with ID [%s] for cluster [%s]", vcdCluster.Status.InfraId, vcdCluster.Name)
		}
		if resp != nil && resp.StatusCode != http.StatusOK {
			return fmt.Errorf("error while resolving defined entity with ID [%s] for cluster [%s] with message: [%s]", vcdCluster.Status.InfraId, vcdCluster.Name, entityState.Message)
		}
		if entityState.State != RDEStatusResolved {
			return fmt.Errorf("defined entity resolution failed for RDE with ID [%s] for cluster [%s] with message: [%s]", vcdCluster.Status.InfraId, vcdCluster.Name, entityState.Message)
		}
		log.Info("Resolved defined entity of cluster", "InfraId", vcdCluster.Status.InfraId)
	}
	return nil

}

func (r *VCDClusterReconciler) reconcileInfraID(ctx context.Context, cluster *clusterv1.Cluster,
	vcdCluster *infrav1beta3.VCDCluster, vcdClient *vcdsdk.Client, skipRDEEventUpdates bool) error {

	log := ctrl.LoggerFrom(ctx)

	if vcdClient == nil {
		return fmt.Errorf("vcdClient is nil")
	}

	// General note on RDE operations, always ensure CAPVCD cluster reconciliation progress
	//is not affected by any RDE operation failures.
	infraID := vcdCluster.Status.InfraId
	capvcdRdeManager := capisdk.NewCapvcdRdeManager(vcdClient, infraID)

	specInfraID := vcdCluster.Spec.RDEId

	if infraID == "" {
		infraID = specInfraID
	}

	// creating RDEs for the clusters -
	// 1. clusters already created will have NO_RDE_ prefix in the infra ID. We should make sure that the cluster can co-exist
	// 2. Clusters which are newly created should check for CAPVCD_SKIP_RDE environment variable to determine if an RDE should be created for the cluster

	// NOTE: If CAPVCD_SKIP_RDE is not set, CAPVCD will error out if there is any error in RDE creation
	// rdeVersionInUseByCluster is the current version of the RDE associated with the cluster. Note that this version can be different from the latest RDE version used by the CAPVCD product.
	// In such cases, RDE version of the cluster will be upgraded to the latest RDE version used by CAPVCD.
	rdeVersionInUseByCluster := vcdCluster.Status.RdeVersionInUse
	if infraID == "" {
		// Create RDE for the cluster or generate a NO_RDE infra ID for the cluster.
		if !SkipRDE {
			// Create an RDE for the cluster. If RDE creation results in a failure, error out cluster creation.
			// check rights for RDE creation and create an RDE
			if !capvcdRdeManager.IsCapvcdEntityTypeRegistered(rdeType.CapvcdRDETypeVersion) {
				capvcdRdeManager.AddToErrorSet(ctx, capisdk.RdeError, "", vcdCluster.Name,
					"CapvcdCluster entity type not registered or capvcdCluster rights missing from the user's role")
				return fmt.Errorf(
					"capvcdCluster entity type not registered or capvcdCluster rights missing from the user's role"+
						"cluster create issued with executeWithoutRDE=[%v] but unable to create capvcdCluster entity at version [%s]",
					SkipRDE, rdeType.CapvcdRDETypeVersion)
			}
			// create RDE
			nameFilter := &swagger.DefinedEntityApiGetDefinedEntitiesByEntityTypeOpts{
				Filter: optional.NewString(fmt.Sprintf("name==%s", vcdCluster.Name)),
			}
			org, err := vcdClient.VCDClient.GetOrgByName(vcdClient.ClusterOrgName)
			if err != nil {
				return fmt.Errorf("error getting org by name for org [%s]: [%v]", vcdClient.ClusterOrgName, err)
			}
			if org == nil || org.Org == nil {
				return fmt.Errorf("obtained nil org when getting org by name [%s]", vcdClient.ClusterOrgName)
			}

			// the following api call will return an empty list if there are no entities with the same name as the cluster
			definedEntities, resp, err := vcdClient.APIClient.DefinedEntityApi.GetDefinedEntitiesByEntityType(ctx,
				capisdk.CAPVCDTypeVendor, capisdk.CAPVCDTypeNss, capisdk.CAPVCDEntityTypeDefaultMajorVersion, org.Org.ID, 1, 25, nameFilter)
			if err != nil {
				capvcdRdeManager.AddToErrorSet(ctx, capisdk.RdeError, "", vcdCluster.Name,
					fmt.Sprintf("Error fetching RDE: [%v]", err))
				log.Error(err, "Error while checking if RDE is already present for the cluster",
					"entityTypeId", CAPVCDEntityTypeID)
				return fmt.Errorf(
					"error checking if RDE is already present for the cluster [%s] with entity type ID [%s]: [%v]",
					vcdCluster.Name, CAPVCDEntityTypeID, err)
			}
			if resp == nil {
				msg := fmt.Sprintf("Error while checking if RDE for the cluster [%s] is already present for the cluster; "+
					"obtained an empty response for get defined entity call for the cluster", vcdCluster.Name)
				log.Error(nil, msg)
				capvcdRdeManager.AddToErrorSet(ctx, capisdk.RdeError, "", vcdCluster.Name, msg)
				return fmt.Errorf(msg)

			} else if resp.StatusCode != http.StatusOK {
				msg := fmt.Sprintf("Invalid status code [%d] while checking if RDE is already present for the cluster using the entityTYpeID [%s]",
					resp.StatusCode, CAPVCDEntityTypeID)
				log.Error(nil, msg)
				capvcdRdeManager.AddToErrorSet(ctx, capisdk.RdeError, "", vcdCluster.Name, msg)
				return fmt.Errorf(msg)
			}
			err = capvcdRdeManager.RdeManager.RemoveErrorByNameOrIdFromErrorSet(ctx, vcdsdk.ComponentCAPVCD, capisdk.RdeError, "", "")
			if err != nil {
				log.Error(err, "failed to remove RdeError from RDE", "rdeID", infraID)
			}
			// create an RDE with the same name as the vcdCluster object if there are no RDEs with the same name as the vcdCluster object
			if err == nil && resp != nil && resp.StatusCode == http.StatusOK {
				if len(definedEntities.Values) == 0 {
					rdeID, err := r.constructAndCreateRDEFromCluster(ctx, vcdClient, cluster, vcdCluster)
					if err != nil {
						log.Error(err, "Error creating RDE for the cluster")
						return fmt.Errorf("error creating RDE for the cluster [%s]: [%v]", vcdCluster.Name, err)
					}
					infraID = rdeID
					rdeVersionInUseByCluster = rdeType.CapvcdRDETypeVersion
				} else {
					log.Info("RDE for the cluster is already present; skipping RDE creation", "InfraId",
						definedEntities.Values[0].Id)
					infraID = definedEntities.Values[0].Id
					entityTypeArr := strings.Split(definedEntities.Values[0].EntityType, ":")
					rdeVersionInUseByCluster = entityTypeArr[len(entityTypeArr)-1]
				}
			}
		} else {
			// If there is no RDE ID specified (or) created for any reason, self-generate one and use.
			// We need UUIDs to single-instance cleanly in the Virtual Services etc.
			noRDEID := NoRdePrefix + uuid.New().String()
			log.Info("Error retrieving InfraId. Hence using a self-generated UUID", "UUID", noRDEID)
			infraID = noRDEID
			rdeVersionInUseByCluster = NoRdePrefix
		}

	} else {
		log.V(3).Info("Reusing already available InfraID", "infraID", infraID)
		if strings.HasPrefix(infraID, NoRdePrefix) {
			log.V(3).Info("Infra ID has NO_RDE_ prefix. Using RdeVersionInUse -", "RdeVersionInUse", NoRdePrefix)
			rdeVersionInUseByCluster = NoRdePrefix
		} else {
			_, rdeVersion, err := capvcdRdeManager.GetRDEVersion(ctx, infraID)
			if err != nil {
				return fmt.Errorf("unexpected error retrieving RDE [%s] for the cluster [%s]: [%v]",
					infraID, vcdCluster.Name, err)
			}
			// update the RdeVersionInUse with the entity type version of the rde.
			rdeVersionInUseByCluster = rdeVersion

			// update the createdByVersion if not present already
			if err := capvcdRdeManager.CheckForEmptyRDEAndUpdateCreatedByVersions(ctx, infraID); err != nil {
				return fmt.Errorf("failed to update RDE [%s] for cluster [%s] with created by version: [%v]",
					vcdCluster.Name, infraID, err)
			}
		}

		//1. If infraID indicates that there is no RDE (Runtime Defined Entity), skip the RDE upgrade process.
		//2. If vcdCluster.Status.RdeVersionInUse is empty, skip the RDE upgrade process.
		//3. If version outdated is detected, proceed with the RDE upgrade process.
		if !strings.Contains(infraID, NoRdePrefix) && vcdCluster.Status.RdeVersionInUse != "" &&
			vcdCluster.Status.RdeVersionInUse != rdeType.CapvcdRDETypeVersion {
			capvcdRdeManager := capisdk.NewCapvcdRdeManager(vcdClient, infraID)
			// 4. Skip the RDE upgrade process if the VCDKECluster flag is set to true
			//    and current_rde_version < RDE should be in use for a given CAPVCD version- rdeType.CapvcdRDETypeVersion always be the latest RDE version
			if !capvcdRdeManager.IsVCDKECluster(ctx, infraID) && capisdk.CheckIfClusterRdeNeedsUpgrade(rdeVersionInUseByCluster, rdeType.CapvcdRDETypeVersion) {
				log.Info("Upgrading RDE", "rdeID", infraID,
					"targetRDEVersion", rdeType.CapvcdRDETypeVersion)
				if _, err := capvcdRdeManager.ConvertToLatestRDEVersionFormat(ctx, infraID); err != nil {
					log.Error(err, "failed to upgrade RDE", "rdeID", infraID,
						"sourceVersion", vcdCluster.Status.RdeVersionInUse,
						"targetVersion", rdeType.CapvcdRDETypeVersion)
					capvcdRdeManager.AddToErrorSet(ctx, capisdk.RdeError, "", vcdCluster.Name, fmt.Sprintf("RDE upgrade failed: [%v]", err))
					return fmt.Errorf("failed to upgrade RDE [%s] for cluster [%s]: [%v]", vcdCluster.Name,
						infraID, err)
				}
				// calling reconcileRDE here to avoid delay in updating the RDE contents
				if err := r.reconcileRDE(ctx, cluster, vcdCluster, vcdClient, "", false); err != nil {
					// TODO: can we recover the RDE to a proper state if RDE fails to reconcile?
					log.Error(err, "failed to reconcile RDE after upgrading RDE", "rdeID", infraID)
					capvcdRdeManager.AddToErrorSet(ctx, capisdk.RdeError, "", vcdCluster.Name,
						fmt.Sprintf("failed to reconcile RDE after upgrading RDE: [%v]", err))
				}
				capvcdRdeManager.AddToEventSet(ctx, capisdk.RdeUpgraded, infraID, "", "",
					skipRDEEventUpdates)
				if err := capvcdRdeManager.RdeManager.RemoveErrorByNameOrIdFromErrorSet(ctx, vcdsdk.ComponentCAPVCD,
					capisdk.RdeError, "", ""); err != nil {
					log.Error(err, "failed to remove RdeError (RDE upgraded successfully) ", "rdeID", infraID)
				}
				rdeVersionInUseByCluster = rdeType.CapvcdRDETypeVersion
			}
		}
	}

	if err := validateDerivedRDEProperties(vcdCluster, infraID, rdeVersionInUseByCluster); err != nil {
		log.Error(err, "Error validating derived infraID and RDE version")
		return fmt.Errorf("error validating derived infraID and RDE version with VCDCluster status for [%s]: [%v]",
			vcdCluster.Name, err)
	}

	if vcdCluster.Status.InfraId == "" || vcdCluster.Status.RdeVersionInUse != rdeVersionInUseByCluster {
		// update the status
		log.Info("updating vcdCluster with the following data",
			"vcdCluster.Status.InfraId", infraID, "vcdCluster.Status.RdeVersionInUse", rdeVersionInUseByCluster)

		oldVCDCluster := vcdCluster.DeepCopy()
		vcdCluster.Status.InfraId = infraID
		vcdCluster.Status.RdeVersionInUse = rdeVersionInUseByCluster
		if err := r.Status().Patch(ctx, vcdCluster, client.MergeFrom(oldVCDCluster)); err != nil {
			capvcdRdeManager.AddToErrorSet(ctx, capisdk.CAPVCDObjectPatchError, "",
				vcdCluster.Name, fmt.Sprintf("failed to patch vcdcluster: [%v]", err))
			return fmt.Errorf("unable to patch status of vcdCluster [%s] with InfraID [%s], RDEVersion [%s]: [%v]",
				vcdCluster.Name, infraID, rdeType.CapvcdRDETypeVersion, err)
		}
		if err := capvcdRdeManager.RdeManager.RemoveErrorByNameOrIdFromErrorSet(ctx, vcdsdk.ComponentCAPVCD,
			capisdk.CAPVCDObjectPatchError, "", vcdCluster.Name); err != nil {
			log.Error(err, "failed to remove CAPVCDObjectPatchError from RDE", "rdeID",
				vcdCluster.Status.InfraId)
		}
	}
	// spec.RDEId should be populated because clusterctl move operation erases the status section of the VCDCluster object.
	// This can cause issues for a cluster which has no RDE because the auto-generated infraID will be lost.
	vcdCluster.Spec.RDEId = infraID

	return nil
}

func (r *VCDClusterReconciler) reconcileLoadBalancer(ctx context.Context, vcdCluster *infrav1beta3.VCDCluster,
	vcdClient *vcdsdk.Client, skipRDEEventUpdates bool) (ctrl.Result, error) {

	log := ctrl.LoggerFrom(ctx)
	virtualServiceNamePrefix := capisdk.GetVirtualServiceNamePrefix(vcdCluster.Name, vcdCluster.Status.InfraId)
	lbPoolNamePrefix := capisdk.GetLoadBalancerPoolNamePrefix(vcdCluster.Name, vcdCluster.Status.InfraId)

	if vcdClient == nil {
		return ctrl.Result{}, fmt.Errorf("vcdClient is nil")
	}

	var oneArm *vcdsdk.OneArm = nil
	if vcdCluster.Spec.LoadBalancerConfigSpec.UseOneArm {
		oneArm = &OneArmDefault
	}
	var resourcesAllocated *vcdsdkutil.AllocatedResourcesMap

	capvcdRdeManager := capisdk.NewCapvcdRdeManager(vcdClient, vcdCluster.Status.InfraId)
	gateway, err := vcdsdk.NewGatewayManager(ctx, vcdClient, vcdCluster.Spec.OvdcNetwork,
		vcdCluster.Spec.LoadBalancerConfigSpec.VipSubnet, vcdCluster.Spec.Ovdc)
	if err != nil {
		capvcdRdeManager.AddToErrorSet(ctx, capisdk.VCDClusterError, "", vcdCluster.Name,
			fmt.Sprintf("failed to create gateway manager: [%v]", err))
		return ctrl.Result{}, fmt.Errorf("failed to create gateway manager using the workload client to reconcile cluster [%s]: [%v]",
			vcdCluster.Name, err)
	}

	rdeManager := vcdsdk.NewRDEManager(vcdClient, vcdCluster.Status.InfraId,
		capisdk.StatusComponentNameCAPVCD, release.Version)

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
			capvcdRdeManager.AddToErrorSet(ctx, capisdk.LoadBalancerError, "", "",
				fmt.Sprintf("failed to create load balancer for the cluster [%s(%s)]: [%v]",
					vcdCluster.Name, vcdCluster.Status.InfraId, err))
			return ctrl.Result{}, fmt.Errorf("failed to create load balancer for the cluster [%s(%s)]: [%v]",
				vcdCluster.Name, vcdCluster.Status.InfraId, err)
		}

		// Update VCDResourceSet even if the creation has failed since we may have partially
		// created set of resources
		if err = addLBResourcesToVCDResourceSet(ctx, rdeManager, resourcesAllocated, controlPlaneNodeIP); err != nil {
			log.Error(err, "failed to add LoadBalancer resources to VCD resource set of RDE",
				"rdeID", vcdCluster.Status.InfraId)
			capvcdRdeManager.AddToErrorSet(ctx, capisdk.RdeError, "", vcdCluster.Name,
				fmt.Sprintf("failed to add VCD Resource [%s] of type [%s] from VCDResourceSet of RDE [%s]: [%v]",
					vcdCluster.Name, VcdResourceTypeVM, vcdCluster.Status.InfraId, err))
		}
		if err = capvcdRdeManager.RdeManager.RemoveErrorByNameOrIdFromErrorSet(ctx, vcdsdk.ComponentCAPVCD,
			capisdk.RdeError, "", ""); err != nil {
			log.Error(err, "failed to remove RdeError ", "rdeID", vcdCluster.Status.InfraId)
		}

		if len(resourcesAllocated.Get(vcdsdk.VcdResourceVirtualService)) > 0 {
			virtualServiceHref = resourcesAllocated.Get(vcdsdk.VcdResourceVirtualService)[0].Id
		}

		if err != nil {
			if vsError, ok := err.(*vcdsdk.VirtualServicePendingError); ok {
				log.Info("Error creating load balancer for cluster. Virtual Service is still pending",
					"virtualServiceName", vsError.VirtualServiceName, "error", err)
				capvcdRdeManager.AddToErrorSet(ctx, capisdk.LoadBalancerPending, virtualServiceHref,
					"", fmt.Sprintf("Error creating load balancer: [%v]", err))
				return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
			}
			if err = capvcdRdeManager.RdeManager.RemoveErrorByNameOrIdFromErrorSet(ctx, vcdsdk.ComponentCAPVCD,
				capisdk.LoadBalancerError, "", ""); err != nil {
				log.Error(err, "failed to remove LoadBalancerError ", "rdeID", vcdCluster.Status.InfraId)
			}
			return ctrl.Result{}, errors.Wrapf(err,
				"Error creating create load balancer [%s] for the cluster [%s]: [%v]",
				virtualServiceNamePrefix, vcdCluster.Name, err)
		}
		log.Info("Resources Allocated in creation of load balancer", "resourcesAllocated", resourcesAllocated)
	}

	if err = addLBResourcesToVCDResourceSet(ctx, rdeManager, resourcesAllocated, controlPlaneNodeIP); err != nil {
		log.Error(err, "failed to add LoadBalancer resources to VCD resource set of RDE",
			"rdeID", vcdCluster.Status.InfraId)
		capvcdRdeManager.AddToErrorSet(ctx, capisdk.RdeError, "", vcdCluster.Name,
			fmt.Sprintf("failed to add VCD Resource [%s] of type [%s] from VCDResourceSet of RDE [%s]: [%v]",
				vcdCluster.Name, VcdResourceTypeVM, vcdCluster.Status.InfraId, err))
	}
	if err = capvcdRdeManager.RdeManager.RemoveErrorByNameOrIdFromErrorSet(ctx, vcdsdk.ComponentCAPVCD, capisdk.RdeError,
		"", ""); err != nil {
		log.Error(err, "failed to remove RdeError from RDE", "rdeID", vcdCluster.Status.InfraId)
	}

	if len(resourcesAllocated.Get(vcdsdk.VcdResourceVirtualService)) > 0 {
		virtualServiceHref = resourcesAllocated.Get(vcdsdk.VcdResourceVirtualService)[0].Id
	}

	vcdCluster.Spec.ControlPlaneEndpoint = infrav1beta3.APIEndpoint{
		Host: controlPlaneNodeIP,
		Port: controlPlanePort,
	}
	log.Info(fmt.Sprintf("Control plane endpoint for the cluster is [%s]", controlPlaneNodeIP))

	capvcdRdeManager.AddToEventSet(ctx, capisdk.LoadBalancerAvailable, virtualServiceHref, "",
		"", skipRDEEventUpdates)
	if err != nil {
		log.Error(err, "failed to add LoadBalancerAvailable event into RDE", "rdeID", vcdCluster.Status.InfraId)
	}
	err = capvcdRdeManager.RdeManager.RemoveErrorByNameOrIdFromErrorSet(ctx, vcdsdk.ComponentCAPVCD,
		capisdk.LoadBalancerPending, "", "")
	if err != nil {
		log.Error(err, "failed to remove LoadBalancerPending error (RDE upgraded successfully) ",
			"rdeID", vcdCluster.Status.InfraId)
	}
	return ctrl.Result{}, nil
}

func (r *VCDClusterReconciler) reconcileNormal(ctx context.Context, cluster *clusterv1.Cluster,
	vcdCluster *infrav1beta3.VCDCluster) (ctrl.Result, error) {

	log := ctrl.LoggerFrom(ctx)

	// To avoid spamming RDEs with updates, only update the RDE with events when machine creation is ongoing
	skipRDEEventUpdates := clusterv1.ClusterPhase(cluster.Status.Phase) == clusterv1.ClusterPhaseProvisioned

	vcdClient, err := createVCDClientFromSecrets(ctx, r.Client, vcdCluster, "")

	// close all idle connections when reconciliation is done
	defer func() {
		if vcdClient != nil && vcdClient.VCDClient != nil {
			vcdClient.VCDClient.Client.Http.CloseIdleConnections()
			log.V(6).Info(fmt.Sprintf("closed connection to the http client [%#v]",
				vcdClient.VCDClient.Client.Http))
		}
	}()
	if err != nil {
		log.Error(err, "error occurred while logging in to VCD")
		return ctrl.Result{}, errors.Wrapf(err, "Error creating VCD client to reconcile Cluster [%s] infrastructure",
			vcdCluster.Name)
	}

	// updating the VCD cluster resource with any VDC name changes to is necessary in VCD cluster controller because
	// the OVDC name is used to get the OVDC network
	if vcdClient.VDC != nil && vcdClient.VDC.Vdc != nil {
		err = updateVdcResourceToVcdCluster(vcdCluster, ResourceTypeOvdc, vcdClient.VDC.Vdc.ID, vcdClient.VDC.Vdc.Name)
		if err != nil {
			return ctrl.Result{}, errors.Wrapf(err, "Error updating vcdResource into vcdcluster.status to reconcile Cluster [%s] infrastructure", vcdCluster.Name)
		}
	}

	if err := r.reconcileInfraID(ctx, cluster, vcdCluster, vcdClient, skipRDEEventUpdates); err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "Unable to reconcile Infra ID for cluster [%s]", vcdCluster.Name)
	}

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

	// create load balancer for the cluster
	if result, err := r.reconcileLoadBalancer(ctx, vcdCluster, vcdClient, skipRDEEventUpdates); err != nil {
		return result, errors.Wrapf(err, "Unable to reconcile Load Balancer for cluster [%s(%s)]",
			vcdCluster.Name, vcdCluster.Status.InfraId)
	} else if result.Requeue || result.RequeueAfter > 0 {
		log.Info("Re queuing the request",
			"result.Requeue", result.Requeue, "result.RequeueAfter", result.RequeueAfter.String())
		return result, nil
	}

	if err := r.reconcileRDE(ctx, cluster, vcdCluster, vcdClient, "", false); err != nil {
		log.Error(err, "Error occurred during RDE reconciliation", "InfraId", vcdCluster.Status.InfraId)
	}

	// Update the vcdCluster resource with updated information
	// TODO Check if updating ovdcNetwork, Org and Vdc should be done somewhere earlier in the code.
	vcdCluster.Status.Ready = true
	conditions.MarkTrue(vcdCluster, LoadBalancerAvailableCondition)
	if cluster.Status.ControlPlaneReady {
		capvcdRdeManager := capisdk.NewCapvcdRdeManager(vcdClient, vcdCluster.Status.InfraId)
		capvcdRdeManager.AddToEventSet(ctx, capisdk.ControlplaneReady, vcdCluster.Status.InfraId,
			"", "", skipRDEEventUpdates)
	}

	return ctrl.Result{}, nil
}

func (r *VCDClusterReconciler) deleteLB(ctx context.Context, vcdClient *vcdsdk.Client, vcdCluster *infrav1beta3.VCDCluster,
	ovdcNetworkName string, ovdcName string, controlPlanePort int) error {

	log := ctrl.LoggerFrom(ctx)

	if vcdClient == nil {
		return fmt.Errorf("vcdClient is nil")
	}

	// AMK: multiAZ TODO this is probably not needed since we don't create the VDC here
	//err = updateVcdResourceToVcdCluster(vcdCluster, ResourceTypeOvdc, vcdClient.VDC.Vdc.ID, vcdClient.VDC.Vdc.Name)
	//if err != nil {
	//	return errors.Wrapf(err,
	//		"Error updating vcdResource into vcdcluster.status to reconcile Cluster [%s] infrastructure",
	//		vcdCluster.Name)
	//}

	capvcdRdeManager := capisdk.NewCapvcdRdeManager(vcdClient, vcdCluster.Status.InfraId)
	gateway, err := vcdsdk.NewGatewayManager(ctx, vcdClient, ovdcNetworkName,
		vcdCluster.Spec.LoadBalancerConfigSpec.VipSubnet, ovdcName)
	if err != nil {
		capvcdRdeManager.AddToErrorSet(ctx, capisdk.VCDClusterError, "", vcdCluster.Name,
			fmt.Sprintf("failed to create new gateway manager: [%v]", err))
		return errors.Wrapf(err,
			"failed to create gateway manager using the workload client to reconcile cluster [%s]",
			vcdCluster.Name)
	}
	if err = capvcdRdeManager.RdeManager.RemoveErrorByNameOrIdFromErrorSet(ctx, vcdsdk.ComponentCAPVCD,
		capisdk.VCDClusterError, "", ""); err != nil {
		log.Error(err, "failed to remove VCDClusterError from RDE")
	}

	// Delete the load balancer components
	virtualServiceNamePrefix := capisdk.GetVirtualServiceNamePrefix(vcdCluster.Name, vcdCluster.Status.InfraId)
	lbPoolNamePrefix := capisdk.GetVirtualServiceNamePrefix(vcdCluster.Name, vcdCluster.Status.InfraId)

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
		capvcdRdeManager.AddToErrorSet(ctx, capisdk.LoadBalancerError, "", virtualServiceNamePrefix,
			fmt.Sprintf("%v", err))
		return errors.Wrapf(err,
			"Error occurred during cluster [%s] deletion; unable to delete the load balancer [%s]: [%v]",
			vcdCluster.Name, virtualServiceNamePrefix, err)
	}
	log.Info("Deleted the load balancer components (virtual service, lb pool, dnat rule) of the cluster",
		"virtual service", virtualServiceNamePrefix, "lb pool", lbPoolNamePrefix)
	capvcdRdeManager.AddToEventSet(ctx, capisdk.LoadbalancerDeleted, virtualServiceNamePrefix,
		"", "", true)
	if err != nil {
		log.Error(err, "failed to add LoadBalancerDeleted event into RDE", "rdeID", vcdCluster.Status.InfraId)
	}
	err = capvcdRdeManager.RdeManager.RemoveErrorByNameOrIdFromErrorSet(ctx, vcdsdk.ComponentCAPVCD,
		capisdk.LoadBalancerError, "", "")
	if err != nil {
		log.Error(err, "failed to remove LoadBalancerError from RDE", "rdeID", vcdCluster.Status.InfraId)
	}

	if err = capvcdRdeManager.RdeManager.RemoveErrorByNameOrIdFromErrorSet(ctx, vcdsdk.ComponentCAPVCD,
		capisdk.VCDClusterError, "", vcdCluster.Name); err != nil {
		log.Error(err, "failed to remove VCDClusterError from RDE", "rdeID", vcdCluster.Status.InfraId)
	}

	return nil
}

func (r *VCDClusterReconciler) reconcileDeleteSingleVApp(ctx context.Context, ovdcName string,
	vAppName string, vcdClient *vcdsdk.Client, capvcdRdeManager *capisdk.CapvcdRdeManager,
	vcdCluster *infrav1beta3.VCDCluster) (ctrl.Result, error) {

	log := ctrl.LoggerFrom(ctx)

	if vcdClient == nil {
		return ctrl.Result{}, fmt.Errorf("vcdClient is nil")
	}

	vdcManager, err := vcdsdk.NewVDCManager(vcdClient, vcdClient.ClusterOrgName,
		ovdcName)
	if err != nil {
		capvcdRdeManager.AddToErrorSet(ctx, capisdk.VCDClusterError,
			"", vcdCluster.Name, fmt.Sprintf("failed to get vdcManager: [%v]", err))
		return ctrl.Result{}, errors.Wrapf(err,
			"Error creating vdc manager to to reconcile vcd infrastructure for cluster [%s]", vcdCluster.Name)
	}

	vApp, err := vdcManager.Vdc.GetVAppByName(vAppName, true)
	if err != nil && err != govcd.ErrorEntityNotFound {
		log.Error(err, fmt.Sprintf("Error occurred during vApp deletion; vApp [%s] not found",
			vAppName))
		return ctrl.Result{}, errors.Wrapf(err, "Error occurred during vApp deletion")
	}
	if err == govcd.ErrorEntityNotFound {
		log.Info("vApp with name [%s] not found in OVDC [%s]", vAppName, ovdcName)
		return ctrl.Result{}, nil
	}

	if vApp == nil {
		log.Info("nil vApp found for vApp name [%s] in OVDC [%s]", vAppName, ovdcName)
		return ctrl.Result{}, nil
	}

	// TODO: remove the usages of VCDCluster.Status.VAppMetadataUpdated
	//Delete the vApp if and only if rdeId (matches) present in the vApp
	//if !vcdCluster.Status.VAppMetadataUpdated {
	//	err1 := capvcdRdeManager.AddToErrorSet(ctx, capisdk.VCDClusterError, "", vAppName,
	//		fmt.Sprintf("rdeId is not presented in vApp metadata"))
	//	if err1 != nil {
	//		log.Error(err1, "failed to add VCDClusterError into RDE",
	//			"rdeID", vcdCluster.Status.InfraId)
	//	}
	//	return ctrl.Result{}, errors.Errorf(
	//		"Error occurred during cluster deletion; Field [VAppMetadataUpdated] is %t",
	//		vcdCluster.Status.VAppMetadataUpdated)
	//}
	metadataInfraId, err := vdcManager.GetMetadataByKey(vApp, CapvcdInfraId)
	if err != nil {
		capvcdRdeManager.AddToErrorSet(ctx, capisdk.VCDClusterError, "", vAppName,
			fmt.Sprintf("%v", err))
		return ctrl.Result{}, errors.Errorf("Error occurred during fetching metadata in vApp")
	}
	// checking the metadata value and vcdCluster.Status.InfraId are equal or not
	if metadataInfraId != vcdCluster.Status.InfraId {
		capvcdRdeManager.AddToErrorSet(ctx, capisdk.VCDClusterError,
			"", vAppName, fmt.Sprintf("%v", err))
		return ctrl.Result{},
			errors.Errorf("error occurred during cluster deletion; failed to delete vApp [%s]",
				vcdCluster.Name)
	}
	err = capvcdRdeManager.RdeManager.RemoveErrorByNameOrIdFromErrorSet(ctx, vcdsdk.ComponentCAPVCD,
		capisdk.VCDClusterError, "", "")
	if err != nil {
		log.Error(err, "failed to remove VCDClusterError from RDE",
			"rdeID", vcdCluster.Status.InfraId)
	}
	if vApp.VApp.Children != nil {
		capvcdRdeManager.AddToErrorSet(ctx, capisdk.VCDClusterVappDeleteError, "", vcdCluster.Name, fmt.Sprintf(
			"Error occurred during cluster deletion; %d VMs detected in the vApp %s",
			len(vApp.VApp.Children.VM), vcdCluster.Name))
		return ctrl.Result{}, errors.Errorf(
			"Error occurred during cluster deletion; %d VMs detected in the vApp %s",
			len(vApp.VApp.Children.VM), vcdCluster.Name)
	} else {
		log.Info("Deleting vApp of the cluster", "vAppName", vcdCluster.Name)
		err = vdcManager.DeleteVApp(vAppName)
		if err != nil {
			capvcdRdeManager.AddToErrorSet(ctx, capisdk.VCDClusterVappDeleteError,
				"", vAppName, fmt.Sprintf("%v", err))
			return ctrl.Result{}, errors.Wrapf(err,
				"Error occurred during cluster deletion; failed to delete vApp [%s]", vAppName)
		}
		log.Info("Successfully deleted vApp of the cluster", "vAppName", vAppName)
	}

	// Remove vapp from VCDResourceSet in the RDE
	rdeManager := vcdsdk.NewRDEManager(vcdClient, vcdCluster.Status.InfraId,
		capisdk.StatusComponentNameCAPVCD, release.Version)
	err = rdeManager.RemoveFromVCDResourceSet(ctx, vcdsdk.ComponentCAPVCD, VCDResourceVApp, vcdCluster.Name)
	if err != nil {
		log.Error(
			fmt.Errorf("failed to remove VCD resource [%s] from VCD resource set of RDE [%s]: [%v]",
				VCDResourceVApp, vcdCluster.Status.InfraId, err),
			"error occurred while removing VCD resource from VCD resource set in RDE")
		capvcdRdeManager.AddToErrorSet(ctx, capisdk.RdeError, "", vcdCluster.Name,
			fmt.Sprintf("failed to delete VCD Resource [%s] of type [%s] from VCDResourceSet of RDE [%s]: [%v]",
				vcdCluster.Name, VCDResourceVApp, vcdCluster.Status.InfraId, err))
	}
	if err = capvcdRdeManager.RdeManager.RemoveErrorByNameOrIdFromErrorSet(ctx, vcdsdk.ComponentCAPVCD, capisdk.RdeError, "", vcdCluster.Name); err != nil {
		log.Error(err, "failed to remove RdeError from RDE", "rdeID", vcdCluster.Status.InfraId)
	}
	capvcdRdeManager.AddToEventSet(ctx, capisdk.VappDeleted, "", "", "", true)
	err = capvcdRdeManager.RdeManager.RemoveErrorByNameOrIdFromErrorSet(ctx, vcdsdk.ComponentCAPVCD, capisdk.VCDClusterVappDeleteError, "", "")
	if err != nil {
		log.Error(err, "failed to remove vAppDeleteError from RDE", "rdeID", vcdCluster.Status.InfraId)
	}

	return ctrl.Result{}, nil
}

func (r *VCDClusterReconciler) reconcileDeleteVApps(ctx context.Context,
	vcdCluster *infrav1beta3.VCDCluster, vcdClient *vcdsdk.Client) (ctrl.Result, error) {

	log := ctrl.LoggerFrom(ctx)

	// TODO (multi-AZ): we need to find all the VApps in different orgs and delete all of them

	if vcdClient == nil {
		return ctrl.Result{}, fmt.Errorf("vcdClient is nil")
	}

	capvcdRDEManager := capisdk.NewCapvcdRdeManager(vcdClient, vcdCluster.Status.InfraId)

	result, err := r.reconcileDeleteSingleVApp(ctx, vcdCluster.Spec.Ovdc, vcdCluster.Name,
		vcdClient, capvcdRDEManager, vcdCluster)
	if err != nil {
		// this is potentially an irrecoverable FATAL error
		log.Error(err, "unable to delete single vApp",
			"orgName", vcdClient.ClusterOrgName, "ovdcName", vcdCluster.Spec.Ovdc,
			"vAppName", vcdCluster.Name)
		return result, errors.Wrapf(err,
			"unable to get delete single vApp [%s] in Org [%s], OVDC [%s]", vcdCluster.Name,
			vcdClient.ClusterOrgName, vcdCluster.Spec.Ovdc)
	}
	log.Info("Successfully deleted vApp", "vAppName", vcdCluster.Name,
		"org", vcdClient.ClusterOrgName, "ovdc", vcdCluster.Spec.Ovdc)
	return ctrl.Result{}, nil
}

func (r *VCDClusterReconciler) reconcileDeleteRDE(ctx context.Context, vcdClient *vcdsdk.Client, vcdCluster *infrav1beta3.VCDCluster) error {

	log := ctrl.LoggerFrom(ctx)

	if vcdClient == nil {
		return fmt.Errorf("vcdClient is nil")
	}

	log.Info("Deleting RDE for the cluster", "InfraID", vcdCluster.Status.InfraId)
	// TODO: If RDE deletion fails, should we throw an error during reconciliation?
	// Delete RDE
	capvcdRdeManager := capisdk.NewCapvcdRdeManager(vcdClient, vcdCluster.Status.InfraId)

	if vcdCluster.Status.InfraId != "" && !strings.HasPrefix(vcdCluster.Status.InfraId, NoRdePrefix) {
		org, err := vcdClient.VCDClient.GetOrgByName(vcdClient.ClusterOrgName)
		if err != nil {
			return errors.Wrapf(errors.New("failed to get org by name"), "error getting org by name for org [%s]: [%v]", vcdClient.ClusterOrgName, err)
		}
		if org == nil || org.Org == nil {
			return errors.Wrapf(errors.New("invalid org ref obtained"),
				"obtained nil org when getting org by name [%s]", vcdClient.ClusterOrgName)
		}
		definedEntities, resp, err := vcdClient.APIClient.DefinedEntityApi.GetDefinedEntitiesByEntityType(ctx,
			capisdk.CAPVCDTypeVendor, capisdk.CAPVCDTypeNss, capisdk.CAPVCDEntityTypeDefaultMajorVersion, org.Org.ID, 1, 25,
			&swagger.DefinedEntityApiGetDefinedEntitiesByEntityTypeOpts{
				Filter: optional.NewString(fmt.Sprintf("id==%s", vcdCluster.Status.InfraId)),
			})
		if err != nil {
			capvcdRdeManager.AddToErrorSet(ctx, capisdk.RdeError, "", "", fmt.Sprintf("failed to get RDE [%s]: %v", vcdCluster.Status.InfraId, err))
			return errors.Wrapf(err, "Error occurred during RDE deletion; failed to fetch defined entities by entity type [%s] and ID [%s] for cluster [%s]", CAPVCDEntityTypeID, vcdCluster.Status.InfraId, vcdCluster.Name)
		}
		if resp != nil && resp.StatusCode != http.StatusOK {
			capvcdRdeManager.AddToErrorSet(ctx, capisdk.RdeError, "", "", fmt.Sprintf("Got wrong status code while fetching RDE [%s]: %v", vcdCluster.Status.InfraId, err))
			return errors.Errorf("Error occurred during RDE deletion; error while fetching defined entities by entity type [%s] and ID [%s] for cluster [%s]", CAPVCDEntityTypeID, vcdCluster.Status.InfraId, vcdCluster.Name)
		}
		if len(definedEntities.Values) > 0 {
			// resolve defined entity before deleting
			entityState, resp, err := vcdClient.APIClient.DefinedEntityApi.ResolveDefinedEntity(ctx,
				vcdCluster.Status.InfraId, org.Org.ID)
			if err != nil {
				capvcdRdeManager.AddToErrorSet(ctx, capisdk.RdeError, "", "", fmt.Sprintf("failed to resolve entity: [%v]", err))
				return errors.Wrapf(err, "Error occurred during RDE deletion; error occurred while resolving defined entity [%s] with ID [%s] before deleting", vcdCluster.Name, vcdCluster.Status.InfraId)
			}
			if resp.StatusCode != http.StatusOK {
				log.Error(nil, "Error occurred during RDE deletion; failed to resolve RDE with ID [%s] for cluster [%s]: [%s]", vcdCluster.Status.InfraId, vcdCluster.Name, entityState.Message)
			}
			resp, err = vcdClient.APIClient.DefinedEntityApi.DeleteDefinedEntity(ctx,
				vcdCluster.Status.InfraId, org.Org.ID, nil)
			if err != nil {
				capvcdRdeManager.AddToErrorSet(ctx, capisdk.RdeError, "", "", fmt.Sprintf("%v", err))
				return errors.Wrapf(err, "error occurred during RDE deletion; failed to execute delete defined entity call for RDE with ID [%s]", vcdCluster.Status.InfraId)
			}
			if resp.StatusCode != http.StatusNoContent {
				capvcdRdeManager.AddToErrorSet(ctx, capisdk.RdeError, "", "", fmt.Sprintf("%v", err))
				return errors.Errorf("Error occurred during RDE deletion; error deleting defined entity associated with the cluster. RDE id: [%s]", vcdCluster.Status.InfraId)
			}
			log.Info("Successfully deleted the (RDE) defined entity of the cluster")
		} else {
			log.Info("Attempted deleting the RDE, but corresponding defined entity is not found", "RDEId", vcdCluster.Status.InfraId)
		}
	}

	return nil
}

func (r *VCDClusterReconciler) reconcileDelete(ctx context.Context,
	vcdCluster *infrav1beta3.VCDCluster) (ctrl.Result, error) {

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

	vcdClient, err := createVCDClientFromSecrets(ctx, r.Client, vcdCluster, "")
	// close all idle connections when reconciliation is done
	defer func() {
		if vcdClient != nil && vcdClient.VCDClient != nil {
			vcdClient.VCDClient.Client.Http.CloseIdleConnections()
			log.Info(fmt.Sprintf("closed connection to the http client [%#v]", vcdClient.VCDClient.Client.Http))
		}
	}()
	if err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "Error creating VCD client to reconcile Cluster [%s] infrastructure", vcdCluster.Name)
	}

	ovdcName := vcdCluster.Spec.Ovdc
	ovdcNetworkName := vcdCluster.Spec.OvdcNetwork
	controlPlaneHost := vcdCluster.Spec.ControlPlaneEndpoint.Host
	controlPlanePort := vcdCluster.Spec.ControlPlaneEndpoint.Port
	if controlPlanePort == 0 {
		controlPlanePort = TcpPort
	}
	if err = r.deleteLB(ctx, vcdClient, vcdCluster, ovdcNetworkName, ovdcName, controlPlanePort); err != nil {
		return ctrl.Result{}, errors.Wrapf(err,
			"unable to delete LB with control plane host [%s], port[%d] in ovdc [%s] and network [%s]: [%v]",
			controlPlaneHost, controlPlanePort, ovdcName, ovdcNetworkName, err)
	}

	// Delete vApp
	result, err := r.reconcileDeleteVApps(ctx, vcdCluster, vcdClient)
	if err != nil {
		log.Error(err, "unable to delete vApps of cluster", "cluster", vcdCluster.Name)
		return result, errors.Wrapf(err, "error occurred during cluster deletion; failed to delete vApp [%s]",
			vcdCluster.Name)
	}

	// Delete RDE
	if deleteErr := r.reconcileDeleteRDE(ctx, vcdClient, vcdCluster); deleteErr != nil {
		log.Error(err, "Error occurred while deleting RDE: [%v]", deleteErr)
		return ctrl.Result{}, errors.Wrapf(err, "error occurred during deleting RDE for the cluster [%s]",
			vcdCluster.Status.InfraId)
	}

	log.Info("Successfully deleted all the infra resources of the cluster")
	// Cluster is deleted so remove the finalizer.
	controllerutil.RemoveFinalizer(vcdCluster, infrav1beta3.ClusterFinalizer)

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *VCDClusterReconciler) SetupWithManager(mgr ctrl.Manager, options controller.Options) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&infrav1beta3.VCDCluster{}).
		WithOptions(options).
		Complete(r)
}
