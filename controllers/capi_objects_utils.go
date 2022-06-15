package controllers

import (
	"context"
	"fmt"
	"github.com/pkg/errors"
	infrav1 "github.com/vmware/cluster-api-provider-cloud-director/api/v1beta1"
	rdeType "github.com/vmware/cluster-api-provider-cloud-director/pkg/vcdtypes/rde_type_1_1_0"
	"gopkg.in/yaml.v2"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1beta1"
	kcpv1 "sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"strings"
)

// filterTypeMetaAndObjectMetaFromK8sObjectMap is a helper function to remove extraneous contents in "objectmeta" and "typemeta"
//  keys. The function moves name and namespace from "objectmeta" key to "metadata" key and moves all the keys from "typemeta"
//  key to objMap
func filterTypeMetaAndObjectMetaFromK8sObjectMap(objMap map[string]interface{}) error {
	if _, ok := objMap["typemeta"]; ok {
		typeMetaMap, ok := objMap["typemeta"].(map[interface{}]interface{})
		if !ok {
			return fmt.Errorf("failed to convert typemeta [%v] to map[interface{}]interface{}", objMap["typemeta"])
		}
		// move contents of typeMetaMap to objMap. This preserves keys like apiVersion and Kind
		for k, v := range typeMetaMap {
			objMap[k.(string)] = v
		}
		delete(objMap, "typemeta")
	}

	if _, ok := objMap["objectmeta"]; ok {
		objectMetaMap, ok := objMap["objectmeta"].(map[interface{}]interface{})
		if !ok {
			return fmt.Errorf("failed to convert objectmeta [%v] to map[interface{}]interface{}", objMap["objectmeta"])
		}
		// remove all keys from objectMetaMap except for name and namespace.
		for k, _ := range objectMetaMap {
			if k.(string) != "name" && k.(string) != "namespace" {
				delete(objectMetaMap, k)
			}
		}
		// preserve name and namespace of the object as part of "metadata"
		objMap["metadata"] = objectMetaMap
		delete(objMap, "objectmeta")
	}
	return nil
}

func yamlWithoutStatus(obj interface{}) (string, error) {
	// Redact the password and refresh token
	// get yaml string for obj
	objInByteArr, err := yaml.Marshal(obj)
	if err != nil {
		return "", fmt.Errorf("failed to marshal object: [%v]", err)
	}

	objMap := make(map[string]interface{})
	if err := yaml.Unmarshal(objInByteArr, &objMap); err != nil {
		return "", fmt.Errorf("failed to unmarshal object to map[string]interface{}: [%v]", err)
	}

	// delete status key
	if _, ok := objMap["status"]; ok {
		delete(objMap, "status")
	}

	err = filterTypeMetaAndObjectMetaFromK8sObjectMap(objMap)
	if err != nil {
		return "", fmt.Errorf("failed to remove type meta and object meta from kubernetes object [%v]: [%v]", objMap, err)
	}

	// marshal back to a string
	output, err := yaml.Marshal(objMap)
	if err != nil {
		return "", fmt.Errorf("failed to marshal modified object: [%v]", err)
	}
	return string(output), nil
}

func getK8sObjectStatus(obj interface{}) (string, error) {
	// Redact the password and refresh token
	// get yaml string for obj
	objInByteArr, err := yaml.Marshal(obj)
	if err != nil {
		return "", fmt.Errorf("failed to marshal object: [%v]", err)
	}

	objMap := make(map[string]interface{})
	if err := yaml.Unmarshal(objInByteArr, &objMap); err != nil {
		return "", fmt.Errorf("failed to unmarshal object to map[string]interface{}: [%v]", err)
	}

	// delete spec key
	if _, ok := objMap["spec"]; ok {
		delete(objMap, "spec")
	}

	err = filterTypeMetaAndObjectMetaFromK8sObjectMap(objMap)
	if err != nil {
		return "", fmt.Errorf("failed to remove type meta and object meta from kubernetes object [%v]: [%v]", objMap, err)
	}

	// marshal back to a string
	output, err := yaml.Marshal(objMap)
	if err != nil {
		return "", fmt.Errorf("failed to marshal modified object: [%v]", err)
	}
	return string(output), nil
}

