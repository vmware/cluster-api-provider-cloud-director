/*
   Copyright 2021 VMware, Inc.
   SPDX-License-Identifier: Apache-2.0
*/

package controllers

import (
	"context"
	_ "embed"
	"fmt"
	"github.com/antihax/optional"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/vmware/cloud-provider-for-cloud-director/pkg/vcdsdk"
	swagger "github.com/vmware/cloud-provider-for-cloud-director/pkg/vcdswaggerclient"
	infrav1 "github.com/vmware/cluster-api-provider-cloud-director/api/v1beta1"
	"github.com/vmware/cluster-api-provider-cloud-director/pkg/capisdk"
	"github.com/vmware/cluster-api-provider-cloud-director/pkg/config"
	vcdutil "github.com/vmware/cluster-api-provider-cloud-director/pkg/util"
	rdeType "github.com/vmware/cluster-api-provider-cloud-director/pkg/vcdtypes/rde_type_1_1_0"
	"github.com/vmware/go-vcloud-director/v2/govcd"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"net/http"
	"reflect"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/conditions"
	kcfg "sigs.k8s.io/cluster-api/util/kubeconfig"
	"sigs.k8s.io/cluster-api/util/patch"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"strings"
	"time"
)

const (
	CAPVCDTypeVendor  = "vmware"
	CAPVCDTypeNss     = "capvcdCluster"
	CAPVCDTypeVersion = "1.1.0"

	CAPVCDClusterCniName = "antrea" // TODO: Get the correct value for CNI name

	RDEStatusResolved = "RESOLVED"
	VCDLocationHeader = "Location"

	ClusterApiStatusPhaseReady    = "Ready"
	ClusterApiStatusPhaseNotReady = "Not Ready"

	NoRdePrefix = `NO_RDE_`
)

var (
	CAPVCDEntityTypeID = fmt.Sprintf("urn:vcloud:type:%s:%s:%s", CAPVCDTypeVendor, CAPVCDTypeNss, CAPVCDTypeVersion)
)

// VCDClusterReconciler reconciles a VCDCluster object
type VCDClusterReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Config *config.CAPVCDConfig
}

//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=vcdclusters,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=vcdclusters/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=vcdclusters/finalizers,verbs=update
//+kubebuilder:rbac:groups="",resources=secrets;,verbs=get;list;watch
//+kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch
//+kubebuilder:rbac:groups="",resources=events,verbs=get;list;watch;create;update;patch
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

