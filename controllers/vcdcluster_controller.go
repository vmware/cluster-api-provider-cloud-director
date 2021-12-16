/*
   Copyright 2021 VMware, Inc.
   SPDX-License-Identifier: Apache-2.0
*/

package controllers

import (
	"context"
	"fmt"
	"github.com/antihax/optional"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	infrav1 "github.com/vmware/cluster-api-provider-cloud-director/api/v1alpha4"
	vcdutil "github.com/vmware/cluster-api-provider-cloud-director/pkg/util"
	"github.com/vmware/cluster-api-provider-cloud-director/pkg/vcdclient"
	swagger "github.com/vmware/cluster-api-provider-cloud-director/pkg/vcdswaggerclient"
	"github.com/vmware/cluster-api-provider-cloud-director/pkg/vcdtypes"
	"github.com/vmware/go-vcloud-director/v2/govcd"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"net/http"
	"reflect"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha4"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/cluster-api/util/patch"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"strings"
)

const (
	CAPVCDTypeVendor  = "vmware"
	CAPVCDTypeNss     = "capvcd"
	CAPVCDTypeVersion = "1.0.0"

	CAPVCDClusterKind             = "CAPVCDCluster"
	CAPVCDClusterEntityApiVersion = "capvcd.vmware.com/v1.0"
	CAPVCDClusterCniName          = "antrea" // TODO: Get the correct value for CNI name
	VcdCsiName                    = "cloud-director-named-disk-csi-driver"
	VcdCpiName                    = "cloud-provider-for-cloud-director"

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
	Scheme    *runtime.Scheme
	VcdClient *vcdclient.Client
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
	if err := r.Client.Get(ctx, req.NamespacedName, vcdCluster); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Fetch the Cluster.
	cluster, err := util.GetOwnerCluster(ctx, r.Client, vcdCluster.ObjectMeta)
	if err != nil {
		return ctrl.Result{}, err
	}
	if cluster == nil {
		log.Info("Waiting for Cluster Controller to set OwnerRef on VCDCluster")
		return ctrl.Result{}, nil
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
	}()

	if !controllerutil.ContainsFinalizer(vcdCluster, infrav1.ClusterFinalizer) {
		controllerutil.AddFinalizer(vcdCluster, infrav1.ClusterFinalizer)
		return ctrl.Result{}, nil
	}

	if !vcdCluster.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, vcdCluster)
	}

	return r.reconcileNormal(ctx, cluster, vcdCluster)
}

func patchVCDCluster(ctx context.Context, patchHelper *patch.Helper, vcdCluster *infrav1.VCDCluster) error {
	conditions.SetSummary(vcdCluster,
		conditions.WithConditions(
			infrav1.LoadBalancerAvailableCondition,
		),
		conditions.WithStepCounterIf(vcdCluster.ObjectMeta.DeletionTimestamp.IsZero()),
	)

	return patchHelper.Patch(
		ctx,
		vcdCluster,
		patch.WithOwnedConditions{Conditions: []clusterv1.ConditionType{
			clusterv1.ReadyCondition,
			infrav1.LoadBalancerAvailableCondition,
		}},
	)
}

