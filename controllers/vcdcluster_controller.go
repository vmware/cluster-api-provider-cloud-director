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
	"k8s.io/klog/v2"
	"net/http"
	"reflect"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha4"
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

func (r *VCDClusterReconciler) constructCapvcdRDE(vcdCluster *infrav1.VCDCluster) (*swagger.DefinedEntity, error) {
	org := r.VcdClient.VcdAuthConfig.Org
	if vcdCluster.Spec.Org != "" && vcdCluster.Spec.Org != org {
		org = vcdCluster.Spec.Org
	}
	vdc := r.VcdClient.VcdAuthConfig.VDC
	if vcdCluster.Spec.Ovdc != "" && vcdCluster.Spec.Ovdc != vdc {
		vdc = vcdCluster.Spec.Ovdc
	}
	ovdcNetwork := r.VcdClient.NetworkName
	if vcdCluster.Spec.OvdcNetwork != "" && vcdCluster.Spec.OvdcNetwork != ovdcNetwork {
		ovdcNetwork = vcdCluster.Spec.OvdcNetwork
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
			Site: r.VcdClient.VcdAuthConfig.Host,
		},
		Spec: vcdtypes.ClusterSpec{
			Settings: vcdtypes.Settings{
				OvdcNetwork: ovdcNetwork,
				SshKey:      "", // TODO: Should add ssh key as part of vcdCluster representation
				Network: vcdtypes.Network{
					Cni: vcdtypes.Cni{
						Name: CAPVCDClusterCniName,
					},
				},
			},
			Topology: vcdtypes.Topology{
				ControlPlane: vcdtypes.ControlPlane{
					SizingClass: vcdCluster.Spec.DefaultComputePolicy, // TODO: Need to fill sizing policy from KCP object
					Count:       int32(0),                             // TODO: Fill with right value
				},
				Workers: vcdtypes.Workers{
					SizingClass: vcdCluster.Spec.DefaultComputePolicy, // TODO: Need to fill sizing class from KCP object.
					Count:       int32(0),                             // TODO: Fill with right value
				},
			},
			Distribution: vcdtypes.Distribution{
				TemplateName: "some-template-name", // TODO: Should add template name as part of vcdCluster representation
			},
		},
		Status: vcdtypes.Status{
			Phase:      "",                   // TODO: should be the Cluster object status
			Cni:        CAPVCDClusterCniName, // TODO: Should add cni as part of vcdCluster representation
			Kubernetes: vcdCluster.APIVersion,
			CloudProperties: vcdtypes.CloudProperties{
				Site: r.VcdClient.VcdAuthConfig.Host,
				Org:  org,
				Vdc:  vdc,
				Distribution: vcdtypes.Distribution{
					TemplateName: "some-template-name", // TODO: Fix with right value
				},
				SshKey: "", // TODO: Should add ssh key as part of vcdCluster representation
			},
			ClusterAPIStatus: vcdtypes.ClusterApiStatus{
				Phase:        "",
				ApiEndpoints: []vcdtypes.ApiEndpoints{},
			},
			NodeStatus: make(map[string]string),
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

func (r *VCDClusterReconciler) syncRDE(ctx context.Context, cluster *clusterv1.Cluster, vcdCluster *infrav1.VCDCluster, controlPlaneIP string) error {
	updatePatch := make(map[string]interface{})
	_, capvcdEntity, err := r.VcdClient.GetCAPVCDEntity(ctx, vcdCluster.Status.ClusterRDEId)
	if err != nil {
		return fmt.Errorf("failed to get RDE with ID [%s] for cluster [%s]: [%v]", vcdCluster.Status.ClusterRDEId, vcdCluster.Name, err)
	}

	// TODO(VCDA-3107): Should we be updating org and vdc information here.
	org := r.VcdClient.VcdAuthConfig.Org
	if vcdCluster.Spec.Org != "" {
		org = vcdCluster.Spec.Org
	}
	if org != capvcdEntity.Metadata.Org {
		updatePatch["Metadata.Org"] = org
	}

	vdc := r.VcdClient.VcdAuthConfig.VDC
	if vcdCluster.Spec.Ovdc != "" {
		vdc = vcdCluster.Spec.Ovdc
	}
	if vdc != capvcdEntity.Metadata.Vdc {
		updatePatch["Metadata.Vdc"] = vdc
	}

	if capvcdEntity.Metadata.Site != r.VcdClient.VcdAuthConfig.Host {
		updatePatch["Metadata.Site"] = r.VcdClient.VcdAuthConfig.Host
	}

	networkName := r.VcdClient.NetworkName
	if vcdCluster.Spec.OvdcNetwork != "" {
		networkName = vcdCluster.Spec.OvdcNetwork
	}
	if networkName != capvcdEntity.Spec.Settings.OvdcNetwork {
		updatePatch["Spec.Settings.OvdcNetwork"] = networkName
	}

	if capvcdEntity.Spec.Topology.ControlPlane.Count != int32(0) {
		updatePatch["Spec.Topology.ControlPlane.Count"] = int32(0) // TODO (3097): Get proper control palne count value
	}
	if capvcdEntity.Spec.Topology.ControlPlane.SizingClass != "" {
		updatePatch["Spec.Topology.ControlPlane.SizingClass"] = "" // TODO (3097): Get proper control palne count value
	}

	if capvcdEntity.Spec.Topology.Workers.Count != int32(0) {
		updatePatch["Spec.Topology.Workers.Count"] = int32(0) // TODO (3097): Get proper control palne count value
	}
	if capvcdEntity.Spec.Topology.Workers.SizingClass != "" {
		updatePatch["Spec.Topology.Workers.SizingClass"] = "" // TODO (3097): Get proper control palne count value
	}

	if capvcdEntity.Spec.Distribution.TemplateName != "some-template-name" {
		updatePatch["Spec.Distribution.TemplateName"] = "some-template-name" // TODO (3097): Get proper control palne count value
	}

	if capvcdEntity.Status.Uid != vcdCluster.Status.ClusterRDEId {
		updatePatch["Status.Uid"] = vcdCluster.Status.ClusterRDEId
	}

	if capvcdEntity.Status.Phase != cluster.Status.Phase {
		updatePatch["Status.Phase"] = cluster.Status.Phase
	}
	clusterApiStatus := vcdtypes.ClusterApiStatus{
		Phase: "", // TODO: Find out what should be filled out here
		ApiEndpoints: []vcdtypes.ApiEndpoints{
			{
				Host: controlPlaneIP,
				Port: 6443,
			},
		},
	}
	if !reflect.DeepEqual(clusterApiStatus, capvcdEntity.Status.ClusterAPIStatus) {
		updatePatch["Status.ClusterAPIStatus"] = clusterApiStatus
	}

	updatedRDE, err := r.VcdClient.PatchRDE(ctx, updatePatch, vcdCluster.Status.ClusterRDEId)
	if err != nil {
		return fmt.Errorf("failed to update defined entity with ID [%s]: [%v]", vcdCluster.Status.ClusterRDEId, err)
	}

	if updatedRDE.State != RDEStatusResolved {
		// try to resolve the defined entity
		entityState, resp, err := r.VcdClient.ApiClient.DefinedEntityApi.ResolveDefinedEntity(ctx, updatedRDE.Id)
		if err != nil {
			return fmt.Errorf("failed to resolve defined entity with ID [%s] for cluster [%s]", vcdCluster.Status.ClusterRDEId, vcdCluster.Name)
		}
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("error while resolving defined entity with ID [%s] for cluster [%s] with message: [%s]", vcdCluster.Status.ClusterRDEId, vcdCluster.Name, entityState.Message)
		}
		if entityState.State != RDEStatusResolved {
			return fmt.Errorf("defined entity resolution failed for RDE with ID [%s] for cluster [%s] with message: [%s]", vcdCluster.Status.ClusterRDEId, vcdCluster.Name, entityState.Message)
		}
		klog.Infof("resolved defined entity with ID [%s] for cluster [%s]", vcdCluster.Status.ClusterRDEId, vcdCluster.Name)
	}
	return nil
}

func (r *VCDClusterReconciler) reconcileNormal(ctx context.Context, cluster *clusterv1.Cluster, vcdCluster *infrav1.VCDCluster) (ctrl.Result, error) {

	_ = ctrl.LoggerFrom(ctx)

	gateway := &vcdclient.GatewayManager{
		NetworkName:        r.VcdClient.NetworkName,
		Client:             r.VcdClient,
		GatewayRef:         r.VcdClient.GatewayRef,
		NetworkBackingType: r.VcdClient.NetworkBackingType,
	}
	// Replace the default ovdc network with the user specified inputs.
	if vcdCluster.Spec.OvdcNetwork != "" && vcdCluster.Spec.OvdcNetwork != r.VcdClient.NetworkName {
		gateway.NetworkName = vcdCluster.Spec.OvdcNetwork
		err := gateway.CacheGatewayDetails(ctx)
		if err != nil {
			return ctrl.Result{}, errors.Wrapf(err, "unable to cache ovdc network details for [%s]: [%v]", vcdCluster.Spec.OvdcNetwork, err)
		}
	}

	// NOTE: Since RDE is used just as a book-keeping mechanism, we should not fail reconciliation if RDE operations fail
	// create RDE for cluster
	if vcdCluster.Status.ClusterRDEId == "" {
		nameFilter := &swagger.DefinedEntityApiGetDefinedEntitiesByEntityTypeOpts{
			Filter: optional.NewString(fmt.Sprintf("name==%s", vcdCluster.Name)),
		}
		definedEntities, resp, err := r.VcdClient.ApiClient.DefinedEntityApi.GetDefinedEntitiesByEntityType(ctx, CAPVCDTypeVendor, CAPVCDTypeNss, CAPVCDTypeVersion, 1, 25, nameFilter)
		if err != nil {
			klog.Errorf("failed to get entities by entity type [%s] with name filter [name==%s]", CAPVCDEntityTypeID, vcdCluster.Name)
		}
		if resp.StatusCode != http.StatusOK {
			klog.Errorf("error while getting entities by entity type [%s] with name filter [name==%s]", CAPVCDEntityTypeID, vcdCluster.Name)
		}
		if err == nil && resp.StatusCode == http.StatusOK {
			if len(definedEntities.Values) == 0 {
				rde, err := r.constructCapvcdRDE(vcdCluster)
				if err != nil {
					return ctrl.Result{}, errors.Wrapf(err, "unable to create defined entity for cluster [%s]", vcdCluster.Name)
				}

				resp, err := r.VcdClient.ApiClient.DefinedEntityApi.CreateDefinedEntity(ctx, *rde, rde.EntityType, nil)
				if err != nil {
					return ctrl.Result{}, errors.Wrapf(err, "failed to create defined entity for cluster [%s]", vcdCluster.Name)
				}

				if resp.StatusCode != http.StatusAccepted {
					return ctrl.Result{}, errors.Wrapf(err, "error while creating the defined entity for cluster [%s]", vcdCluster.Name)
				}
				taskURL := resp.Header.Get(VCDLocationHeader)
				task := govcd.NewTask(&r.VcdClient.VcdClient.Client)
				task.Task.HREF = taskURL
				err = task.Refresh()
				if err == nil {
					vcdCluster.Status.ClusterRDEId = task.Task.Owner.ID
					klog.Infof("created defined entity for cluster [%s]. RDE ID: [%s]", vcdCluster.Name, vcdCluster.Status.ClusterRDEId)
				} else {
					klog.Errorf("error refreshing task: [%s]", task.Task.HREF)
				}
			} else {
				klog.Infof("defined entity for cluster [%s] already present. RDE ID: [%s]", vcdCluster.Name, definedEntities.Values[0].Id)
				vcdCluster.Status.ClusterRDEId = definedEntities.Values[0].Id
			}
		}
	}

	// TODO: What should be the prefix if cluster creation fails here?
	// create load balancer for the cluster. Only one-arm load balancer is fully tested.
	virtualServiceNamePrefix := vcdCluster.Name + "-" + vcdCluster.Status.ClusterRDEId
	lbPoolNamePrefix := vcdCluster.Name + "-" + vcdCluster.Status.ClusterRDEId

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

	_, resp, _, err := r.VcdClient.ApiClient.DefinedEntityApi.GetDefinedEntity(ctx, vcdCluster.Status.ClusterRDEId)
	if err != nil {
		klog.Errorf("failed to get defined entity with ID [%s] for cluster [%s]: [%s]", vcdCluster.Status.ClusterRDEId, vcdCluster.Name, err)
	}
	if resp.StatusCode != http.StatusOK {
		klog.Errorf("error getting defined entity with ID [%s] for cluster [%s]", vcdCluster.Status.ClusterRDEId, vcdCluster.Name)
	}
	if err == nil && resp.StatusCode == http.StatusOK {
		if err = r.syncRDE(ctx, cluster, vcdCluster, controlPlaneNodeIP); err != nil {
			klog.Errorf("failed to update and resolve defined entity with ID [%s] for cluster [%s]: [%v]", vcdCluster.Status.ClusterRDEId, vcdCluster.Name, err)
		}
	}

	// Update the vcdCluster resource with updated information
	// TODO Check if updating ovdcNetwork, Org and Ovdc should be done somewhere earlier in the code.
	vcdCluster.Spec.ControlPlaneEndpoint = infrav1.APIEndpoint{
		Host: controlPlaneNodeIP,
		Port: 6443,
	}
	vcdCluster.Spec.OvdcNetwork = gateway.NetworkName
	vcdCluster.Spec.Org = r.VcdClient.VcdAuthConfig.Org
	vcdCluster.Spec.Ovdc = r.VcdClient.VcdAuthConfig.VDC
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
	gateway := &vcdclient.GatewayManager{
		NetworkName:        r.VcdClient.NetworkName,
		Client:             r.VcdClient,
		GatewayRef:         r.VcdClient.GatewayRef,
		NetworkBackingType: r.VcdClient.NetworkBackingType,
	}
	// Replace the default ovdc network with the user specified inputs.
	if vcdCluster.Spec.OvdcNetwork != "" && vcdCluster.Spec.OvdcNetwork != r.VcdClient.NetworkName {
		gateway.NetworkName = vcdCluster.Spec.OvdcNetwork
		err := gateway.CacheGatewayDetails(ctx)
		if err != nil {
			return ctrl.Result{}, errors.Wrapf(err, "unable to cache ovdc network details [%s]: [%v]", vcdCluster.Spec.OvdcNetwork, err)
		}
	}

	// Delete the load balancer components
	virtualServiceNamePrefix := vcdCluster.Name + "-" + vcdCluster.Status.ClusterRDEId
	lbPoolNamePrefix := vcdCluster.Name + "-" + vcdCluster.Status.ClusterRDEId
	err = gateway.DeleteLoadBalancer(ctx, virtualServiceNamePrefix, lbPoolNamePrefix)
	if err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "unable to delete load balancer [%s]: [%v]", virtualServiceNamePrefix, err)
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
			return ctrl.Result{}, errors.Wrapf(err, "unable to cache ovdc details [%s]: [%v]", vcdCluster.Spec.Ovdc, err)
		}
	}

	// Delete vApp
	vApp, err := vdcManager.Vdc.GetVAppByName(vcdCluster.Name, true)
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
	if vcdCluster.Status.ClusterRDEId != "" {
		definedEntities, resp, err := r.VcdClient.ApiClient.DefinedEntityApi.GetDefinedEntitiesByEntityType(ctx, CAPVCDTypeVendor, CAPVCDTypeNss, CAPVCDTypeVersion, 1, 25, &swagger.DefinedEntityApiGetDefinedEntitiesByEntityTypeOpts{
			Filter: optional.NewString(fmt.Sprintf("id==%s", vcdCluster.Status.ClusterRDEId)),
		})
		if err != nil {
			return ctrl.Result{}, errors.Wrapf(err, "failed to fetch defined entities by entity type [%s] and ID [%s] for cluster [%s]", CAPVCDEntityTypeID, vcdCluster.Status.ClusterRDEId, vcdCluster.Name)
		}
		if resp.StatusCode != http.StatusOK {
			return ctrl.Result{}, errors.Errorf("error while fetching defined entities by entity type [%s] and ID [%s] for cluster [%s]", CAPVCDEntityTypeID, vcdCluster.Status.ClusterRDEId, vcdCluster.Name)
		}
		if len(definedEntities.Values) > 0 {
			// resolve defined entity before deleting
			entityState, resp, err := r.VcdClient.ApiClient.DefinedEntityApi.ResolveDefinedEntity(ctx, vcdCluster.Status.ClusterRDEId)
			if err != nil {
				return ctrl.Result{}, errors.Wrapf(err, "failed to resolve defined entity for cluster [%s] with ID [%s] before deleting", vcdCluster.Name, vcdCluster.Status.ClusterRDEId)
			}
			if resp.StatusCode != http.StatusOK {
				klog.Errorf("failed to resolve RDE with ID [%s] for cluster [%s]: [%s]", vcdCluster.Status.ClusterRDEId, vcdCluster.Name, entityState.Message)
			}
			resp, err = r.VcdClient.ApiClient.DefinedEntityApi.DeleteDefinedEntity(ctx, vcdCluster.Status.ClusterRDEId, nil)
			if err != nil {
				return ctrl.Result{}, errors.Wrapf(err, "falied to execute delete defined entity call for RDE with ID [%s]", vcdCluster.Status.ClusterRDEId)
			}
			if resp.StatusCode != http.StatusNoContent {
				return ctrl.Result{}, errors.Errorf("error deleting defined entity associated with the cluster. RDE id: [%s]", vcdCluster.Status.ClusterRDEId)
			}
			log.Info("successfully deleted the defined entity for cluster", "clusterName", vcdCluster.Name)
		} else {
			log.Info("No defined entity found", "clusterName", vcdCluster.Name, "RDE ID", vcdCluster.Status.ClusterRDEId)
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