func getAllMachineDeploymentsForCluster(ctx context.Context, cli client.Client, c clusterv1.Cluster) (*clusterv1.MachineDeploymentList, error) {
	mdListLabels := map[string]string{clusterv1.ClusterLabelName: c.Name}
	mdList := &clusterv1.MachineDeploymentList{}
	if err := cli.List(ctx, mdList, client.InNamespace(c.Namespace), client.MatchingLabels(mdListLabels)); err != nil {
		return nil, errors.Wrapf(err, "error getting machine deployments for the cluster [%s]", c.Name)
	}
	return mdList, nil
}

func getAllKubeadmControlPlaneForCluster(ctx context.Context, cli client.Client, c clusterv1.Cluster) (*kcpv1.KubeadmControlPlaneList, error) {
	kcpListLabels := map[string]string{clusterv1.ClusterLabelName: c.Name}
	kcpList := &kcpv1.KubeadmControlPlaneList{}

	if err := cli.List(ctx, kcpList, client.InNamespace(c.Namespace), client.MatchingLabels(kcpListLabels)); err != nil {
		return nil, errors.Wrapf(err, "error getting all kubeadm control planes for the cluster [%s]", c.Name)
	}
	return kcpList, nil
}

func getVCDMachineTemplateFromKCP(ctx context.Context, cli client.Client, kcp kcpv1.KubeadmControlPlane) (*infrav1.VCDMachineTemplate, error) {
	vcdMachineTemplateRef := kcp.Spec.MachineTemplate.InfrastructureRef
	vcdMachineTemplate := &infrav1.VCDMachineTemplate{}
	vcdMachineTemplateKey := types.NamespacedName{
		Namespace: vcdMachineTemplateRef.Namespace,
		Name:      vcdMachineTemplateRef.Name,
	}
	if err := cli.Get(ctx, vcdMachineTemplateKey, vcdMachineTemplate); err != nil {
		return nil, fmt.Errorf("failed to get VCDMachineTemplate by name [%s] from KCP [%s]: [%v]", vcdMachineTemplateRef.Name, kcp.Name, err)
	}

	return vcdMachineTemplate, nil
}

func getVCDMachineTemplateFromMachineDeployment(ctx context.Context, cli client.Client, md clusterv1.MachineDeployment) (*infrav1.VCDMachineTemplate, error) {
	vcdMachineTemplateRef := md.Spec.Template.Spec.InfrastructureRef
	vcdMachineTemplate := &infrav1.VCDMachineTemplate{}
	vcdMachineTemplateKey := client.ObjectKey{
		Namespace: vcdMachineTemplateRef.Namespace,
		Name:      vcdMachineTemplateRef.Name,
	}
	if err := cli.Get(ctx, vcdMachineTemplateKey, vcdMachineTemplate); err != nil {
		return nil, fmt.Errorf("failed to get VCDMachineTemplate by name [%s] from machine deployment [%s]: [%v]", vcdMachineTemplateRef.Name, md.Name, err)
	}

	return vcdMachineTemplate, nil
}

func getMachineListFromCluster(ctx context.Context, cli client.Client, cluster clusterv1.Cluster) (*clusterv1.MachineList, error) {
	machineListLabels := map[string]string{clusterv1.ClusterLabelName: cluster.Name}
	machineList := &clusterv1.MachineList{}
	if err := cli.List(ctx, machineList, client.InNamespace(cluster.Namespace), client.MatchingLabels(machineListLabels)); err != nil {
		return nil, errors.Wrapf(err, "error getting machine list for the cluster [%s]", cluster.Name)
	}
	return machineList, nil
}

func getVCDMachineTemplateByObjRef(ctx context.Context, cli client.Client, objRef v1.ObjectReference) (*infrav1.VCDMachineTemplate, error) {
	vcdMachineTemplate := &infrav1.VCDMachineTemplate{}
	vcdMachineTemplateKey := client.ObjectKey{
		Namespace: objRef.Namespace,
		Name:      objRef.Name,
	}
	if err := cli.Get(ctx, vcdMachineTemplateKey, vcdMachineTemplate); err != nil {
		return nil, fmt.Errorf("failed to get VCDMachineTemplate by ObjectReference [%v]: [%v]", objRef, err)
	}

	return vcdMachineTemplate, nil
}