func (r *VCDClusterReconciler) constructCapvcdRDE(ctx context.Context, cluster *clusterv1.Cluster,
	vcdCluster *infrav1.VCDCluster) (*swagger.DefinedEntity, error) {
	org := vcdCluster.Spec.Org
	vdc := vcdCluster.Spec.Ovdc
	ovdcNetwork := vcdCluster.Spec.OvdcNetwork

	kcpList, err := getAllKubeadmControlPlaneForCluster(ctx, r.Client, *cluster)
	if err != nil {
		return nil, fmt.Errorf("error getting KubeadmControlPlane objects for cluster [%s]: [%v]", vcdCluster.Name, err)
	}
	topologyControlPlanes := make([]vcdtypes.ControlPlane, len(kcpList.Items))
	kubernetesVersion := ""
	for _, kcp := range kcpList.Items {
		vcdMachineTemplate, err := getVCDMachineTemplateFromKCP(ctx, r.Client, kcp)
		if err != nil {
			return nil, fmt.Errorf("error getting the VCDMachineTemplate object from KCP [%s] for cluster [%s]: [%v]", kcp.Name, cluster.Name, err)
		}
		topologyControlPlane := vcdtypes.ControlPlane{
			Count:        *kcp.Spec.Replicas,
			SizingClass:  vcdMachineTemplate.Spec.Template.Spec.ComputePolicy,
			TemplateName: vcdMachineTemplate.Spec.Template.Spec.Template,
		}
		topologyControlPlanes = append(topologyControlPlanes, topologyControlPlane)
		kubernetesVersion = kcp.Spec.Version
	}

	mdList, err := getAllMachineDeploymentsForCluster(ctx, r.Client, *cluster)
	if err != nil {
		return nil, fmt.Errorf("error getting the MachineDeployments for cluster [%s]: [%v]", vcdCluster.Name, err)
	}
	topologyWorkers := make([]vcdtypes.Workers, len(mdList.Items))
	for _, md := range mdList.Items {
		vcdMachineTemplate, err := getVCDMachineTemplateFromMachineDeployment(ctx, r.Client, md)
		if err != nil {
			return nil, fmt.Errorf("error getting the VCDMachineTemplate object from MachineDeployment [%s] for cluster [%s]: [%v]", md.Name, cluster.Name, err)
		}
		topologyWorker := vcdtypes.Workers{
			Count:        *md.Spec.Replicas,
			SizingClass:  vcdMachineTemplate.Spec.Template.Spec.ComputePolicy,
			TemplateName: vcdMachineTemplate.Spec.Template.Spec.Template,
		}
		topologyWorkers = append(topologyWorkers, topologyWorker)
	}
	rde := &swagger.DefinedEntity{
		EntityType: CAPVCDEntityTypeID,
		Name:       vcdCluster.Name,
	}
	capvcdEntity := vcdtypes.CAPVCDEntity{
		Kind:       CAPVCDClusterKind,
		ApiVersion: CAPVCDClusterEntityApiVersion,
		Metadata: vcdtypes.Metadata{
			Name: vcdCluster.Name,
			Org:  org,
			Vdc:  vdc,
			Site: vcdCluster.Spec.Site,
		},
		Spec: vcdtypes.ClusterSpec{
			Settings: vcdtypes.Settings{
				OvdcNetwork: ovdcNetwork,
				Network: vcdtypes.Network{
					Cni: vcdtypes.Cni{
						Name: CAPVCDClusterCniName,
					},
					Pods: vcdtypes.Pods{
						CidrBlocks: cluster.Spec.ClusterNetwork.Pods.CIDRBlocks,
					},
					Services: vcdtypes.Services{
						CidrBlocks: cluster.Spec.ClusterNetwork.Services.CIDRBlocks,
					},
				},
			},
			Topology: vcdtypes.Topology{
				ControlPlane: topologyControlPlanes,
				Workers:      topologyWorkers,
			},
			Distribution: vcdtypes.Distribution{
				Version: kubernetesVersion,
			},
		},
		Status: vcdtypes.Status{
			Phase: ClusterApiStatusPhaseNotReady,
			// TODO: Discuss with sahithi if "kubernetes" needs to be removed from the RDE.
			Kubernetes: kubernetesVersion,
			CloudProperties: vcdtypes.CloudProperties{
				Site: vcdCluster.Spec.Site,
				Org:  org,
				Vdc:  vdc,
			},
			ClusterAPIStatus: vcdtypes.ClusterApiStatus{
				Phase:        "",
				ApiEndpoints: []vcdtypes.ApiEndpoints{},
			},
			NodeStatus:          make(map[string]string),
			IsManagementCluster: false,
			CapvcdVersion:       "0.5.0", // TODO: Discuss with Arun on how to get the CAPVCD version.
			ParentUID:           r.VcdClient.ManagementClusterRDEId,
			Csi: vcdtypes.VersionedAddon{
				Name:    VcdCsiName,
				Version: r.VcdClient.CsiVersion, // TODO: get CPI, CNI, CSI versions from the CLusterResourceSet objects
			},
			Cpi: vcdtypes.VersionedAddon{
				Name:    VcdCpiName,
				Version: r.VcdClient.CpiVersion,
			},
			Cni: vcdtypes.VersionedAddon{
				Name:    CAPVCDClusterCniName,
				Version: r.VcdClient.CniVersion,
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

func (r *VCDClusterReconciler) constructAndCreateRDEFromCluster(ctx context.Context, workloadVCDClient *vcdclient.Client, cluster *clusterv1.Cluster, vcdCluster *infrav1.VCDCluster) (string, error) {
	log := ctrl.LoggerFrom(ctx)

	rde, err := r.constructCapvcdRDE(ctx, cluster, vcdCluster)
	if err != nil {
		return "", fmt.Errorf("error occurred while constructing RDE payload for the cluster [%s]: [%v]", vcdCluster.Name, err)
	}
	resp, err := workloadVCDClient.ApiClient.DefinedEntityApi.CreateDefinedEntity(ctx, *rde,
		rde.EntityType, nil)
	if err != nil {
		return "", fmt.Errorf("error occurred during RDE creation for the cluster [%s]: [%v]", vcdCluster.Name, err)
	}
	if resp.StatusCode != http.StatusAccepted {
		return "", fmt.Errorf("error occurred during RDE creation for the cluster [%s]", vcdCluster.Name)
	}
	taskURL := resp.Header.Get(VCDLocationHeader)
	task := govcd.NewTask(&workloadVCDClient.VcdClient.Client)
	task.Task.HREF = taskURL
	err = task.Refresh()
	if err != nil {
		return "", fmt.Errorf("error occurred during RDE creation for the cluster [%s]; error refreshing task: [%s]", vcdCluster.Name, task.Task.HREF)
	}
	rdeID := task.Task.Owner.ID
	log.Info("Created defined entity for cluster", "RDEId", rdeID)
	return rdeID, nil
}

func (r *VCDClusterReconciler) reconcileRDE(ctx context.Context, cluster *clusterv1.Cluster, vcdCluster *infrav1.VCDCluster, workloadVCDClient *vcdclient.Client) error {
	log := ctrl.LoggerFrom(ctx)

	updatePatch := make(map[string]interface{})
	_, capvcdEntity, err := workloadVCDClient.GetCAPVCDEntity(ctx, vcdCluster.Status.RDEId)
	if err != nil {
		return fmt.Errorf("failed to get RDE with ID [%s] for cluster [%s]: [%v]", vcdCluster.Status.RDEId, vcdCluster.Name, err)
	}
	// TODO(VCDA-3107): Should we be updating org and vdc information here.
	org := vcdCluster.Spec.Org
	if org != capvcdEntity.Metadata.Org {
		updatePatch["Metadata.Org"] = org
	}

	vdc := vcdCluster.Spec.Ovdc
	if vdc != capvcdEntity.Metadata.Vdc {
		updatePatch["Metadata.Vdc"] = vdc
	}

	if capvcdEntity.Metadata.Site != vcdCluster.Spec.Site {
		updatePatch["Metadata.Site"] = vcdCluster.Spec.Site
	}

	networkName := vcdCluster.Spec.OvdcNetwork
	if networkName != capvcdEntity.Spec.Settings.OvdcNetwork {
		updatePatch["Spec.Settings.OvdcNetwork"] = networkName
	}

	kcpList, err := getAllKubeadmControlPlaneForCluster(ctx, r.Client, *cluster)
	if err != nil {
		return fmt.Errorf("error getting all KubeadmControlPlane objects for cluster [%s]: [%v]", vcdCluster.Name, err)
	}
	topologyControlPlanes := make([]vcdtypes.ControlPlane, len(kcpList.Items))
	kubernetesVersion := ""
	for idx, kcp := range kcpList.Items {
		vcdMachineTemplate, err := getVCDMachineTemplateFromKCP(ctx, r.Client, kcp)
		if err != nil {
			return fmt.Errorf("error getting VCDMachineTemplate from KCP [%s] for cluster [%s]: [%v]", kcp.Name, cluster.Name, err)
		}
		topologyControlPlane := vcdtypes.ControlPlane{
			Count:        *kcp.Spec.Replicas,
			SizingClass:  vcdMachineTemplate.Spec.Template.Spec.ComputePolicy,
			TemplateName: vcdMachineTemplate.Spec.Template.Spec.Template,
		}
		topologyControlPlanes[idx] = topologyControlPlane
		kubernetesVersion = kcp.Spec.Version
	}

	mdList, err := getAllMachineDeploymentsForCluster(ctx, r.Client, *cluster)
	if err != nil {
		return fmt.Errorf("error getting getting all MachineDeployment for cluster [%s]: [%v]", vcdCluster.Name, err)
	}
	topologyWorkers := make([]vcdtypes.Workers, len(mdList.Items))
	for idx, md := range mdList.Items {
		vcdMachineTemplate, err := getVCDMachineTemplateFromMachineDeployment(ctx, r.Client, md)
		if err != nil {
			return fmt.Errorf("error getting VCDMachineTemplate from MachineDeployment [%s] for cluster [%s]: [%v]", md.Name, cluster.Name, err)
		}
		topologyWorker := vcdtypes.Workers{
			Count:        *md.Spec.Replicas,
			SizingClass:  vcdMachineTemplate.Spec.Template.Spec.ComputePolicy,
			TemplateName: vcdMachineTemplate.Spec.Template.Spec.Template,
		}
		topologyWorkers[idx] = topologyWorker
	}

	// The following code only updates the spec portion of the RDE
	if !reflect.DeepEqual(capvcdEntity.Spec.Topology.ControlPlane, topologyControlPlanes) {
		updatePatch["Spec.Topology.ControlPlane"] = topologyControlPlanes
	}
	if !reflect.DeepEqual(capvcdEntity.Spec.Topology.Workers, topologyWorkers) {
		updatePatch["Spec.Topology.Workers"] = topologyWorkers
	}
	capiYaml, err := getCapiYaml(ctx, r.Client, *cluster, *vcdCluster)
	if err != nil {
		log.Error(err, "Error occurred during RDE reconciliation; failed to construct capi yaml using kubernetes resources for the cluster")
	}

	if err == nil && capvcdEntity.Spec.CapiYaml != capiYaml {
		updatePatch["Spec.CapiYaml"] = capiYaml
	}

	// Updating status portion of the RDE in the following code
	// TODO: Delete "kubernetes" string in RDE. Discuss with Sahithi
	if capvcdEntity.Status.Kubernetes != kubernetesVersion {
		updatePatch["Status.Kubernetes"] = kubernetesVersion
	}

	if capvcdEntity.Spec.Distribution.Version != kubernetesVersion {
		updatePatch["Spec.Distribution.Version"] = kubernetesVersion
	}

	if capvcdEntity.Status.Uid != vcdCluster.Status.RDEId {
		updatePatch["Status.Uid"] = vcdCluster.Status.RDEId
	}

	if capvcdEntity.Status.Phase != cluster.Status.Phase {
		updatePatch["Status.Phase"] = cluster.Status.Phase
	}
	clusterApiStatusPhase := ClusterApiStatusPhaseNotReady
	if cluster.Status.ControlPlaneReady {
		clusterApiStatusPhase = ClusterApiStatusPhaseReady
	}
	clusterApiStatus := vcdtypes.ClusterApiStatus{
		Phase: clusterApiStatusPhase,
		ApiEndpoints: []vcdtypes.ApiEndpoints{
			{
				Host: vcdCluster.Spec.ControlPlaneEndpoint.Host,
				Port: 6443,
			},
		},
	}
	if !reflect.DeepEqual(clusterApiStatus, capvcdEntity.Status.ClusterAPIStatus) {
		updatePatch["Status.ClusterAPIStatus"] = clusterApiStatus
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
	if !reflect.DeepEqual(updatedNodeStatus, capvcdEntity.Status.NodeStatus) {
		updatePatch["Status.NodeStatus"] = updatedNodeStatus
	}

	updatedRDE, err := workloadVCDClient.PatchRDE(ctx, updatePatch, vcdCluster.Status.RDEId)
	if err != nil {
		return fmt.Errorf("failed to update defined entity with ID [%s] for cluster [%s]: [%v]", vcdCluster.Status.RDEId, vcdCluster.Name, err)
	}

	if updatedRDE.State != RDEStatusResolved {
		// try to resolve the defined entity
		entityState, resp, err := workloadVCDClient.ApiClient.DefinedEntityApi.ResolveDefinedEntity(ctx, updatedRDE.Id)
		if err != nil {
			return fmt.Errorf("failed to resolve defined entity with ID [%s] for cluster [%s]", vcdCluster.Status.RDEId, vcdCluster.Name)
		}
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("error while resolving defined entity with ID [%s] for cluster [%s] with message: [%s]", vcdCluster.Status.RDEId, vcdCluster.Name, entityState.Message)
		}
		if entityState.State != RDEStatusResolved {
			return fmt.Errorf("defined entity resolution failed for RDE with ID [%s] for cluster [%s] with message: [%s]", vcdCluster.Status.RDEId, vcdCluster.Name, entityState.Message)
		}
		log.Info("Resolved defined entity of cluster", "RDEId", vcdCluster.Status.RDEId)
	}
	return nil
}

func (r *VCDClusterReconciler) reconcileNormal(ctx context.Context, cluster *clusterv1.Cluster,
	vcdCluster *infrav1.VCDCluster) (ctrl.Result, error) {

	log := ctrl.LoggerFrom(ctx)

	workloadVCDClient, err := vcdclient.NewVCDClientFromSecrets(vcdCluster.Spec.Site, vcdCluster.Spec.Org,
		vcdCluster.Spec.Ovdc, vcdCluster.Name, vcdCluster.Spec.OvdcNetwork, r.VcdClient.IPAMSubnet,
		r.VcdClient.VcdAuthConfig.UserOrg, vcdCluster.Spec.UserCredentialsContext.Username,
		vcdCluster.Spec.UserCredentialsContext.Password, vcdCluster.Spec.UserCredentialsContext.RefreshToken,
		true, vcdCluster.Status.RDEId, r.VcdClient.OneArm, 0, 0, r.VcdClient.TCPPort,
		true, "", r.VcdClient.CsiVersion, r.VcdClient.CpiVersion, r.VcdClient.CniVersion)
	if err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "Error creating VCD client to reconcile Cluster [%s] infrastructure", vcdCluster.Name)
	}

	gateway := &vcdclient.GatewayManager{
		NetworkName:        workloadVCDClient.NetworkName,
		Client:             workloadVCDClient,
		GatewayRef:         workloadVCDClient.GatewayRef,
		NetworkBackingType: workloadVCDClient.NetworkBackingType,
	}

	// NOTE: Since RDE is used just as a book-keeping mechanism, we should not fail reconciliation if RDE operations fail
	// create RDE for cluster
	if vcdCluster.Status.RDEId == "" {
		nameFilter := &swagger.DefinedEntityApiGetDefinedEntitiesByEntityTypeOpts{
			Filter: optional.NewString(fmt.Sprintf("name==%s", vcdCluster.Name)),
		}
		definedEntities, resp, err := workloadVCDClient.ApiClient.DefinedEntityApi.GetDefinedEntitiesByEntityType(ctx,
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
					vcdCluster.Status.RDEId = rdeID
				}
			} else {
				log.Info("RDE for the cluster is already present; skipping RDE creation", "RDEId",
					definedEntities.Values[0].Id)
				vcdCluster.Status.RDEId = definedEntities.Values[0].Id
			}
		}
	}

	// If there is no specified RDE ID, self-generate one and use. We need UUIDs to single-instance
	// cleanly in the Virtual Services etc.
	if vcdCluster.Status.RDEId == "" {
		rdeID := NoRdePrefix + uuid.New().String()
		log.Info("Error retrieving RDEId. Hence using a self-generated UUID", "UUID", rdeID)
		vcdCluster.Status.RDEId = rdeID
	}

	// TODO: What should be the prefix if cluster creation fails here?
	// create load balancer for the cluster. Only one-arm load balancer is fully tested.
	virtualServiceNamePrefix := vcdCluster.Name + "-" + vcdCluster.Status.RDEId
	lbPoolNamePrefix := vcdCluster.Name + "-" + vcdCluster.Status.RDEId

	controlPlaneNodeIP, err := gateway.GetLoadBalancer(ctx, fmt.Sprintf("%s-tcp", virtualServiceNamePrefix))
	//TODO: Sahithi: Check if error is really because of missing virtual service.
	// In any other error cases, force create the new load balancer with the original control plane endpoint
	// (if already present). Do not overwrite the existing control plane endpoint with a new endpoint.

	if err != nil {
		log.Info("Creating load balancer for the cluster")
		controlPlaneNodeIP, err = gateway.CreateL4LoadBalancer(ctx, virtualServiceNamePrefix, lbPoolNamePrefix,
			[]string{}, 6443)
		if err != nil {
			return ctrl.Result{}, errors.Wrapf(err, "Error creating create load balancer [%s] for the cluster [%s]: [%v]",
				virtualServiceNamePrefix, vcdCluster.Name, err)
		}
	}
	vcdCluster.Spec.ControlPlaneEndpoint = infrav1.APIEndpoint{
		Host: controlPlaneNodeIP,
		Port: 6443,
	}
	log.Info(fmt.Sprintf("Control plane endpoint for the cluster is [%s]", controlPlaneNodeIP))

	if !strings.HasPrefix(vcdCluster.Status.RDEId, NoRdePrefix) {
		_, resp, _, err := workloadVCDClient.ApiClient.DefinedEntityApi.GetDefinedEntity(ctx, vcdCluster.Status.RDEId)
		if err != nil {
			log.Error(err, "Error retrieving RDE for the cluster from VCD", "RDEId", vcdCluster.Status.RDEId)
		}
		if resp == nil {
			log.Error(nil, "Error retrieving RDE for the cluster from VCD; obtained an empty response", "RDEId", vcdCluster.Status.RDEId)
		} else if resp.StatusCode != http.StatusOK {
			log.Error(nil, "Error retrieving RDE for the cluster from VCD", "RDEId", vcdCluster.Status.RDEId)
		}
		if err == nil && resp != nil && resp.StatusCode == http.StatusOK {
			if err = r.reconcileRDE(ctx, cluster, vcdCluster, workloadVCDClient); err != nil {
				log.Error(err, "Error occurred during RDE reconciliation", "RDEId", vcdCluster.Status.RDEId)
			}
		}
	}

	// Update the vcdCluster resource with updated information
	// TODO Check if updating ovdcNetwork, Org and Ovdc should be done somewhere earlier in the code.
	vcdCluster.ClusterName = vcdCluster.Name

	vcdCluster.Status.Ready = true
	conditions.MarkTrue(vcdCluster, infrav1.LoadBalancerAvailableCondition)

	return ctrl.Result{}, nil
}

