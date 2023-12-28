package utils

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/antihax/optional"
	"github.com/pkg/errors"
	"github.com/vmware/cloud-provider-for-cloud-director/pkg/testingsdk"
	"github.com/vmware/cloud-provider-for-cloud-director/pkg/vcdsdk"
	swagger "github.com/vmware/cloud-provider-for-cloud-director/pkg/vcdswaggerclient_36_0"
	"github.com/vmware/cluster-api-provider-cloud-director/api/v1beta3"
	"github.com/vmware/cluster-api-provider-cloud-director/controllers"
	"github.com/vmware/cluster-api-provider-cloud-director/pkg/capisdk"
	"github.com/vmware/cluster-api-provider-cloud-director/pkg/util"
	rdeType "github.com/vmware/cluster-api-provider-cloud-director/pkg/vcdtypes/rde_type_1_1_0"
	"io"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/serializer/yaml"
	"k8s.io/apimachinery/pkg/types"
	k8syaml "k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/clientcmd"
	"net/http"
	clusterv1beta1 "sigs.k8s.io/cluster-api/api/v1beta1"
	kcpv1beta1 "sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1beta1"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"strings"
)

func ValidateKubeConfig(ctx context.Context, kubeConfigBytes []byte) (*kubernetes.Clientset, error) {
	workloadRestConfig, csConfig, err := CreateClientConfig(kubeConfigBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to create client configuration: %v", err)
	}

	if workloadRestConfig == nil {
		return nil, fmt.Errorf("failed to create REST configuration from kubeconfig")
	}

	if csConfig == nil {
		return nil, fmt.Errorf("failed to create clientset from REST configuration")
	}

	fmt.Println("checking pods of the cluster from all the namespaces")
	podList, err := csConfig.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return csConfig, fmt.Errorf("failed to list pods: %v", err)
	}

	if podList == nil || len(podList.Items) == 0 {
		return csConfig, fmt.Errorf("kubeConfig is invalid as the workload cluster should have more than 1 pod")
	}

	return csConfig, nil
}

func CreateClientConfig(kubeConfigBytes []byte) (*rest.Config, *kubernetes.Clientset, error) {
	workloadRestConfig, err := clientcmd.RESTConfigFromKubeConfig(kubeConfigBytes)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create REST config from kubeconfig: %w", err)
	}

	csConfig, err := kubernetes.NewForConfig(workloadRestConfig)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create clientset from REST config: %w", err)
	}

	return workloadRestConfig, csConfig, nil
}

func GetVCDClusterFromCluster(ctx context.Context, client runtimeclient.Client, namespace string, name string) (*v1beta3.VCDCluster, error) {
	vcdCluster, err := getVCDCluster(ctx, client, namespace, name)
	if err != nil {
		return nil, fmt.Errorf("failed to get VCDCluster: %w", err)
	}

	if vcdCluster == nil {
		return nil, fmt.Errorf("VCDCluster is nil")
	}

	return vcdCluster, nil
}

func GetClusterNameAndNamespaceFromCAPIYaml(yamlContent []byte) (string, string, error) {
	var err error
	var resourceName string
	var namespace string
	yamlReader := k8syaml.NewYAMLReader(bufio.NewReader(bytes.NewReader(yamlContent)))
	hundredKB := 100 * 1024
	for err == nil {
		yamlBytes, readErr := yamlReader.Read()
		if readErr == io.EOF {
			break
		}
		yamlDecoder := k8syaml.NewYAMLOrJSONDecoder(bytes.NewReader(yamlBytes), hundredKB)
		unstructuredObj := unstructured.Unstructured{}
		err = yamlDecoder.Decode(&unstructuredObj)
		if err != nil {
			return "", "", fmt.Errorf("failed to decode YAML object: %w", err)
		}

		kind := unstructuredObj.GetKind()
		resourceName = unstructuredObj.GetName()
		namespace = unstructuredObj.GetNamespace()
		switch kind {
		case Cluster:
			if resourceName == "" || namespace == "" {
				return resourceName, namespace, fmt.Errorf("please examine the format of capi yaml. The clustername and namespace should not be empty")
			}
			return resourceName, namespace, nil
		}

		if err != nil {
			return resourceName, namespace, fmt.Errorf("failed to get clustername and namespace: %v", err)
		}
	}
	return resourceName, namespace, nil
}