func getKubeadmConfigTemplateByObjRef(ctx context.Context, cli client.Client, objRef v1.ObjectReference) (*v1beta1.KubeadmConfigTemplate, error) {
	kubeadmConfigTemplate := &v1beta1.KubeadmConfigTemplate{}
	kubeadmConfigTemplateKey := client.ObjectKey{
		Namespace: objRef.Namespace,
		Name:      objRef.Name,
	}
	if err := cli.Get(ctx, kubeadmConfigTemplateKey, kubeadmConfigTemplate); err != nil {
		return nil, fmt.Errorf("failed to get KubeadmConfigTemplate by ObjectReference [%v]: [%v]", objRef, err)
	}

	return kubeadmConfigTemplate, nil
}

func getAllMachinesInMachineDeployment(ctx context.Context, cli client.Client, machineDeployment clusterv1.MachineDeployment) (*clusterv1.MachineList, error) {
	machineListLabels := map[string]string{clusterv1.MachineDeploymentLabelName: machineDeployment.Name}
	machineList := &clusterv1.MachineList{}
	if err := cli.List(ctx, machineList, client.InNamespace(machineDeployment.Namespace), client.MatchingLabels(machineListLabels)); err != nil {
		return nil, errors.Wrapf(err, "error getting machine list for the cluster [%s]", machineDeployment.Name)
	}
	return machineList, nil
}

func getAllMachinesInKCP(ctx context.Context, cli client.Client, kcp kcpv1.KubeadmControlPlane, clusterName string) ([]clusterv1.Machine, error) {
	machineListLabels := map[string]string{clusterv1.ClusterLabelName: clusterName}
	machineList := &clusterv1.MachineList{}
	if err := cli.List(ctx, machineList, client.InNamespace(kcp.Namespace), client.MatchingLabels(machineListLabels)); err != nil {
		return nil, errors.Wrapf(err, "error getting machine list associated with KCP [%s]: [%v]", kcp.Name, err)
	}
	// TODO find a better way to find all machines in KCP
	machinesWithKCPOwnerRef := make([]clusterv1.Machine, 0)
	for _, m := range machineList.Items {
		for _, ref := range m.OwnerReferences {
			if ref.Kind == "KubeadmControlPlane" && ref.Name == kcp.Name {
				machinesWithKCPOwnerRef = append(machinesWithKCPOwnerRef, m)
				break
			}
		}
	}
	return machinesWithKCPOwnerRef, nil
}

