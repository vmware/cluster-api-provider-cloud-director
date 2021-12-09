/*
   Copyright 2021 VMware, Inc.
   SPDX-License-Identifier: Apache-2.0
*/

package controllers

import (
	"context"
	"fmt"
	"github.com/antihax/optional"
	"github.com/pkg/errors"
	infrav1 "github.com/vmware/cluster-api-provider-cloud-director/api/v1alpha4"
	vcdutil "github.com/vmware/cluster-api-provider-cloud-director/pkg/util"
	"github.com/vmware/cluster-api-provider-cloud-director/pkg/vcdclient"
	swagger "github.com/vmware/cluster-api-provider-cloud-director/pkg/vcdswaggerclient"
	"github.com/vmware/cluster-api-provider-cloud-director/pkg/vcdtypes"
	"github.com/vmware/go-vcloud-director/v2/govcd"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	"net/http"
	"reflect"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha4"
	kcpv1 "sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1alpha4"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/cluster-api/util/patch"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	CAPVCDTypeVendor  = "vmware"
	CAPVCDTypeNss     = "capvcd"
	CAPVCDTypeVersion = "1.0.0"

	CAPVCDClusterKind             = "CAPVCDCluster"
	CAPVCDClusterEntityApiVersion = "capvcd.vmware.com/v1.0"
	CAPVCDClusterCniName          = "antrea" // TODO: Get the correct value for CNI name

	RDEStatusResolved = "RESOLVED"
	VCDLocationHeader = "Location"

	ClusterApiStatusPhaseReady    = "Ready"
	ClusterApiStatusPhaseNotReady = "Not Ready"
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

func (r *VCDClusterReconciler) getAllMachineDeploymentsForCluster(ctx context.Context, c clusterv1.Cluster) (*clusterv1.MachineDeploymentList, error) {
	mdListLabels := map[string]string{clusterv1.ClusterLabelName: c.Name}
	mdList := &clusterv1.MachineDeploymentList{}
	if err := r.Client.List(ctx, mdList, client.InNamespace(c.Namespace), client.MatchingLabels(mdListLabels)); err != nil {
		return nil, errors.Wrapf(err, "error getting machine deployments for the cluster [%s]", c.Name)
	}
	return mdList, nil
}

func (r *VCDClusterReconciler) getAllKubeadmControlPlaneForCluster(ctx context.Context, c clusterv1.Cluster) (*kcpv1.KubeadmControlPlaneList, error) {
	kcpListLabels := map[string]string{clusterv1.ClusterLabelName: c.Name}
	kcpList := &kcpv1.KubeadmControlPlaneList{}

	if err := r.Client.List(ctx, kcpList, client.InNamespace(c.Namespace), client.MatchingLabels(kcpListLabels)); err != nil {
		return nil, errors.Wrapf(err, "error getting all kubeadm control planes for the cluster [%s]", c.Name)
	}
	return kcpList, nil
}

func (r *VCDClusterReconciler) getVCDMachineTemplateFromKCP(ctx context.Context, kcp kcpv1.KubeadmControlPlane) (*infrav1.VCDMachineTemplate, error) {
	vcdMachineTemplateRef := kcp.Spec.MachineTemplate.InfrastructureRef
	vcdMachineTemplate := &infrav1.VCDMachineTemplate{}
	vcdMachineTemplateKey := types.NamespacedName{
		Namespace: vcdMachineTemplateRef.Namespace,
		Name:      vcdMachineTemplateRef.Name,
	}
	if err := r.Client.Get(ctx, vcdMachineTemplateKey, vcdMachineTemplate); err != nil {
		return nil, fmt.Errorf("failed to get VCDMachineTemplate by name [%s] from KCP [%s]: [%v]", vcdMachineTemplateRef.Name, kcp.Name, err)
	}

	return vcdMachineTemplate, nil
}

func (r *VCDClusterReconciler) getVCDMachineTemplateFromMachineDeployment(ctx context.Context, md clusterv1.MachineDeployment) (*infrav1.VCDMachineTemplate, error) {
	vcdMachineTemplateRef := md.Spec.Template.Spec.InfrastructureRef
	vcdMachineTemplate := &infrav1.VCDMachineTemplate{}
	vcdMachineTemplateKey := client.ObjectKey{
		Namespace: vcdMachineTemplateRef.Namespace,
		Name:      vcdMachineTemplateRef.Name,
	}
	if err := r.Client.Get(ctx, vcdMachineTemplateKey, vcdMachineTemplate); err != nil {
		return nil, fmt.Errorf("failed to get VCDMachineTemplate by name [%s] from machine deployment [%s]: [%v]", vcdMachineTemplateRef.Name, md.Name, err)
	}

	return vcdMachineTemplate, nil
}

func (r *VCDClusterReconciler) getMachineListFromCluster(ctx context.Context, cluster clusterv1.Cluster) (*clusterv1.MachineList, error) {
	machineListLabels := map[string]string{clusterv1.ClusterLabelName: cluster.Name}
	machineList := &clusterv1.MachineList{}
	if err := r.Client.List(ctx, machineList, client.InNamespace(cluster.Namespace), client.MatchingLabels(machineListLabels)); err != nil {
		return nil, errors.Wrapf(err, "error getting machine list for the cluster [%s]", cluster.Name)
	}
	return machineList, nil
}

//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=vcdclusters,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=vcdclusters/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=vcdclusters/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=secrets;,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=events,verbs=get;list;watch;create;update;patch
func (r *VCDClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (_ ctrl.Result, rerr error) {
	// your logic here
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
	log = log.WithValues("cluster", cluster.Name)

	patchHelper, err := patch.NewHelper(vcdCluster, r.Client)
	if err != nil {
		return ctrl.Result{}, err
	}
	defer func() {
		if err := patchVCDCluster(ctx, patchHelper, vcdCluster); err != nil {
			log.Error(err, "failed to patch VCDCluster")
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

	kcpList, err := r.getAllKubeadmControlPlaneForCluster(ctx, *cluster)
	if err != nil {
		return nil, fmt.Errorf("error getting KubeadmControlPlane objects for cluster [%s]: [%v]", vcdCluster.Name, err)
	}
	topologyControlPlanes := make([]vcdtypes.ControlPlane, len(kcpList.Items))
	kubernetesVersion := ""
	for _, kcp := range kcpList.Items {
		vcdMachineTemplate, err := r.getVCDMachineTemplateFromKCP(ctx, kcp)
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

	mdList, err := r.getAllMachineDeploymentsForCluster(ctx, *cluster)
	if err != nil {
		return nil, fmt.Errorf("error getting the MachineDeployments for cluster [%s]: [%v]", vcdCluster.Name, err)
	}
	topologyWorkers := make([]vcdtypes.Workers, len(mdList.Items))
	for _, md := range mdList.Items {
		vcdMachineTemplate, err := r.getVCDMachineTemplateFromMachineDeployment(ctx, md)
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
				SshKey:      "", // TODO: Should add ssh key as part of vcdCluster representation
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
				Site:   vcdCluster.Spec.Site,
				Org:    org,
				Vdc:    vdc,
				SshKey: "", // TODO: Should add ssh key as part of vcdCluster representation
			},
			ClusterAPIStatus: vcdtypes.ClusterApiStatus{
				Phase:        "",
				ApiEndpoints: []vcdtypes.ApiEndpoints{},
			},
			NodeStatus:          make(map[string]string),
			IsManagementCluster: false,
			CapvcdVersion:       "0.5.0", // TODO: Discuss with Arun on how to get the CAPVCD version.
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
	rde, err := r.constructCapvcdRDE(ctx, cluster, vcdCluster)
	if err != nil {
		return "", fmt.Errorf("unable to create defined entity for cluster [%s]: [%v]", vcdCluster.Name, err)
	}
	resp, err := workloadVCDClient.ApiClient.DefinedEntityApi.CreateDefinedEntity(ctx, *rde, rde.EntityType, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create defined entity for cluster [%s]", vcdCluster.Name)
	}
	if resp.StatusCode != http.StatusAccepted {
		return "", fmt.Errorf("create defined entity call failed for cluster [%s]", vcdCluster.Name)
	}
	taskURL := resp.Header.Get(VCDLocationHeader)
	task := govcd.NewTask(&workloadVCDClient.VcdClient.Client)
	task.Task.HREF = taskURL
	err = task.Refresh()
	if err != nil {
		return "", fmt.Errorf("error refreshing task: [%s]", task.Task.HREF)
	}
	klog.Infof("created defined entity for cluster [%s]. RDE ID: [%s]", vcdCluster.Name,
		vcdCluster.Status.RDEId)
	return task.Task.Owner.ID, nil
}

func (r *VCDClusterReconciler) reconcileRDE(ctx context.Context, cluster *clusterv1.Cluster, vcdCluster *infrav1.VCDCluster, workloadVCDClient *vcdclient.Client) error {
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

	kcpList, err := r.getAllKubeadmControlPlaneForCluster(ctx, *cluster)
	if err != nil {
		return fmt.Errorf("error getting all KubeadmControlPlane objects for cluster [%s]: [%v]", vcdCluster.Name, err)
	}
	topologyControlPlanes := make([]vcdtypes.ControlPlane, len(kcpList.Items))
	kubernetesVersion := ""
	for idx, kcp := range kcpList.Items {
		vcdMachineTemplate, err := r.getVCDMachineTemplateFromKCP(ctx, kcp)
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

	mdList, err := r.getAllMachineDeploymentsForCluster(ctx, *cluster)
	if err != nil {
		return fmt.Errorf("error getting getting all MachineDeployment for cluster [%s]: [%v]", vcdCluster.Name, err)
	}
	topologyWorkers := make([]vcdtypes.Workers, len(mdList.Items))
	for idx, md := range mdList.Items {
		vcdMachineTemplate, err := r.getVCDMachineTemplateFromMachineDeployment(ctx, md)
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
		klog.Errorf("______________DEBUG: error occurred: [%v]", err)
	}
	klog.Infof("-------------------DEBUG: capiYaml: \n[%s]", capiYaml)

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
	machineList, err := r.getMachineListFromCluster(ctx, *cluster)
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
		return fmt.Errorf("failed to update defined entity with ID [%s]: [%v]", vcdCluster.Status.RDEId, err)
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
		klog.Infof("resolved defined entity with ID [%s] for cluster [%s]", vcdCluster.Status.RDEId, vcdCluster.Name)
	}
	return nil
}

func (r *VCDClusterReconciler) reconcileNormal(ctx context.Context, cluster *clusterv1.Cluster,
	vcdCluster *infrav1.VCDCluster) (ctrl.Result, error) {

	_ = ctrl.LoggerFrom(ctx)

	workloadVCDClient, err := vcdclient.NewVCDClientFromSecrets(vcdCluster.Spec.Site, vcdCluster.Spec.Org,
		vcdCluster.Spec.Ovdc, vcdCluster.Spec.OvdcNetwork, r.VcdClient.IPAMSubnet,
		vcdCluster.Spec.UserCredentialsContext.Username, vcdCluster.Spec.UserCredentialsContext.Password, true,
		vcdCluster.Status.RDEId, r.VcdClient.OneArm, 0, 0, r.VcdClient.TCPPort, true, "")
	if err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "unable to create client for workload cluster")
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
			klog.Errorf("failed to get entities by entity type [%s] with name filter [name==%s]",
				CAPVCDEntityTypeID, vcdCluster.Name)
		}
		if resp == nil {
			klog.Errorf("obtained an empty response for get defined entity call for cluster with name [%s]", vcdCluster.Name)
		} else if resp.StatusCode != http.StatusOK {
			klog.Errorf("error while getting entities by entity type [%s] with name filter [name==%s]", CAPVCDEntityTypeID, vcdCluster.Name)
		}
		if err == nil && resp != nil && resp.StatusCode == http.StatusOK {
			if len(definedEntities.Values) == 0 {
				rdeID, err := r.constructAndCreateRDEFromCluster(ctx, workloadVCDClient, cluster, vcdCluster)
				if err != nil {
					klog.Errorf("failed to create RDE from Cluster and VCDCluster objects for cluster [%s]: [%v]", vcdCluster.Name, err)
				} else {
					vcdCluster.Status.RDEId = rdeID
				}
			} else {
				klog.Infof("defined entity for cluster [%s] already present. RDE ID: [%s]", vcdCluster.Name,
					definedEntities.Values[0].Id)
				vcdCluster.Status.RDEId = definedEntities.Values[0].Id
			}
		}
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
		controlPlaneNodeIP, err = gateway.CreateL4LoadBalancer(ctx, virtualServiceNamePrefix, lbPoolNamePrefix,
			[]string{}, 6443)
		if err != nil {
			return ctrl.Result{}, errors.Wrapf(err, "unable to create load balancer [%s]: [%v]",
				virtualServiceNamePrefix, err)
		}
	}
	vcdCluster.Spec.ControlPlaneEndpoint = infrav1.APIEndpoint{
		Host: controlPlaneNodeIP,
		Port: 6443,
	}

	_, resp, _, err := workloadVCDClient.ApiClient.DefinedEntityApi.GetDefinedEntity(ctx, vcdCluster.Status.RDEId)
	if err != nil {
		klog.Errorf("failed to get defined entity with ID [%s] for cluster [%s]: [%s]", vcdCluster.Status.RDEId, vcdCluster.Name, err)
	}
	if resp == nil {
		klog.Errorf("obtained an empty response for get defined entity call for cluster [%s] and RDE [%s]", vcdCluster.Name, vcdCluster.Status.RDEId)
	} else if resp.StatusCode != http.StatusOK {
		klog.Errorf("error getting defined entity with ID [%s] for cluster [%s]", vcdCluster.Status.RDEId, vcdCluster.Name)
	}
	if err == nil && resp != nil && resp.StatusCode == http.StatusOK {
		if err = r.reconcileRDE(ctx, cluster, vcdCluster, workloadVCDClient); err != nil {
			klog.Errorf("failed to update and resolve defined entity with ID [%s] for cluster [%s]: [%v]", vcdCluster.Status.RDEId, vcdCluster.Name, err)
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
	log = log.WithValues("cluster", vcdCluster.Name)

	patchHelper, err := patch.NewHelper(vcdCluster, r.Client)
	if err != nil {
		return ctrl.Result{}, err
	}
	conditions.MarkFalse(vcdCluster, infrav1.LoadBalancerAvailableCondition, clusterv1.DeletingReason,
		clusterv1.ConditionSeverityInfo, "")
	if err := patchVCDCluster(ctx, patchHelper, vcdCluster); err != nil {
		return ctrl.Result{}, errors.Wrap(err, "failed to patch VCDCluster")
	}

	workloadVCDClient, err := vcdclient.NewVCDClientFromSecrets(vcdCluster.Spec.Site, vcdCluster.Spec.Org,
		vcdCluster.Spec.Ovdc, vcdCluster.Spec.OvdcNetwork, r.VcdClient.IPAMSubnet,
		vcdCluster.Spec.UserCredentialsContext.Username, vcdCluster.Spec.UserCredentialsContext.Password, true,
		"", r.VcdClient.OneArm, 0, 0, r.VcdClient.TCPPort, true, "")
	if err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "unable to create client for workload cluster")
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
		return ctrl.Result{}, errors.Wrapf(err, "unable to delete load balancer [%s]: [%v]", virtualServiceNamePrefix, err)
	}

	vdcManager := vcdclient.VdcManager{
		VdcName: workloadVCDClient.VcdAuthConfig.VDC,
		OrgName: workloadVCDClient.VcdAuthConfig.Org,
		Client:  workloadVCDClient,
		Vdc:     workloadVCDClient.Vdc,
	}

	// Delete vApp
	vApp, err := workloadVCDClient.Vdc.GetVAppByName(vcdCluster.Name, true)
	if err != nil {
		log.Error(err, fmt.Sprintf("vApp [%s] not found", vcdCluster.Name))
	}
	if vApp != nil {
		if vApp.VApp.Children != nil {
			return ctrl.Result{}, errors.Errorf("%d VMs detected in the vApp %s", len(vApp.VApp.Children.VM), vcdCluster.Name)
		} else {
			log.Info("deleting vApp", "vAppName", vcdCluster.Name)
			err = vdcManager.DeleteVApp(vcdCluster.Name)
			if err != nil {
				return ctrl.Result{}, errors.Wrapf(err, "failed to delete vApp [%s]", vcdCluster.Name)
			}
			log.Info("successfully deleted vApp", "vAppName", vcdCluster.Name)
		}
	}

	// TODO: If RDE deletion fails, should we throw an error during reconciliation?
	// Delete RDE
	if vcdCluster.Status.RDEId != "" {
		definedEntities, resp, err := workloadVCDClient.ApiClient.DefinedEntityApi.GetDefinedEntitiesByEntityType(ctx, CAPVCDTypeVendor, CAPVCDTypeNss, CAPVCDTypeVersion, 1, 25, &swagger.DefinedEntityApiGetDefinedEntitiesByEntityTypeOpts{
			Filter: optional.NewString(fmt.Sprintf("id==%s", vcdCluster.Status.RDEId)),
		})
		if err != nil {
			return ctrl.Result{}, errors.Wrapf(err, "failed to fetch defined entities by entity type [%s] and ID [%s] for cluster [%s]", CAPVCDEntityTypeID, vcdCluster.Status.RDEId, vcdCluster.Name)
		}
		if resp.StatusCode != http.StatusOK {
			return ctrl.Result{}, errors.Errorf("error while fetching defined entities by entity type [%s] and ID [%s] for cluster [%s]", CAPVCDEntityTypeID, vcdCluster.Status.RDEId, vcdCluster.Name)
		}
		if len(definedEntities.Values) > 0 {
			// resolve defined entity before deleting
			entityState, resp, err := workloadVCDClient.ApiClient.DefinedEntityApi.ResolveDefinedEntity(ctx, vcdCluster.Status.RDEId)
			if err != nil {
				return ctrl.Result{}, errors.Wrapf(err, "error occurred while resolving defined entity [%s] with ID [%s] before deleting", vcdCluster.Name, vcdCluster.Status.RDEId)
			}
			if resp.StatusCode != http.StatusOK {
				klog.Errorf("failed to resolve RDE with ID [%s] for cluster [%s]: [%s]", vcdCluster.Status.RDEId, vcdCluster.Name, entityState.Message)
			}
			resp, err = workloadVCDClient.ApiClient.DefinedEntityApi.DeleteDefinedEntity(ctx, vcdCluster.Status.RDEId, nil)
			if err != nil {
				return ctrl.Result{}, errors.Wrapf(err, "falied to execute delete defined entity call for RDE with ID [%s]", vcdCluster.Status.RDEId)
			}
			if resp.StatusCode != http.StatusNoContent {
				return ctrl.Result{}, errors.Errorf("error deleting defined entity associated with the cluster. RDE id: [%s]", vcdCluster.Status.RDEId)
			}
			log.Info("successfully deleted the defined entity for cluster", "clusterName", vcdCluster.Name)
		} else {
			log.Info("No defined entity found", "clusterName", vcdCluster.Name, "RDE ID", vcdCluster.Status.RDEId)
		}
	}
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