func GetUserOrgAndName(_ context.Context, org, username string) (string, string) {
	parts := strings.Split(username, "/")
	if len(parts) == 2 {
		return parts[0], parts[1]
	}

	return org, username
}

func ConstructCAPVCDTestClient(ctx context.Context, vcdCluster *v1beta3.VCDCluster, workloadClientSet *kubernetes.Clientset, capiYaml, clusterNameSpace, clusterName, rdeId string, getVdcClient bool) (*testingsdk.TestClient, error) {
	var refreshToken, username string
	secret, err := GetSecret(ctx, capiYaml)
	if err != nil {
		return nil, fmt.Errorf("unable to get Secret from CapiYaml: [%v]", err)
	}
	if b, exists := secret.Data["username"]; exists {
		username = strings.TrimRight(string(b), "\n")
	}
	if b, exists := secret.Data["refreshToken"]; exists {
		refreshToken = strings.TrimRight(string(b), "\n")
	}

	userOrgName, userName := GetUserOrgAndName(ctx, vcdCluster.Status.Org, username)

	testClient, err := NewTestClient(vcdCluster.Status.Site, vcdCluster.Status.Org, userOrgName, vcdCluster.Status.Ovdc, userName, refreshToken, rdeId, getVdcClient)
	if err != nil {
		return nil, fmt.Errorf("failed to construct testing client: [%v]", err)
	}
	// Using workload client as the Kubernetes client of testClient
	testClient.Cs = workloadClientSet
	return testClient, nil
}

var decUnstructured = yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)

func doSSA(ctx context.Context, cfg *rest.Config, yamlBytes []byte) error {

	// 1. Prepare a RESTMapper to find GVR
	dc, err := discovery.NewDiscoveryClientForConfig(cfg)
	if err != nil {
		return err
	}
	mapper := restmapper.NewDeferredDiscoveryRESTMapper(memory.NewMemCacheClient(dc))

	// 2. Prepare the dynamic client
	dyn, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return err
	}

	yamlReader := k8syaml.NewYAMLReader(bufio.NewReader(bytes.NewReader(yamlBytes)))
	for {
		yamlBytes, err := yamlReader.Read()
		if err == io.EOF {
			break
		} else if err != nil {
			return err
		}

		// 3. Decode YAML manifest into unstructured.Unstructured
		obj := &unstructured.Unstructured{}
		_, gvk, err := decUnstructured.Decode(yamlBytes, nil, obj)
		if err != nil {
			return err
		}

		// 4. Find GVR
		mapping, err := mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
		if err != nil {
			return err
		}

		// 5. Obtain REST interface for the GVR
		var dr dynamic.ResourceInterface
		if mapping.Scope.Name() == meta.RESTScopeNameNamespace {
			// namespaced resources should specify the namespace
			dr = dyn.Resource(mapping.Resource).Namespace(obj.GetNamespace())
		} else {
			// for cluster-wide resources
			dr = dyn.Resource(mapping.Resource)
		}

		// 6. Marshal object into JSON
		data, err := json.Marshal(obj)
		if err != nil {
			return err
		}

		// 7. Create or Update the object with SSA
		//     types.ApplyPatchType indicates SSA.
		//     FieldManager specifies the field owner ID.
		_, err = dr.Patch(ctx, obj.GetName(), types.ApplyPatchType, data, metav1.PatchOptions{
			FieldManager: "capvcd-standalone-test",
		})
	}

	return err
}