// TODO: Remove uncommented code when decision to only keep capi.yaml as part of RDE spec is finalized
func (r *VCDClusterReconciler) constructCapvcdRDE(ctx context.Context, cluster *clusterv1.Cluster,
	vcdCluster *infrav1.VCDCluster) (*swagger.DefinedEntity, error) {
	org := vcdCluster.Spec.Org
	vdc := vcdCluster.Spec.Ovdc

	kcpList, err := getAllKubeadmControlPlaneForCluster(ctx, r.Client, *cluster)
	if err != nil {
		return nil, fmt.Errorf("error getting KubeadmControlPlane objects for cluster [%s]: [%v]", vcdCluster.Name, err)
	}

	kubernetesVersion := ""
	for _, kcp := range kcpList.Items {
		kubernetesVersion = kcp.Spec.Version
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
			Org:  org,
			Vdc:  vdc,
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
				NodeStatus:             make(map[string]string),
				CapvcdVersion:          r.Config.ClusterResources.CapvcdVersion,
				UseAsManagementCluster: vcdCluster.Spec.UseAsManagementCluster,
				K8sNetwork: rdeType.K8sNetwork{
					Cni: rdeType.Cni{
						Name: CAPVCDClusterCniName,
					},
					Pods: rdeType.Pods{
						CidrBlocks: cluster.Spec.ClusterNetwork.Pods.CIDRBlocks,
					},
					Services: rdeType.Services{
						CidrBlocks: cluster.Spec.ClusterNetwork.Services.CIDRBlocks,
					},
				},
				ParentUID: vcdCluster.Spec.ParentUID,
				VcdProperties: rdeType.VCDProperties{
					Site:        vcdCluster.Spec.Site,
					Org:         org,
					Vdc:         vdc,
					OvdcNetwork: vcdCluster.Spec.OvdcNetwork,
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

	rde, err := r.constructCapvcdRDE(ctx, cluster, vcdCluster)
	if err != nil {
		return "", fmt.Errorf("error occurred while constructing RDE payload for the cluster [%s]: [%v]", vcdCluster.Name, err)
	}
	resp, err := workloadVCDClient.APIClient.DefinedEntityApi.CreateDefinedEntity(ctx, *rde,
		rde.EntityType, nil)
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

// TODO: Remove uncommented code when decision to only keep capi.yaml as part of RDE spec is finalized
func (r *VCDClusterReconciler) reconcileRDE(ctx context.Context, cluster *clusterv1.Cluster, vcdCluster *infrav1.VCDCluster, workloadVCDClient *vcdsdk.Client) error {
	log := ctrl.LoggerFrom(ctx)

	capvcdRdeManager := capisdk.NewCapvcdRdeManager(workloadVCDClient)

	_, capvcdSpec, capvcdMetadata, capvcdStatus, err := capvcdRdeManager.GetCAPVCDEntity(ctx, vcdCluster.Status.InfraId)
	if err != nil {
		return fmt.Errorf("failed to get RDE with ID [%s] for cluster [%s]: [%v]", vcdCluster.Status.InfraId, vcdCluster.Name, err)
	}
	// TODO(VCDA-3107): Should we be updating org and vdc information here.
	metadataPatch := make(map[string]interface{})
	org := vcdCluster.Spec.Org
	if org != capvcdMetadata.Org {
		metadataPatch["Org"] = org
	}

	vdc := vcdCluster.Spec.Ovdc
	if vdc != capvcdMetadata.Vdc {
		metadataPatch["Vdc"] = vdc
	}

	if capvcdMetadata.Site != vcdCluster.Spec.Site {
		metadataPatch["Site"] = vcdCluster.Spec.Site
	}

	specPatch := make(map[string]interface{})
	kcpList, err := getAllKubeadmControlPlaneForCluster(ctx, r.Client, *cluster)
	if err != nil {
		return fmt.Errorf("error getting all KubeadmControlPlane objects for cluster [%s]: [%v]", vcdCluster.Name, err)
	}

	kubernetesVersion := ""
	for _, kcp := range kcpList.Items {
		kubernetesVersion = kcp.Spec.Version
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

	// TODO: Delete "kubernetes" string in RDE. Discuss with Sahithi
	if capvcdStatus.Kubernetes != kubernetesVersion {
		capvcdStatusPatch["Kubernetes"] = kubernetesVersion
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

	cni := rdeType.Cni{
		Name: CAPVCDClusterCniName,
	}
	if !reflect.DeepEqual(capvcdStatus.K8sNetwork.Cni, cni) {
		capvcdStatusPatch["K8sNetwork.Cni"] = cni
	}

	pods := rdeType.Pods{
		CidrBlocks: cluster.Spec.ClusterNetwork.Pods.CIDRBlocks,
	}
	if !reflect.DeepEqual(capvcdStatus.K8sNetwork.Pods, pods) {
		capvcdStatusPatch["K8sNetwork.Pods"] = pods
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
	clusterApiStatus := rdeType.ClusterApiStatus{
		Phase: clusterApiStatusPhase,
		ApiEndpoints: []rdeType.ApiEndpoints{
			{
				Host: vcdCluster.Spec.ControlPlaneEndpoint.Host,
				Port: 6443,
			},
		},
	}
	if !reflect.DeepEqual(clusterApiStatus, capvcdStatus.ClusterAPIStatus) {
		capvcdStatusPatch["ClusterAPIStatus"] = clusterApiStatus
	}

	// update node status. Needed to remove stray nodes which were already deleted
	machineList, err := getMachineListFromCluster(ctx, r.Client, *cluster)
	if err != nil {
		return fmt.Errorf("error getting machine list for cluster with name [%s]: [%v]", cluster.Name, err)
	}

	updatedNodeStatus := make(map[string]string)
	for _, machine := range machineList.Items {
		updatedNodeStatus[machine.Name] = machine.Status.Phase
	}
	if !reflect.DeepEqual(updatedNodeStatus, capvcdStatus.NodeStatus) {
		capvcdStatusPatch["NodeStatus"] = updatedNodeStatus
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

	updatedRDE, err := capvcdRdeManager.PatchRDE(ctx, specPatch, metadataPatch, capvcdStatusPatch, vcdCluster.Status.InfraId)
	if err != nil {
		return fmt.Errorf("failed to update defined entity with ID [%s] for cluster [%s]: [%v]", vcdCluster.Status.InfraId, vcdCluster.Name, err)
	}

	if updatedRDE.State != RDEStatusResolved {
		// try to resolve the defined entity
		entityState, resp, err := workloadVCDClient.APIClient.DefinedEntityApi.ResolveDefinedEntity(ctx, updatedRDE.Id)
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
	workloadVCDClient, err := vcdsdk.NewVCDClientFromSecrets(vcdCluster.Spec.Site, vcdCluster.Spec.Org,
		vcdCluster.Spec.Ovdc, vcdCluster.Spec.Org, vcdCluster.Spec.UserCredentialsContext.Username,
		vcdCluster.Spec.UserCredentialsContext.Password, vcdCluster.Spec.UserCredentialsContext.RefreshToken,
		true, true)
	if err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "Error creating VCD client to reconcile Cluster [%s] infrastructure", vcdCluster.Name)
	}

	gateway, err := vcdsdk.NewGatewayManager(ctx, workloadVCDClient, vcdCluster.Spec.OvdcNetwork, r.Config.VCD.VIPSubnet)
	if err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "failed to create gateway manager using the workload client to reconcile cluster [%s]", vcdCluster.Name)
	}

	infraID := vcdCluster.Status.InfraId

	// General note on RDE operations, always ensure CAPVCD cluster reconciliation progress
	//is not affected by any RDE operation failures.

	// Use the pre-created RDEId specified in the CAPI yaml specification.
	// TODO validate if the RDE ID format is correct.
	if infraID == "" && len(vcdCluster.Spec.RDEId) > 0 {
		infraID = vcdCluster.Spec.RDEId
	}

	// Create a new RDE if it was not already created or assigned.
	if infraID == "" {
		nameFilter := &swagger.DefinedEntityApiGetDefinedEntitiesByEntityTypeOpts{
			Filter: optional.NewString(fmt.Sprintf("name==%s", vcdCluster.Name)),
		}
		definedEntities, resp, err := workloadVCDClient.APIClient.DefinedEntityApi.GetDefinedEntitiesByEntityType(ctx,
			CAPVCDTypeVendor, CAPVCDTypeNss, CAPVCDTypeVersion, 1, 25, nameFilter)
		if err != nil {
			log.Error(err, "Error while checking if RDE is already present for the cluster",
				"entityTypeId", CAPVCDEntityTypeID)
		}
		if resp == nil {
			log.Error(nil, "Error while checking if RDE is already present for the cluster; "+
				"obtained an empty response for get defined entity call for the cluster")
		} else if resp.StatusCode != http.StatusOK {
			log.Error(nil, "Error while checking if RDE is already present for the cluster",
				"entityTypeId", CAPVCDEntityTypeID, "responseStatusCode", resp.StatusCode)
		}
		if err == nil && resp != nil && resp.StatusCode == http.StatusOK {
			if len(definedEntities.Values) == 0 {
				rdeID, err := r.constructAndCreateRDEFromCluster(ctx, workloadVCDClient, cluster, vcdCluster)
				if err != nil {
					log.Error(err, "Error creating RDE for the cluster")
				} else {
					infraID = rdeID
				}
			} else {
				log.Info("RDE for the cluster is already present; skipping RDE creation", "InfraId",
					definedEntities.Values[0].Id)
				infraID = definedEntities.Values[0].Id
			}
		}
	} else {
		log.V(3).Info("Reusing already available InfraID", "infraID", infraID)
		capvcdRdeManager := capisdk.NewCapvcdRdeManager(workloadVCDClient)
		if !strings.Contains(infraID, NoRdePrefix) {
			_, err = capvcdRdeManager.ConvertRDE(ctx, infraID)
			if err != nil {
				log.Error(err, "failed to upgrade RDE", "rdeID", infraID)
				return ctrl.Result{}, errors.Wrapf(err, "failed to upgrade RDE [%s]", infraID)
			}
		}
	}

	// If there is no RDE ID specified (or) created for any reason, self-generate one and use.
	// We need UUIDs to single-instance cleanly in the Virtual Services etc.
	if infraID == "" {
		noRDEID := NoRdePrefix + uuid.New().String()
		log.Info("Error retrieving InfraId. Hence using a self-generated UUID", "UUID", noRDEID)
		infraID = noRDEID
	}

	if vcdCluster.Status.InfraId == "" {
		// This implies we have to set the infraID into the vcdCluster Object.
		vcdCluster.Status.InfraId = infraID
		if err := r.Status().Update(ctx, vcdCluster); err != nil {
			return ctrl.Result{}, errors.Wrapf(err,
				"unable to update status of vcdCluster [%s] with InfraID [%s]",
				vcdCluster.Name, infraID)
		}
	}

	// After InfraId has been set, we can update parentUid, useAsMgmtCluster status
	vcdCluster.Status.UseAsManagementCluster = vcdCluster.Spec.UseAsManagementCluster
	vcdCluster.Status.ParentUID = vcdCluster.Spec.ParentUID

	// create load balancer for the cluster. Only one-arm load balancer is fully tested.
	virtualServiceNamePrefix := capisdk.GetVirtualServiceNamePrefix(vcdCluster.Name, vcdCluster.Status.InfraId)
	lbPoolNamePrefix := capisdk.GetLoadBalancerPoolNamePrefix(vcdCluster.Name, vcdCluster.Status.InfraId)

	controlPlaneNodeIP, err := gateway.GetLoadBalancer(ctx, fmt.Sprintf("%s-tcp", virtualServiceNamePrefix),
		(*vcdsdk.OneArm)(r.Config.LB.OneArm))
	//TODO: Sahithi: Check if error is really because of missing virtual service.
	// In any other error cases, force create the new load balancer with the original control plane endpoint
	// (if already present). Do not overwrite the existing control plane endpoint with a new endpoint.
	if err != nil || controlPlaneNodeIP == "" {
		if vsError, ok := err.(*vcdsdk.VirtualServicePendingError); ok {
			log.Info("Error getting load balancer. Virtual Service is still pending",
				"virtualServiceName", vsError.VirtualServiceName, "error", err)
			return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
		}

		log.Info("Creating load balancer for the cluster")
		capvcdGatewayManger, err := capisdk.NewCapvcdGatewayManager(ctx, workloadVCDClient, vcdCluster.Spec.OvdcNetwork, r.Config.VCD.VIPSubnet)
		if err != nil {
			return ctrl.Result{}, errors.Wrapf(err, "failed to create capvcd's gateway manager object to reconcile cluster [%s]",
				vcdCluster.Name)
		}
		controlPlaneNodeIP, err = capvcdGatewayManger.CreateL4LoadBalancer(ctx, virtualServiceNamePrefix, lbPoolNamePrefix,
			[]string{}, r.Config.LB.Ports.TCP, r.Config.LB.Ports.TCP, (*vcdsdk.OneArm)(r.Config.LB.OneArm))
		if err != nil {
			if vsError, ok := err.(*vcdsdk.VirtualServicePendingError); ok {
				log.Info("Error creating load balancer for cluster. Virtual Service is still pending",
					"virtualServiceName", vsError.VirtualServiceName, "error", err)
				return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
			}
			return ctrl.Result{}, errors.Wrapf(err,
				"Error creating create load balancer [%s] for the cluster [%s]: [%v]",
				virtualServiceNamePrefix, vcdCluster.Name, err)
		}
	}
	vcdCluster.Spec.ControlPlaneEndpoint = infrav1.APIEndpoint{
		Host: controlPlaneNodeIP,
		Port: 6443,
	}
	log.Info(fmt.Sprintf("Control plane endpoint for the cluster is [%s]", controlPlaneNodeIP))

	if !strings.HasPrefix(vcdCluster.Status.InfraId, NoRdePrefix) {
		_, resp, _, err := workloadVCDClient.APIClient.DefinedEntityApi.GetDefinedEntity(ctx, vcdCluster.Status.InfraId)
		if err == nil && resp != nil && resp.StatusCode == http.StatusOK {
			if err = r.reconcileRDE(ctx, cluster, vcdCluster, workloadVCDClient); err != nil {
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
		return ctrl.Result{}, errors.Wrapf(err, "Error creating vdc manager to to reconcile vcd infrastructure for cluster [%s]", vcdCluster.Name)
	}
	_, err = vdcManager.GetOrCreateVApp(vcdCluster.Name, vcdCluster.Spec.OvdcNetwork)
	if err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "Error creating Infra vApp for the cluster [%s]: [%v]", vcdCluster.Name, err)
	}

	// Update the vcdCluster resource with updated information
	// TODO Check if updating ovdcNetwork, Org and Ovdc should be done somewhere earlier in the code.
	vcdCluster.ClusterName = vcdCluster.Name

	vcdCluster.Status.Ready = true
	conditions.MarkTrue(vcdCluster, LoadBalancerAvailableCondition)

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

	if err := patchVCDCluster(ctx, patchHelper, vcdCluster); err != nil {
		return ctrl.Result{}, errors.Wrap(err, "Error occurred during cluster deletion; failed to patch VCDCluster")
	}

	workloadVCDClient, err := vcdsdk.NewVCDClientFromSecrets(vcdCluster.Spec.Site, vcdCluster.Spec.Org,
		vcdCluster.Spec.Ovdc, vcdCluster.Spec.Org, vcdCluster.Spec.UserCredentialsContext.Username,
		vcdCluster.Spec.UserCredentialsContext.Password, vcdCluster.Spec.UserCredentialsContext.RefreshToken,
		true, true)
	if err != nil {
		return ctrl.Result{}, errors.Wrapf(err,
			"Error occurred during cluster deletion; unable to create client for the workload cluster [%s]",
			vcdCluster.Name)
	}

	gateway, err := capisdk.NewCapvcdGatewayManager(ctx, workloadVCDClient, vcdCluster.Spec.OvdcNetwork, r.Config.VCD.VIPSubnet)
	if err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "failed to create gateway manager using the workload client to reconcile cluster [%s]", vcdCluster.Name)
	}

	// Delete the load balancer components
	virtualServiceNamePrefix := capisdk.GetVirtualServiceNamePrefix(vcdCluster.Name, vcdCluster.Status.InfraId)
	lbPoolNamePrefix := capisdk.GetLoadBalancerPoolNamePrefix(vcdCluster.Name, vcdCluster.Status.InfraId)
	err = gateway.DeleteLoadBalancer(ctx, virtualServiceNamePrefix, lbPoolNamePrefix, (*vcdsdk.OneArm)(r.Config.LB.OneArm))
	if err != nil {
		return ctrl.Result{}, errors.Wrapf(err,
			"Error occurred during cluster [%s] deletion; unable to delete the load balancer [%s]: [%v]",
			vcdCluster.Name, virtualServiceNamePrefix, err)
	}
	log.Info("Deleted the load balancer components (virtual service, lb pool, dnat rule) of the cluster",
		"virtual service", virtualServiceNamePrefix, "lb pool", lbPoolNamePrefix)

	vdcManager, err := vcdsdk.NewVDCManager(workloadVCDClient, workloadVCDClient.ClusterOrgName,
		workloadVCDClient.ClusterOVDCName)
	if err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "Error creating vdc manager to to reconcile vcd infrastructure for cluster [%s]", vcdCluster.Name)
	}

	// Delete vApp
	vApp, err := workloadVCDClient.VDC.GetVAppByName(vcdCluster.Name, true)
	if err != nil {
		log.Error(err, fmt.Sprintf("Error occurred during cluster deletion; vApp [%s] not found", vcdCluster.Name))
	}
	if vApp != nil {
		if vApp.VApp.Children != nil {
			return ctrl.Result{}, errors.Errorf(
				"Error occurred during cluster deletion; %d VMs detected in the vApp %s",
				len(vApp.VApp.Children.VM), vcdCluster.Name)
		} else {
			log.Info("Deleting vApp of the cluster", "vAppName", vcdCluster.Name)
			err = vdcManager.DeleteVApp(vcdCluster.Name)
			if err != nil {
				return ctrl.Result{}, errors.Wrapf(err,
					"Error occurred during cluster deletion; failed to delete vApp [%s]", vcdCluster.Name)
			}
			log.Info("Successfully deleted vApp of the cluster", "vAppName", vcdCluster.Name)
		}
	}

	// TODO: If RDE deletion fails, should we throw an error during reconciliation?
	// Delete RDE
	if vcdCluster.Status.InfraId != "" && !strings.HasPrefix(vcdCluster.Status.InfraId, NoRdePrefix) {
		definedEntities, resp, err := workloadVCDClient.APIClient.DefinedEntityApi.GetDefinedEntitiesByEntityType(ctx,
			CAPVCDTypeVendor, CAPVCDTypeNss, CAPVCDTypeVersion, 1, 25,
			&swagger.DefinedEntityApiGetDefinedEntitiesByEntityTypeOpts{
				Filter: optional.NewString(fmt.Sprintf("id==%s", vcdCluster.Status.InfraId)),
			})
		if err != nil {
			return ctrl.Result{}, errors.Wrapf(err, "Error occurred during RDE deletion; failed to fetch defined entities by entity type [%s] and ID [%s] for cluster [%s]", CAPVCDEntityTypeID, vcdCluster.Status.InfraId, vcdCluster.Name)
		}
		if resp.StatusCode != http.StatusOK {
			return ctrl.Result{}, errors.Errorf("Error occurred during RDE deletion; error while fetching defined entities by entity type [%s] and ID [%s] for cluster [%s]", CAPVCDEntityTypeID, vcdCluster.Status.InfraId, vcdCluster.Name)
		}
		if len(definedEntities.Values) > 0 {
			// resolve defined entity before deleting
			entityState, resp, err := workloadVCDClient.APIClient.DefinedEntityApi.ResolveDefinedEntity(ctx,
				vcdCluster.Status.InfraId)
			if err != nil {
				return ctrl.Result{}, errors.Wrapf(err, "Error occurred during RDE deletion; error occurred while resolving defined entity [%s] with ID [%s] before deleting", vcdCluster.Name, vcdCluster.Status.InfraId)
			}
			if resp.StatusCode != http.StatusOK {
				log.Error(nil, "Error occurred during RDE deletion; failed to resolve RDE with ID [%s] for cluster [%s]: [%s]", vcdCluster.Status.InfraId, vcdCluster.Name, entityState.Message)
			}
			resp, err = workloadVCDClient.APIClient.DefinedEntityApi.DeleteDefinedEntity(ctx,
				vcdCluster.Status.InfraId, nil)
			if err != nil {
				return ctrl.Result{}, errors.Wrapf(err, "error occurred during RDE deletion; failed to execute delete defined entity call for RDE with ID [%s]", vcdCluster.Status.InfraId)
			}
			if resp.StatusCode != http.StatusNoContent {
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