func (r *VCDClusterReconciler) reconcileDelete(ctx context.Context,
	vcdCluster *infrav1.VCDCluster) (ctrl.Result, error) {

	log := ctrl.LoggerFrom(ctx)

	patchHelper, err := patch.NewHelper(vcdCluster, r.Client)
	if err != nil {
		return ctrl.Result{}, err
	}
	conditions.MarkFalse(vcdCluster, infrav1.LoadBalancerAvailableCondition, clusterv1.DeletingReason,
		clusterv1.ConditionSeverityInfo, "")
	if err := patchVCDCluster(ctx, patchHelper, vcdCluster); err != nil {
		return ctrl.Result{}, errors.Wrap(err, "Error occurred during cluster deletion; failed to patch VCDCluster")
	}

	workloadVCDClient, err := vcdclient.NewVCDClientFromSecrets(vcdCluster.Spec.Site, vcdCluster.Spec.Org,
		vcdCluster.Spec.Ovdc, vcdCluster.Name, vcdCluster.Spec.OvdcNetwork, r.VcdClient.IPAMSubnet,
		r.VcdClient.VcdAuthConfig.UserOrg, vcdCluster.Spec.UserCredentialsContext.Username,
		vcdCluster.Spec.UserCredentialsContext.Password, vcdCluster.Spec.UserCredentialsContext.RefreshToken,
		true, vcdCluster.Status.RDEId, r.VcdClient.OneArm, 0, 0, r.VcdClient.TCPPort,
		true, "", r.VcdClient.CsiVersion, r.VcdClient.CpiVersion, r.VcdClient.CniVersion)
	if err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "Error occurred during cluster deletion; unable to create client for the workload cluster [%s]", vcdCluster.Name)
	}

	gateway := &vcdclient.GatewayManager{
		NetworkName:        workloadVCDClient.NetworkName,
		Client:             workloadVCDClient,
		GatewayRef:         workloadVCDClient.GatewayRef,
		NetworkBackingType: workloadVCDClient.NetworkBackingType,
	}

	// Delete the load balancer components
	virtualServiceNamePrefix := vcdCluster.Name + "-" + vcdCluster.Status.RDEId
	lbPoolNamePrefix := vcdCluster.Name + "-" + vcdCluster.Status.RDEId
	err = gateway.DeleteLoadBalancer(ctx, virtualServiceNamePrefix, lbPoolNamePrefix)
	if err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "Error occurred during cluster [%s] deletion; unable to delete the load balancer [%s]: [%v]", vcdCluster.Name, virtualServiceNamePrefix, err)
	}
	log.Info("Deleted the load balancer components (virtual service, lb pool, dnat rule) of the cluster",
		"virtual service", virtualServiceNamePrefix, "lb pool", lbPoolNamePrefix)

	vdcManager := vcdclient.VdcManager{
		VdcName: workloadVCDClient.ClusterOVDCName,
		OrgName: workloadVCDClient.ClusterOrgName,
		Client:  workloadVCDClient,
		Vdc:     workloadVCDClient.Vdc,
	}

	// Delete vApp
	vApp, err := workloadVCDClient.Vdc.GetVAppByName(vcdCluster.Name, true)
	if err != nil {
		log.Error(err, fmt.Sprintf("Error occurred during cluster deletion; vApp [%s] not found", vcdCluster.Name))
	}
	if vApp != nil {
		if vApp.VApp.Children != nil {
			return ctrl.Result{}, errors.Errorf("Error occurred during cluster deletion; %d VMs detected in the vApp %s", len(vApp.VApp.Children.VM), vcdCluster.Name)
		} else {
			log.Info("Deleting vApp of the cluster", "vAppName", vcdCluster.Name)
			err = vdcManager.DeleteVApp(vcdCluster.Name)
			if err != nil {
				return ctrl.Result{}, errors.Wrapf(err, "Error occurred during cluster deletion; failed to delete vApp [%s]", vcdCluster.Name)
			}
			log.Info("Successfully deleted vApp of the cluster", "vAppName", vcdCluster.Name)
		}
	}

	// TODO: If RDE deletion fails, should we throw an error during reconciliation?
	// Delete RDE
	if vcdCluster.Status.RDEId != "" && !strings.HasPrefix(vcdCluster.Status.RDEId, NoRdePrefix) {
		definedEntities, resp, err := workloadVCDClient.ApiClient.DefinedEntityApi.GetDefinedEntitiesByEntityType(ctx,
			CAPVCDTypeVendor, CAPVCDTypeNss, CAPVCDTypeVersion, 1, 25,
			&swagger.DefinedEntityApiGetDefinedEntitiesByEntityTypeOpts{
				Filter: optional.NewString(fmt.Sprintf("id==%s", vcdCluster.Status.RDEId)),
			})
		if err != nil {
			return ctrl.Result{}, errors.Wrapf(err, "Error occurred during cluster deletion; failed to fetch defined entities by entity type [%s] and ID [%s] for cluster [%s]", CAPVCDEntityTypeID, vcdCluster.Status.RDEId, vcdCluster.Name)
		}
		if resp.StatusCode != http.StatusOK {
			return ctrl.Result{}, errors.Errorf("Error occurred during cluster deletion; error while fetching defined entities by entity type [%s] and ID [%s] for cluster [%s]", CAPVCDEntityTypeID, vcdCluster.Status.RDEId, vcdCluster.Name)
		}
		if len(definedEntities.Values) > 0 {
			// resolve defined entity before deleting
			entityState, resp, err := workloadVCDClient.ApiClient.DefinedEntityApi.ResolveDefinedEntity(ctx,
				vcdCluster.Status.RDEId)
			if err != nil {
				return ctrl.Result{}, errors.Wrapf(err, "Error occurred during cluster deletion; error occurred while resolving defined entity [%s] with ID [%s] before deleting", vcdCluster.Name, vcdCluster.Status.RDEId)
			}
			if resp.StatusCode != http.StatusOK {
				log.Error(nil, "Error occurred during cluster deletion; failed to resolve RDE with ID [%s] for cluster [%s]: [%s]", vcdCluster.Status.RDEId, vcdCluster.Name, entityState.Message)
			}
			resp, err = workloadVCDClient.ApiClient.DefinedEntityApi.DeleteDefinedEntity(ctx,
				vcdCluster.Status.RDEId, nil)
			if err != nil {
				return ctrl.Result{}, errors.Wrapf(err, "Error occurred during cluster deletion; failed to execute delete defined entity call for RDE with ID [%s]", vcdCluster.Status.RDEId)
			}
			if resp.StatusCode != http.StatusNoContent {
				return ctrl.Result{}, errors.Errorf("Error occurred during cluster deletion; error deleting defined entity associated with the cluster. RDE id: [%s]", vcdCluster.Status.RDEId)
			}
			log.Info("Successfully deleted the defined entity for cluster")
		} else {
			log.Info("Attempting to delete RDE, but corresponding defined entity is not found", "RDEId", vcdCluster.Status.RDEId)
		}
	}
	log.Info("Successfully deleted all the infra resources of the cluster")
	// Cluster is deleted so remove the finalizer.
	controllerutil.RemoveFinalizer(vcdCluster, infrav1.ClusterFinalizer)

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *VCDClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&infrav1.VCDCluster{}).
		Complete(r)
}