func InstallAntreaFromYaml(ctx context.Context, workloadKubeconfigBytes []byte, antreaYamlBytes []byte) error {
	restConfig, err := clientcmd.RESTConfigFromKubeConfig(workloadKubeconfigBytes)
	if err != nil {
		return fmt.Errorf("failed to create REST config from kubeconfig: [%v]", err)
	}
	err = doSSA(ctx, restConfig, antreaYamlBytes)
	if err != nil {
		return fmt.Errorf("error occurred while applying antrea yaml: [%v]", err)
	}
	return nil
}

func CreateWorkloadClusterResources(ctx context.Context, workloadClientSet *kubernetes.Clientset,
	capiYaml, rdeID string, vcdcluster *v1beta3.VCDCluster) error {

	var refreshToken string
	namespace := "kube-system"
	secret, err := GetSecret(ctx, capiYaml)
	if err != nil {
		return fmt.Errorf("unable to get Secret from CapiYaml: [%v]", err)
	}

	if b, exists := secret.Data["refreshToken"]; exists {
		refreshToken = strings.TrimRight(string(b), "\n")
	}

	err = CreateClusterIDSecret(ctx, workloadClientSet, namespace, rdeID)
	if err != nil {
		return fmt.Errorf("failed to create cluster ID secret: %w", err)
	}

	err = CreateAuthSecret(ctx, workloadClientSet, namespace, refreshToken)
	if err != nil {
		return fmt.Errorf("failed to create auth secret: %w", err)
	}

	host := vcdcluster.Status.Site

	org := vcdcluster.Status.Org

	clusterName := vcdcluster.Name

	ovdc := vcdcluster.Status.Ovdc

	ovdcNetwork := vcdcluster.Status.OvdcNetwork

	if host == "" {
		return fmt.Errorf("host is empty in the cluster %s", rdeID)
	}

	if org == "" {
		return fmt.Errorf("org is empty in the cluster %s", rdeID)
	}

	if clusterName == "" {
		return fmt.Errorf("clusterName is empty in the cluster %s", rdeID)
	}

	if ovdc == "" {
		return fmt.Errorf("ovdc is empty in the cluster %s", rdeID)
	}

	if ovdcNetwork == "" {
		return fmt.Errorf("ovdcNetwork is empty in the cluster %s", rdeID)
	}

	err = CreateCCMConfigMap(ctx, workloadClientSet, namespace, host, org, ovdc, rdeID, clusterName, ovdcNetwork)
	if err != nil {
		return fmt.Errorf("failed to create CCM config map: %w", err)
	}

	err = CreateCSIConfigMap(ctx, workloadClientSet, namespace, host, org, ovdc, rdeID, clusterName)
	if err != nil {
		return fmt.Errorf("failed to create CSI config map: %w", err)
	}
	return nil
}

func GetSecret(_ context.Context, capiYaml string) (*corev1.Secret, error) {
	hundredKB := 100 * 1024
	var err error = nil
	var secret *corev1.Secret

	yamlReader := k8syaml.NewYAMLReader(bufio.NewReader(bytes.NewReader([]byte(capiYaml))))

	for err == nil {
		yamlBytes, err := yamlReader.Read()
		if err == io.EOF {
			break
		}
		yamlDecoder := k8syaml.NewYAMLOrJSONDecoder(bytes.NewReader(yamlBytes), hundredKB)
		unstructuredObj := unstructured.Unstructured{}
		err = yamlDecoder.Decode(&unstructuredObj)
		if err != nil {
			return nil, fmt.Errorf("unable to parse yaml segment: [%v]\n", err)
		}

		kind := unstructuredObj.GetKind()
		name := unstructuredObj.GetName()
		switch kind {
		case SECRET:
			secret = &corev1.Secret{}
			yamlDecoder := k8syaml.NewYAMLOrJSONDecoder(bytes.NewReader(yamlBytes), hundredKB)
			if err = yamlDecoder.Decode(secret); err != nil {
				return nil, fmt.Errorf("unable to parse object of kind [%s] and name [%s]: [%v]\n",
					kind, name, err)
			}
			break
		}
	}

	err = nil
	if secret == nil {
		err = fmt.Errorf("unexpected empty secret in yaml")
	}

	return secret, err
}