func getNodePoolList(ctx context.Context, cli client.Client, cluster clusterv1.Cluster) ([]rdeType.NodePool, error) {
	nodePoolList := make([]rdeType.NodePool, 0)
	mds, err := getAllMachineDeploymentsForCluster(ctx, cli, cluster)
	if err != nil {
		return nil, fmt.Errorf("failed to query all machine deployments for the cluster [%s]: [%v]", cluster.Name, err)
	}
	for _, md := range mds.Items {
		// create a node pool for each machine deployment
		vcdMachineTemplate, err := getVCDMachineTemplateFromMachineDeployment(ctx, cli, md)
		if err != nil {
			return nil, fmt.Errorf("failed to get VCDMachineTemplate associated with the MachineDeployment [%s]: [%v]", md.Name, err)
		}
		// query all machines in machine deployment using machine deployment label
		machineList, err := getAllMachinesInMachineDeployment(ctx, cli, md)
		if err != nil {
			return nil, fmt.Errorf("failed to get MachineList for MachineDeployment [%s]: [%v]", md.Name, err)
		}
		nodeStatusMap := make(map[string]string)
		for _, machine := range machineList.Items {
			nodeStatusMap[machine.Name] = machine.Status.Phase
		}
		nodePool := rdeType.NodePool{
			Name:            md.Name,
			SizingPolicy:    vcdMachineTemplate.Spec.Template.Spec.SizingPolicy,
			PlacementPolicy: vcdMachineTemplate.Spec.Template.Spec.PlacementPolicy,
			Replicas:        md.Status.Replicas,
			NodeStatus:      nodeStatusMap,
		}
		nodePoolList = append(nodePoolList, nodePool)
	}

	kcpList, err := getAllKubeadmControlPlaneForCluster(ctx, cli, cluster)
	if err != nil {
		return nil, fmt.Errorf("failed to query all KubeadmControlPlane objects for the cluster [%s]: [%v]", cluster.Name, err)
	}
	for _, kcp := range kcpList.Items {
		// create a node pool for each kcp
		vcdMachineTemplate, err := getVCDMachineTemplateFromKCP(ctx, cli, kcp)
		if err != nil {
			return nil, fmt.Errorf("failed to get VCDMachineTemplate associated with KubeadmControlPlane [%s]: [%v]", kcp.Name, err)
		}
		// query all machines with the kcp
		machineArr, err := getAllMachinesInKCP(ctx, cli, kcp, cluster.Name)
		if err != nil {
			return nil, fmt.Errorf("failed to get Machines associated with the KubeadmControlPlane [%s]: [%v]", kcp.Name, err)
		}
		nodeStatusMap := make(map[string]string)
		for _, machine := range machineArr {
			nodeStatusMap[machine.Name] = machine.Status.Phase
		}
		nodePool := rdeType.NodePool{
			Name:            kcp.Name,
			SizingPolicy:    vcdMachineTemplate.Spec.Template.Spec.SizingPolicy,
			PlacementPolicy: vcdMachineTemplate.Spec.Template.Spec.PlacementPolicy,
			Replicas:        kcp.Status.Replicas,
			NodeStatus:      nodeStatusMap,
		}
		nodePoolList = append(nodePoolList, nodePool)
	}
	return nodePoolList, nil
}

func getK8sClusterObjects(ctx context.Context, cli client.Client, cluster clusterv1.Cluster, vcdCluster infrav1.VCDCluster) ([]interface{}, error) {
	// Redacting username, password and refresh token from the UserCredentialsContext for security purposes.
	vcdCluster.Spec.UserCredentialsContext.Username = "***REDACTED***"
	vcdCluster.Spec.UserCredentialsContext.Password = "***REDACTED***"
	vcdCluster.Spec.UserCredentialsContext.RefreshToken = "***REDACTED***"
	capiYamlObjects := []interface{}{
		cluster,
		vcdCluster,
	}

	kcpList, err := getAllKubeadmControlPlaneForCluster(ctx, cli, cluster)
	if err != nil {
		return nil, fmt.Errorf("failed to get all KCPs from Cluster object: [%v]", err)
	}

	mdList, err := getAllMachineDeploymentsForCluster(ctx, cli, cluster)
	if err != nil {
		return nil, fmt.Errorf("failed to get all the MachineDeployments from Cluster: [%v]", err)
	}

	vcdMachineTemplateNameToObjRef := make(map[string]v1.ObjectReference)
	for _, kcp := range kcpList.Items {
		vcdMachineTemplateNameToObjRef[kcp.Spec.MachineTemplate.InfrastructureRef.Name] = kcp.Spec.MachineTemplate.InfrastructureRef
	}

	kubeadmConfigTemplateNameToObjRef := make(map[string]*v1.ObjectReference)
	for _, md := range mdList.Items {
		vcdMachineTemplateNameToObjRef[md.Spec.Template.Spec.InfrastructureRef.Name] = md.Spec.Template.Spec.InfrastructureRef
		kubeadmConfigTemplateNameToObjRef[md.Spec.Template.Spec.Bootstrap.ConfigRef.Name] = md.Spec.Template.Spec.Bootstrap.ConfigRef
	}

	vcdMachineTemplates := make([]*infrav1.VCDMachineTemplate, 0)
	for _, objRef := range vcdMachineTemplateNameToObjRef {
		vcdMachineTemplate, err := getVCDMachineTemplateByObjRef(ctx, cli, objRef)
		if err != nil {
			return nil, fmt.Errorf("failed to get VCDMachineTemplate by ObjectReference [%v]: [%v]", objRef, err)
		}
		vcdMachineTemplates = append(vcdMachineTemplates, vcdMachineTemplate)
	}

	kubeadmConfigTemplates := make([]*v1beta1.KubeadmConfigTemplate, 0)
	for _, objRef := range kubeadmConfigTemplateNameToObjRef {
		kubeadmConifgTemplate, err := getKubeadmConfigTemplateByObjRef(ctx, cli, *objRef)
		if err != nil {
			return nil, fmt.Errorf("failed to get KubeadmConfigTemplate by ObjectReference [%v]: [%v]", objRef, err)
		}
		kubeadmConfigTemplates = append(kubeadmConfigTemplates, kubeadmConifgTemplate)
	}

	// add objects
	for _, vcdMachineTemplate := range vcdMachineTemplates {
		capiYamlObjects = append(capiYamlObjects, *vcdMachineTemplate)
	}
	for _, kubeadmConfigTemplate := range kubeadmConfigTemplates {
		capiYamlObjects = append(capiYamlObjects, *kubeadmConfigTemplate)
	}
	for _, kcp := range kcpList.Items {
		capiYamlObjects = append(capiYamlObjects, kcp)
	}
	for _, md := range mdList.Items {
		capiYamlObjects = append(capiYamlObjects, md)
	}
	return capiYamlObjects, nil
}

