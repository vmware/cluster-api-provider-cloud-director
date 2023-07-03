package testingsdk

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"github.com/vmware/cloud-provider-for-cloud-director/pkg/vcdsdk"
	swagger "github.com/vmware/cloud-provider-for-cloud-director/pkg/vcdswaggerclient"
	"io"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	k8syaml "k8s.io/apimachinery/pkg/util/yaml"
)

const (
	MachineDeployment     = "MachineDeployment"
	KubeadmConfigTemplate = "KubeadmConfigTemplate"
	VCDMachineTemplate    = "VCDMachineTemplate"
)

func GetCapvcdYamlFromRde(capvcdRDE swagger.DefinedEntity) (string, error) {
	specIf, ok := capvcdRDE.Entity["spec"]
	if !ok {
		return "", fmt.Errorf("unable to get spec field from capvcd RDE [%s]", capvcdRDE.Id)
	}

	spec, ok := specIf.(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("unable to convert spec of capvcd rde [%s] from type [%T] to map[string]interface{}",
			capvcdRDE.Id, specIf)
	}

	capiYamlIf, ok := spec["capiYaml"]
	if !ok {
		return "", fmt.Errorf("`capiYaml` field not found in spec for capvcd RDE [%s]", capvcdRDE.Id)
	}

	capiYaml, ok := capiYamlIf.(string)
	if !ok {
		return "", fmt.Errorf("unable to convert `capiYaml` of capvcd rde [%s] from type [%T] to string",
			capvcdRDE.Id, specIf)
	}

	return capiYaml, nil
}

func GetMapBySpecName(specMap map[string]interface{}, specName string, sectionName string) (map[string]interface{}, error) {
	entity, ok := specMap[specName]
	if !ok {
		return nil, fmt.Errorf("unable to get map: [%s] from %s in capiYaml String\n", specName, sectionName)
	}
	updatedSpecMap, ok := entity.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("unable to convert [%T] to map[string]interface{}", entity)
	}
	return updatedSpecMap, nil
}

func getWorkerNodePoolObjectBase(ctx context.Context, client *vcdsdk.Client, clusterId string, objectType string) (map[string]interface{}, error) {
	rde, err := getRdeById(ctx, client, clusterId)
	if err != nil {
		return nil, fmt.Errorf("unable to get defined entity by clusterId [%s]: [%v]", clusterId, err)
	}

	capiYaml, err := GetCapvcdYamlFromRde(*rde)
	if err != nil {
		return nil, fmt.Errorf("unable to get capiyaml from RDE: [%v]", err)
	}

	hundredKB := 100 * 1024
	err = nil
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
		switch kind {
		case objectType:
			return unstructuredObj.Object, nil
		}
	}
	return nil, fmt.Errorf("unable to find any [%s] entities in capiyaml", objectType)
}

func CreateNewVCDMachineTemplate(ctx context.Context, client *vcdsdk.Client, clusterId string, nodePoolName string) (map[string]interface{}, error) {
	newVcdMachineTemplate, err := getWorkerNodePoolObjectBase(ctx, client, clusterId, VCDMachineTemplate)
	if err != nil {
		return nil, fmt.Errorf("unable to get VCDMachineTemplateBase: [%v]", err)
	}

	metadataIf, ok := newVcdMachineTemplate["metadata"]
	if !ok {
		return nil, fmt.Errorf("VcdMachineTemplate base has no metadata attribute")
	}
	metadata, ok := metadataIf.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("could not convert %s.metadata to map", VCDMachineTemplate)
	}
	metadata["name"] = nodePoolName

	newVcdMachineTemplate["metadata"] = metadata
	return newVcdMachineTemplate, nil
}

func CreateNewKubeadmConfigTemplate(ctx context.Context, client *vcdsdk.Client, clusterId string, nodePoolName string) (map[string]interface{}, error) {
	newKubeadmConfigTemplate, err := getWorkerNodePoolObjectBase(ctx, client, clusterId, KubeadmConfigTemplate)
	if err != nil {
		return nil, fmt.Errorf("unable to get KubeadmConfigTemplate: [%v]", err)
	}

	metadataIf, ok := newKubeadmConfigTemplate["metadata"]
	if !ok {
		return nil, fmt.Errorf("KubeadmConfigTemplate base has no metadata attribute")
	}
	metadata, ok := metadataIf.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("could not convert %s.metadata to map", VCDMachineTemplate)
	}
	metadata["name"] = nodePoolName
	newKubeadmConfigTemplate["metadata"] = metadata
	return newKubeadmConfigTemplate, nil
}

func CreateNewMachineDeployment(ctx context.Context, client *vcdsdk.Client, clusterId string, nodePoolName string, nodePoolSize int64) (map[string]interface{}, error) {
	newMachineDeployment, err := getWorkerNodePoolObjectBase(ctx, client, clusterId, MachineDeployment)
	if err != nil {
		return nil, fmt.Errorf("unable to get MachineDeployment: [%v]", err)
	}

	// change node pool name in metadata
	metadataIf, ok := newMachineDeployment["metadata"]
	if !ok {
		return nil, fmt.Errorf("MachineDeployment base has no metadata attribute")
	}
	metadata, ok := metadataIf.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("could not convert %s.metadata to map", VCDMachineTemplate)
	}
	metadata["name"] = nodePoolName
	newMachineDeployment["metadata"] = metadata

	// change node pool name in spec.template.spec.bootstrap.configRef.name
	specMap, err := GetMapBySpecName(newMachineDeployment, "spec", MachineDeployment)
	if err != nil {
		return nil, fmt.Errorf("error converting %s.spec to map: [%v]", MachineDeployment, err)
	}

	templateIf, ok := specMap["template"]
	if !ok {
		return nil, fmt.Errorf("specMap has no template attribute")
	}
	template, ok := templateIf.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("unable to convert spec.template to map")
	}
	spec2If, ok := template["spec"]
	if !ok {
		return nil, fmt.Errorf("spec.template has no spec attribute")
	}
	spec2, ok := spec2If.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("unable to convert spec.template.spec to map")
	}
	bootstrapIf, ok := spec2["bootstrap"]
	if !ok {
		return nil, fmt.Errorf("spec.template.spec has no bootstrap attribute")
	}
	bootstrap, ok := bootstrapIf.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("unable to convert spec.template.spec.bootstrap to map")
	}
	configRefIf, ok := bootstrap["configRef"]
	if !ok {
		return nil, fmt.Errorf("spec.template.spec.bootstrap has no configRef attribute")
	}
	configRef, ok := configRefIf.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("unable to convert spec.template.spec.bootstrap.configRef to map")
	}
	configRef["name"] = nodePoolName

	// change node pool name in spec.template.spec.infrastructureRef.name
	infrastructureRefIf, ok := spec2["infrastructureRef"]
	if !ok {
		return nil, fmt.Errorf("spec.template.spec has no infrastructureRef attribute")
	}
	infrastructureRef, ok := infrastructureRefIf.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("unable to convert spec.template.spec.infrastructureRef to map")
	}
	infrastructureRef["name"] = nodePoolName

	// propagating changes up
	// also change spec.replicas to nodePoolSize
	bootstrap["configRef"] = configRef
	spec2["bootstrap"] = bootstrap
	spec2["infrastructureRef"] = infrastructureRef
	template["spec"] = spec2
	specMap["template"] = template
	specMap["replicas"] = nodePoolSize
	newMachineDeployment["spec"] = specMap

	return newMachineDeployment, nil
}