func CheckRdeEntityNonExisted(ctx context.Context, client *vcdsdk.Client, rdeID, clusterName string) error {
	org, err := client.VCDClient.GetOrgByName(client.ClusterOrgName)
	if err != nil {
		return fmt.Errorf("failed to get org by name [%s]: %v", client.ClusterOrgName, err)
	}

	if org == nil || org.Org == nil {
		return fmt.Errorf("obtained nil org when getting org by name [%s]", client.ClusterOrgName)
	}

	definedEntities, resp, err := client.APIClient.DefinedEntityApi.GetDefinedEntitiesByEntityType(
		ctx,
		capisdk.CAPVCDTypeVendor,
		capisdk.CAPVCDTypeNss,
		rdeType.CapvcdRDETypeVersion,
		org.Org.ID,
		1,
		25,
		&swagger.DefinedEntityApiGetDefinedEntitiesByEntityTypeOpts{
			Filter: optional.NewString(fmt.Sprintf("id==%s", rdeID)),
		},
	)
	if err != nil {
		return fmt.Errorf("error occurred during RDE deletion; failed to fetch defined entities by entity ID [%s] for cluster [%s]: %v", rdeID, clusterName, err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("error occurred during RDE deletion; error while fetching defined entities by entity ID [%s] for cluster [%s]", rdeID, clusterName)
	}

	if len(definedEntities.Values) > 0 {
		return fmt.Errorf("the RDE entity of cluster [%s (%s)] still exists", rdeID, clusterName)
	}

	fmt.Println("The RDE entity of cluster", clusterName, "with ID", rdeID, "has been successfully deleted.")

	return nil
}

// VerifyVappMetadataForCluster verifies if the metadata "CapvcdInfraId" is added with the right value (infraID)
func VerifyVappMetadataForCluster(vcdClient *vcdsdk.Client, vcdCluster *v1beta3.VCDCluster) error {
	if vcdCluster == nil {
		return fmt.Errorf("vcdCluster object is nil")
	}

	orgName := vcdCluster.Status.Org
	ovdcName := vcdCluster.Status.Ovdc
	vAppName := vcdCluster.Name

	vdcManager, err := vcdsdk.NewVDCManager(vcdClient, orgName, ovdcName)
	if err != nil {
		return fmt.Errorf("error occurred while creating the VDC manager object: [%v]", err)
	}

	vApp, err := vdcManager.Vdc.GetVAppByName(vAppName, true)
	if err != nil {
		return fmt.Errorf("error occurred while getting VApp by name [%s]", vAppName)
	}

	val, err := vdcManager.GetMetadataByKey(vApp, controllers.CapvcdInfraId)
	if err != nil {
		return fmt.Errorf("error occurred while getting metadata by key for VApp [%s] and metadata key [%s]: [%v]",
			vAppName, controllers.CapvcdInfraId, err)
	}

	if val != vcdCluster.Status.InfraId {
		return fmt.Errorf("Infra ID [%s] doesn't match the value for the metadata key; metadata key [%s], metadata value [%s]",
			vcdCluster.Status.InfraId, controllers.CapvcdInfraId, val)
	}

	return nil
}

func VerifyRDEContents(ctx context.Context, runtimeClient runtimeclient.Client,
	vcdClient *vcdsdk.Client, vcdCluster *v1beta3.VCDCluster, capiYaml string) error {

	if vcdCluster == nil {
		return fmt.Errorf("vcdCluster object is nil")
	}
	infraID := vcdCluster.Status.InfraId
	if strings.HasPrefix(infraID, vcdsdk.NoRdePrefix) {
		fmt.Println("skipping verification of RDE contents as te Infra ID is auto-generated")
		return nil
	}

	org, err := vcdClient.VCDClient.GetOrgByName(vcdCluster.Spec.Org)
	if err != nil {
		return fmt.Errorf("error getting org by name [%s]", vcdCluster.Spec.Org)
	}

	if org == nil || org.Org == nil {
		return fmt.Errorf("nil org found when getting org by name [%s]", vcdCluster.Spec.Org)
	}

	// get entity
	definedEntity, resp, _, err := vcdClient.APIClient.DefinedEntityApi.GetDefinedEntity(ctx, infraID, org.Org.ID)
	if err != nil {
		return fmt.Errorf("error occurred while getting defined entity by ID: [%s]", infraID)
	}
	if resp == nil {
		return fmt.Errorf("nil response while getting defined entity by ID [%s]", infraID)
	} else if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected response code while getting defined entity by ID [%s]", infraID)
	}

	capvcdEntity, err := util.ConvertMapToCAPVCDEntity(definedEntity.Entity)
	if err != nil {
		return fmt.Errorf("error converting RDE.entity to CAPVCD entity: [%v]", err)
	}

	if err := verifyRDENodePools(ctx, runtimeClient, capvcdEntity, vcdCluster, capiYaml); err != nil {
		return fmt.Errorf("error occurred while verifying RDE node pools: [%v]", err)
	}
	return nil
}

