package controllers

import (
	"context"
	"fmt"
	"github.com/pkg/errors"
	infrav1 "github.com/vmware/cluster-api-provider-cloud-director/api/v1alpha4"
	"gopkg.in/yaml.v2"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha4"
	"sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1alpha4"
	kcpv1 "sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1alpha4"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"strings"
)

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

	if _, ok := objMap["typemeta"]; ok {
		typeMetaMap, ok := objMap["typemeta"].(map[interface{}]interface{})
		if !ok {
			return "", fmt.Errorf("failed to convert typemeta [%v] to map[interface{}]interface{}: [%v]", objMap["typemeta"], err)
		}
		for k, v := range typeMetaMap {
			objMap[k.(string)] = v
		}
		delete(objMap, "typemeta")
	}

	if _, ok := objMap["objectmeta"]; ok {
		objectMetaMap, ok := objMap["objectmeta"].(map[interface{}]interface{})
		if !ok {
			return "", fmt.Errorf("failed to convert objectmeta [%v] to map[interface{}]interface{}: [%v]", objMap["objectmeta"], err)
		}
		for k, _ := range objectMetaMap {
			if k.(string) != "name" && k.(string) != "namespace" {
				delete(objectMetaMap, k)
			}
		}
		objMap["metadata"] = objectMetaMap
		delete(objMap, "objectmeta")
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

func getKubeadmConfigTemplateByObjRef(ctx context.Context, cli client.Client, objRef v1.ObjectReference) (*v1alpha4.KubeadmConfigTemplate, error) {
	kubeadmConfigTemplate := &v1alpha4.KubeadmConfigTemplate{}
	kubeadmConfigTemplateKey := client.ObjectKey{
		Namespace: objRef.Namespace,
		Name:      objRef.Name,
	}
	if err := cli.Get(ctx, kubeadmConfigTemplateKey, kubeadmConfigTemplate); err != nil {
		return nil, fmt.Errorf("failed to get KubeadmConfigTemplate by ObjectReference [%v]: [%v]", objRef, err)
	}

	return kubeadmConfigTemplate, nil
}

func getCapiYaml(ctx context.Context, cli client.Client, cluster clusterv1.Cluster, vcdCluster infrav1.VCDCluster) (string, error) {
	vcdCluster.Spec.UserCredentialsContext.Username = ""
	vcdCluster.Spec.UserCredentialsContext.Password = ""
	vcdCluster.Spec.UserCredentialsContext.RefreshToken = ""
	capiYamlObjects := []interface{}{
		cluster,
		vcdCluster,
	}

	kcpList, err := getAllKubeadmControlPlaneForCluster(ctx, cli, cluster)
	if err != nil {
		return "", fmt.Errorf("failed to get all KCPs from Cluster object: [%v]", err)
	}

	mdList, err := getAllMachineDeploymentsForCluster(ctx, cli, cluster)
	if err != nil {
		return "", fmt.Errorf("failed to get all the MachineDeployments from Cluster: [%v]", err)
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
			return "", fmt.Errorf("failed to get VCDMachineTemplate by ObjectReference [%v]: [%v]", objRef, err)
		}
		vcdMachineTemplates = append(vcdMachineTemplates, vcdMachineTemplate)
	}

	kubeadmConfigTemplates := make([]*v1alpha4.KubeadmConfigTemplate, 0)
	for _, objRef := range kubeadmConfigTemplateNameToObjRef {
		kubeadmConifgTemplate, err := getKubeadmConfigTemplateByObjRef(ctx, cli, *objRef)
		if err != nil {
			return "", fmt.Errorf("failed to get KubeadmConfigTemplate by ObjectReference [%v]: [%v]", objRef, err)
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