func getCapiYaml(ctx context.Context, cli client.Client, cluster clusterv1.Cluster, vcdCluster infrav1.VCDCluster) (string, error) {
	capiYamlObjects, err := getK8sClusterObjects(ctx, cli, cluster, vcdCluster)
	if err != nil {
		return "", fmt.Errorf("failed to get k8s objects related to cluster [%s]: [%v]", cluster.Name, err)
	}
	yamlObjects := make([]string, len(capiYamlObjects))
	for idx, obj := range capiYamlObjects {
		yamlString, err := yamlWithoutStatus(obj)
		if err != nil {
			return "", fmt.Errorf("failed to convert object to yaml: [%v]", err)
		}
		yamlObjects[idx] = yamlString
	}

	return strings.Join(yamlObjects, "---\n"), nil

}

func getCapiStatusYaml(ctx context.Context, cli client.Client, cluster clusterv1.Cluster, vcdCluster infrav1.VCDCluster) (string, error) {
	capiYamlObjects, err := getK8sClusterObjects(ctx, cli, cluster, vcdCluster)
	if err != nil {
		return "", fmt.Errorf("failed to get k8s objects related to cluster [%s]: [%v]", cluster.Name, err)
	}
	yamlObjects := make([]string, len(capiYamlObjects))
	for idx, obj := range capiYamlObjects {
		yamlStatusString, err := getK8sObjectStatus(obj)
		if err != nil {
			return "", fmt.Errorf("failed to extract status from kuberenets object: [%v]", err)
		}
		yamlObjects[idx] = yamlStatusString
	}
	return strings.Join(yamlObjects, "---\n"), nil
}

func getUserCredentialsForCluster(ctx context.Context, cli client.Client, definedCreds infrav1.UserCredentialsContext) (infrav1.UserCredentialsContext, error) {
	username, password, refreshToken := definedCreds.Username, definedCreds.Password, definedCreds.RefreshToken
	if definedCreds.SecretRef != nil {
		secretNamespacedName := types.NamespacedName{
			Name:      definedCreds.SecretRef.Name,
			Namespace: definedCreds.SecretRef.Namespace,
		}
		userCredsSecret := &v1.Secret{}
		if err := cli.Get(ctx, secretNamespacedName, userCredsSecret); err != nil {
			return infrav1.UserCredentialsContext{}, errors.Wrapf(err, "error getting secret [%s] in namespace [%s]",
				secretNamespacedName.Name, secretNamespacedName.Namespace)
		}
		if b, exists := userCredsSecret.Data["username"]; exists {
			username = strings.TrimRight(string(b), "\n")
		}
		if b, exists := userCredsSecret.Data["password"]; exists {
			password = strings.TrimRight(string(b), "\n")
		}
		if b, exists := userCredsSecret.Data["refreshToken"]; exists {
			refreshToken = strings.TrimRight(string(b), "\n")
		}
	}
	userCredentials := infrav1.UserCredentialsContext{
		Username:     username,
		Password:     password,
		RefreshToken: refreshToken,
	}

	return userCredentials, nil
}