// getNodePoolCountFromCapiYaml returns the list of node pools added to the node pool section in RDE.
// note that KubeadmControlPlane is also added as a node pool in RDE
func verifyRDENodePools(ctx context.Context, runtimeClient runtimeclient.Client, capvcdEntity *rdeType.CAPVCDEntity,
	vcdCluster *v1beta3.VCDCluster, capiYaml string) error {

	yamlObjects, err := getObjectsFromYamlString(capiYaml)
	if err != nil {
		return fmt.Errorf("failed to get the yaml objects from the CAPI YAML: [%v]", err)
	}
	type nodePoolNodeCounts struct {
		availableNodeCount int32
		desiredNodeCount   int32
		replicasInStatus   int32
	}
	nodePoolMaps := make(map[string]nodePoolNodeCounts)
	totalNodePoolsInCapiYaml := 0
	for _, yamlObject := range yamlObjects {
		switch yamlObject.GetKind() {
		case "KubeadmControlPlane":
			kcpNamespaced := types.NamespacedName{
				Name:      yamlObject.GetName(),
				Namespace: yamlObject.GetNamespace(),
			}
			kcpObj := kcpv1beta1.KubeadmControlPlane{}
			if err := runtimeClient.Get(ctx, kcpNamespaced, &kcpObj); err != nil {
				return fmt.Errorf("failed to get the kcp object [%s/%s]: [%v]",
					yamlObject.GetNamespace(), yamlObject.GetName(), err)
			}
			n := nodePoolNodeCounts{
				availableNodeCount: kcpObj.Status.ReadyReplicas,
				desiredNodeCount:   *kcpObj.Spec.Replicas,
				replicasInStatus:   kcpObj.Status.Replicas,
			}
			nodePoolMaps[yamlObject.GetName()] = n
			totalNodePoolsInCapiYaml++
			break
		case "MachineDeployment":
			mdNamespaced := types.NamespacedName{
				Name:      yamlObject.GetName(),
				Namespace: yamlObject.GetNamespace(),
			}
			mdObj := clusterv1beta1.MachineDeployment{}
			if err := runtimeClient.Get(ctx, mdNamespaced, &mdObj); err != nil {
				return fmt.Errorf("failed to get the machine deployment object [%s/%s]: [%v]",
					yamlObject.GetNamespace(), yamlObject.GetName(), err)
			}
			n := nodePoolNodeCounts{
				availableNodeCount: mdObj.Status.ReadyReplicas,
				desiredNodeCount:   *mdObj.Spec.Replicas,
				replicasInStatus:   mdObj.Status.Replicas,
			}
			nodePoolMaps[yamlObject.GetName()] = n
			totalNodePoolsInCapiYaml++
			break
		}
	}

	kcpList, err := getAllKubeadmControlPlaneForCluster(ctx, runtimeClient, vcdCluster.Name, vcdCluster.Namespace)
	if err != nil {
		return fmt.Errorf("failed to get all the kubeadmcontrolplane for the cluster [%s/%s]: [%v]",
			vcdCluster.Name, vcdCluster.Namespace, err)
	}

	mdList, err := getAllMachineDeploymentsForCluster(ctx, runtimeClient, vcdCluster.Name, vcdCluster.Namespace)
	if err != nil {
		return fmt.Errorf("failed to get all the MachineDeployments for the cluster [%s/%s]: [%v]",
			vcdCluster.Name, vcdCluster.Namespace, err)
	}

	if len(mdList.Items)+len(kcpList.Items) != totalNodePoolsInCapiYaml {
		return fmt.Errorf("expected number of node pools [%s]; Found [%d] node pools in the RDE CAPVCD status",
			len(mdList.Items)+len(kcpList.Items), totalNodePoolsInCapiYaml)
	}

	// verify the node counts with capvcdEntity
	rdeNodePools := capvcdEntity.Status.CAPVCDStatus.NodePool

	for _, rdeNodePool := range rdeNodePools {
		k8sPool, ok := nodePoolMaps[rdeNodePool.Name]
		if !ok {
			return fmt.Errorf("node pool [%s] not found in capi yaml", rdeNodePool.Name)
		}

		// following condition checks if the RDE contains the correct desired node count for the node pool
		if k8sPool.desiredNodeCount != rdeNodePool.DesiredReplicas {
			return fmt.Errorf("expected desired node count [%d] in node pool [%s] of RDE but has node count [%d]",
				rdeNodePool.DesiredReplicas, rdeNodePool.Name, k8sPool.desiredNodeCount)
		}

		// TODO: this condition may not be satisfied if the vcdcluster controller has not reconciled but the
		//   machines are in ready state
		//if k8sPool.availableNodeCount != rdeNodePool.AvailableReplicas {
		//	return fmt.Errorf("expected desired node count [%d] in node pool [%s] of RDE but has node count [%d]",
		//		rdeNodePool.AvailableReplicas, rdeNodePool.Name, k8sPool.availableNodeCount)
		//}

		// the following condition checks if all the nodes have been created
		if k8sPool.replicasInStatus != rdeNodePool.DesiredReplicas {
			return fmt.Errorf("expected replicas [%d] but found [%d] replicas in RDE status for node pool [%s]",
				k8sPool.replicasInStatus, rdeNodePool.DesiredReplicas)
		}
	}

	fmt.Println("successfully verified the RDE contents of the node pools section")

	return nil
}

