package vcdsdk

import (
	"fmt"
	"gopkg.in/yaml.v2"
	"k8s.io/klog/v2"
	"os"
	"sync"
)

const (
	ZoneMapConfigFilePath = "/opt/vmware-cloud-director/ccm/vcloud-cse-zones.yaml"
)

type ZoneMap struct {
	rwLock        sync.RWMutex
	configMapFile string
	VdcToZoneMap  map[string]string
}

type Zone struct {
	Name     string `yaml:"name"`
	OVDCName string `yaml:"ovdcName"`
}

type ZoneConfigMap struct {
	ZoneType string `yaml:"zoneType"`
	Zones    []Zone `yaml:"zones"`
}

func NewZoneMap(configMapFile string) (*ZoneMap, error) {
	zm := &ZoneMap{
		configMapFile: configMapFile,
		VdcToZoneMap:  make(map[string]string),
	}

	if err := zm.ReloadZoneMap(); err != nil {
		return nil, fmt.Errorf("unable to load and parse configmap [%s]: [%v]", configMapFile, err)
	}

	return zm, nil
}

func (zm *ZoneMap) ReloadZoneMap() error {
	configFileReader, err := os.Open(zm.configMapFile)
	if err != nil {
		return fmt.Errorf("unable to open file [%s]: [%v]", zm.configMapFile, err)
	}
	defer func() {
		if err := configFileReader.Close(); err != nil {
			klog.Infof("unable to close config map file [%s]: [%v]", zm.configMapFile, err)
		}
	}()

	zoneConfigMap := &ZoneConfigMap{
		ZoneType: "",
	}
	decoder := yaml.NewDecoder(configFileReader)
	decoder.SetStrict(true)
	if err = decoder.Decode(&zoneConfigMap); err != nil {
		return fmt.Errorf("unable to decode zone configmap [%s]: [%v]", zm.configMapFile, err)
	}

	zm.rwLock.Lock()
	defer zm.rwLock.Unlock()

	for _, zone := range zoneConfigMap.Zones {
		zm.VdcToZoneMap[zone.OVDCName] = zone.Name
	}

	return nil
}