func getAllMachineDeploymentsForCluster(ctx context.Context, cli runtimeclient.Client, clusterName, clusterNamespace string) (*clusterv1beta1.MachineDeploymentList, error) {
	mdListLabels := map[string]string{clusterv1beta1.ClusterNameLabel: clusterName}
	mdList := &clusterv1beta1.MachineDeploymentList{}
	if err := cli.List(ctx, mdList, runtimeclient.InNamespace(clusterNamespace), runtimeclient.MatchingLabels(mdListLabels)); err != nil {
		return nil, errors.Wrapf(err, "error getting machine deployments for the cluster [%s]", clusterName)
	}
	return mdList, nil
}

func getAllKubeadmControlPlaneForCluster(ctx context.Context, cli runtimeclient.Client, clusterName, clusterNamespace string) (*kcpv1beta1.KubeadmControlPlaneList, error) {
	kcpListLabels := map[string]string{clusterv1beta1.ClusterNameLabel: clusterName}
	kcpList := &kcpv1beta1.KubeadmControlPlaneList{}

	if err := cli.List(ctx, kcpList, runtimeclient.InNamespace(clusterNamespace), runtimeclient.MatchingLabels(kcpListLabels)); err != nil {
		return nil, errors.Wrapf(err, "error getting all kubeadm control planes for the cluster [%s]", clusterName)
	}
	return kcpList, nil
}
