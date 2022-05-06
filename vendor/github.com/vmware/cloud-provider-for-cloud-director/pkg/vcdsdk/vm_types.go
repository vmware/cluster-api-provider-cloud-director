package vcdsdk

import (
	"encoding/xml"
	"github.com/vmware/go-vcloud-director/v2/types/v56"
	"time"
)

type Vm struct {
	Xmlns                   string `xml:"xmlns,attr,omitempty"`
	Vmext                   string `xml:"vmext,attr,omitempty"`
	Ovf                     string `xml:"ovf,attr,omitempty"`
	Vssd                    string `xml:"vssd,attr,omitempty"`
	Common                  string `xml:"common,attr,omitempty"`
	Rasd                    string `xml:"rasd,attr,omitempty"`
	Vmw                     string `xml:"vmw,attr,omitempty"`
	Ovfenv                  string `xml:"ovfenv,attr,omitempty"`
	Ns9                     string `xml:"ns9,attr,omitempty"`
	NeedsCustomization      string `xml:"needsCustomization,attr,omitempty"`
	NestedHypervisorEnabled string `xml:"nestedHypervisorEnabled,attr,omitempty"`
	Deployed                string `xml:"deployed,attr,omitempty"`
	Status                  string `xml:"status,attr,omitempty"`
	Name                    string `xml:"name,attr,omitempty"`
	Id                      string `xml:"id,attr,omitempty"`
	Href                    string `xml:"href,attr,omitempty"`
	Type                    string `xml:"type,attr,omitempty"`

	Description                       string                             `xml:"Description,omitempty"`
	VmSpecSection                     *VmSpecSection                     `xml:"VmSpecSection,omitempty"`
	ExtraConfigVirtualHardwareSection *ExtraConfigVirtualHardwareSection `xml:"VirtualHardwareSection,omitempty"`
	NetworkConnectionSection          *NetworkConnectionSection          `xml:"NetworkConnectionSection,omitempty"`
}

type VmMarshal struct {
	XMLName xml.Name `xml:"Vm"`

	Xmlns                   string `xml:"xmlns,attr,omitempty"`
	Vmext                   string `xml:"xmlns:vmext,attr,omitempty"`
	Ovf                     string `xml:"xmlns:ovf,attr,omitempty"`
	Vssd                    string `xml:"xmlns:vssd,attr,omitempty"`
	Common                  string `xml:"xmlns:common,attr,omitempty"`
	Rasd                    string `xml:"xmlns:rasd,attr,omitempty"`
	Vmw                     string `xml:"xmlns:vmw,attr,omitempty"`
	Ovfenv                  string `xml:"xmlns:ovfenv,attr,omitempty"`
	Ns9                     string `xml:"xmlns:ns9,attr,omitempty"`
	NeedsCustomization      string `xml:"needsCustomization,attr,omitempty"`
	NestedHypervisorEnabled string `xml:"nestedHypervisorEnabled,attr,omitempty"`
	Deployed                string `xml:"deployed,attr,omitempty"`
	Status                  string `xml:"status,attr,omitempty"`
	Name                    string `xml:"name,attr,omitempty"`
	Id                      string `xml:"id,attr,omitempty"`
	Href                    string `xml:"href,attr,omitempty"`
	Type                    string `xml:"type,attr,omitempty"`

	Description              string                                    `xml:"Description,omitempty"`
	VmSpecSection            *VmSpecSectionMarshal                     `xml:"VmSpecSection,omitempty"`
	VirtualHardwareSection   *ExtraConfigVirtualHardwareSectionMarshal `xml:"ovf:VirtualHardwareSection,omitempty"`
	NetworkConnectionSection *NetworkConnectionSectionMarshal          `xml:"NetworkConnectionSection,omitempty"`
}

type ExtraConfigVirtualHardwareSection struct {
	XMLName xml.Name `xml:"VirtualHardwareSection"`
	Xmlns   string   `xml:"vcloud,attr,omitempty"`
	NS10    string   `xml:"ns10,attr,omitempty"`

	Info string `xml:"Info"`
	HREF string `xml:"href,attr,omitempty"`
	Type string `xml:"type,attr,omitempty"`

	Items        []*VirtualHardwareItem `xml:"Item,omitempty"`
	ExtraConfigs []*ExtraConfig         `xml:"ExtraConfig,omitempty"`
	Links        types.LinkList         `xml:"Link,omitempty"`
}

type ExtraConfigVirtualHardwareSectionMarshal struct {
	NS10 string `xml:"xmlns:ns10,attr,omitempty"`

	Info         string                        `xml:"ovf:Info"`
	Items        []*VirtualHardwareItemMarshal `xml:"ovf:Item,omitempty"`
	ExtraConfigs []*ExtraConfigMarshal         `xml:"vmw:ExtraConfig,omitempty"`
}

type VirtualHardwareConnectionMarshal struct {
	IpAddressingMode  string `xml:"ns10:ipAddressingMode,attr,omitempty"`
	IPAddress         string `xml:"ns10:ipAddress,attr,omitempty"`
	PrimaryConnection bool   `xml:"ns10:primaryNetworkConnection,attr,omitempty"`
	Value             string `xml:",chardata"`
}

type VirtualHardwareHostResource struct {
	BusType           int    `xml:"busType,attr,omitempty"`
	BusSubType        string `xml:"busSubType,attr,omitempty"`
	Capacity          int    `xml:"capacity,attr,omitempty"`
	StorageProfile    string `xml:"storageProfileHref,attr,omitempty"`
	OverrideVmDefault string `xml:"storageProfileOverrideVmDefault,attr,omitempty"`
	Disk              string `xml:"disk,attr,omitempty"`
	Iops              string `xml:"iops,attr,omitempty"`
	//OsType            string `xml:"osType,attr,omitempty"`
}

type VirtualHardwareHostResourceMarshal struct {
	StorageProfile    string `xml:"ns10:storageProfileHref,attr,omitempty"`
	BusType           int    `xml:"ns10:busType,attr,omitempty"`
	BusSubType        string `xml:"ns10:busSubType,attr,omitempty"`
	Capacity          int    `xml:"ns10:capacity,attr,omitempty"`
	Iops              string `xml:"ns10:iops,attr,omitempty"`
	OverrideVmDefault string `xml:"ns10:storageProfileOverrideVmDefault,attr,omitempty"`
}

type ExtraConfig struct {
	Key      string `xml:"key,attr"`
	Value    string `xml:"value,attr"`
	Required bool   `xml:"required,attr"`
}

type ExtraConfigMarshal struct {
	Key      string `xml:"vmw:key,attr"`
	Value    string `xml:"vmw:value,attr"`
	Required bool   `xml:"ovf:required,attr"`
}

type NetworkConnection struct {
	Network            string `xml:"network,attr"`            // Name of the network to which this NIC is connected.
	NeedsCustomization bool   `xml:"needsCustomization,attr"` // True if this NIC needs customization.

	NetworkConnectionIndex           int    `xml:"NetworkConnectionIndex"` // Virtual slot number associated with this NIC. First slot number is 0.
	IPAddress                        string `xml:"IpAddress,omitempty"`    // IP address assigned to this NIC.
	IpType                           string `xml:"IpType,omitempty"`
	IsConnected                      bool   `xml:"IsConnected"`             // If the virtual machine is undeployed, this value specifies whether the NIC should be connected upon deployment. If the virtual machine is deployed, this value reports the current status of this NIC's connection, and can be updated to change that connection status.
	MACAddress                       string `xml:"MACAddress,omitempty"`    // MAC address associated with the NIC.
	IPAddressAllocationMode          string `xml:"IpAddressAllocationMode"` // IP address allocation mode for this connection. One of: POOL (A static IP address is allocated automatically from a pool of addresses.) DHCP (The IP address is obtained from a DHCP service.) MANUAL (The IP address is assigned manually in the IpAddress element.) NONE (No IP addressing mode specified.)
	SecondaryIpAddressAllocationMode string `xml:"SecondaryIpAddressAllocationMode"`

	ExternalIPAddress  string `xml:"ExternalIpAddress,omitempty"` // If the network to which this NIC connects provides NAT services, the external address assigned to this NIC appears here.
	NetworkAdapterType string `xml:"NetworkAdapterType,omitempty"`
}

type NetworkConnectionSection struct {
	XMLName     xml.Name `xml:"NetworkConnectionSection"`
	Xmlns       string   `xml:"xmlns,attr,omitempty"`
	OvfRequired string   `xml:"required,attr,omitempty"`

	Info string `xml:"Info"`
	//
	HREF                          string               `xml:"href,attr,omitempty"`
	Type                          string               `xml:"type,attr,omitempty"`
	PrimaryNetworkConnectionIndex int                  `xml:"PrimaryNetworkConnectionIndex"`
	NetworkConnection             []*NetworkConnection `xml:"NetworkConnection,omitempty"`
	Link                          []*types.Link        `xml:"Link,omitempty"`
}

type NetworkConnectionSectionMarshal struct {
	XMLName     xml.Name `xml:"NetworkConnectionSection"`
	Xmlns       string   `xml:"xmlns,attr,omitempty"`
	HREF        string   `xml:"href,attr,omitempty"`
	Type        string   `xml:"type,attr,omitempty"`
	OvfRequired string   `xml:"ovf:required,attr,omitempty"`

	Info string `xml:"ovf:Info"`
	//
	PrimaryNetworkConnectionIndex int                  `xml:"PrimaryNetworkConnectionIndex"`
	NetworkConnection             []*NetworkConnection `xml:"NetworkConnection,omitempty"`
	Link                          []*types.Link        `xml:"Link,omitempty"`
}

type CoresPerSocket struct {
	OvfRequired string `xml:"required,attr,omitempty"`
	Value       string `xml:",chardata"`
}

type CoresPerSocketMarshal struct {
	OvfRequired string `xml:"ovf:required,attr,omitempty"`
	Value       string `xml:",chardata"`
}

type VirtualHardwareItem struct {
	XMLName xml.Name `xml:"Item"`
	Type    string   `xml:"type,attr,omitempty"`
	Href    string   `xml:"href,attr,omitempty"`

	Address               *NillableElement                   `xml:"Address"`
	AddressOnParent       *NillableElement                   `xml:"AddressOnParent"`
	AllocationUnits       *NillableElement                   `xml:"AllocationUnits"`
	AutomaticAllocation   *NillableElement                   `xml:"AutomaticAllocation"`
	AutomaticDeallocation *NillableElement                   `xml:"AutomaticDeallocation"`
	ConfigurationName     *NillableElement                   `xml:"ConfigurationName"`
	Connection            []*types.VirtualHardwareConnection `xml:"Connection,omitempty"`
	ConsumerVisibility    *NillableElement                   `xml:"ConsumerVisibility"`
	Description           *NillableElement                   `xml:"Description"`
	ElementName           *NillableElement                   `xml:"ElementName"`
	Generation            *NillableElement                   `xml:"Generation"`
	HostResource          []*VirtualHardwareHostResource     `xml:"HostResource,omitempty"`
	InstanceID            int                                `xml:"InstanceID,omitempty"`
	Limit                 *NillableElement                   `xml:"Limit"`
	MappingBehavior       *NillableElement                   `xml:"MappingBehavior"`
	OtherResourceType     *NillableElement                   `xml:"OtherResourceType"`
	Parent                *NillableElement                   `xml:"Parent"`
	PoolID                *NillableElement                   `xml:"PoolID"`
	Reservation           *NillableElement                   `xml:"Reservation"`
	ResourceSubType       *NillableElement                   `xml:"ResourceSubType"`
	ResourceType          *NillableElementMarshal            `xml:"ResourceType"`
	VirtualQuantity       *NillableElement                   `xml:"VirtualQuantity"`
	VirtualQuantityUnits  *NillableElement                   `xml:"VirtualQuantityUnits"`
	Weight                *NillableElement                   `xml:"Weight"`

	CoresPerSocket *CoresPerSocket `xml:"CoresPerSocket,omitempty"`
	Link           []*types.Link   `xml:"Link,omitempty"`
}

type VirtualHardwareItemMarshal struct {
	XMLName xml.Name `xml:"ovf:Item"`
	Type    string   `xml:"ns10:type,attr,omitempty"`
	Href    string   `xml:"ns10:href,attr,omitempty"`

	Address               *NillableElementMarshal               `xml:"rasd:Address"`
	AddressOnParent       *NillableElementMarshal               `xml:"rasd:AddressOnParent"`
	AllocationUnits       *NillableElementMarshal               `xml:"rasd:AllocationUnits"`
	AutomaticAllocation   *NillableElementMarshal               `xml:"rasd:AutomaticAllocation"`
	AutomaticDeallocation *NillableElementMarshal               `xml:"rasd:AutomaticDeallocation"`
	ConfigurationName     *NillableElementMarshal               `xml:"rasd:ConfigurationName"`
	Connection            []*VirtualHardwareConnectionMarshal   `xml:"rasd:Connection,omitempty"`
	ConsumerVisibility    *NillableElementMarshal               `xml:"rasd:ConsumerVisibility"`
	Description           *NillableElementMarshal               `xml:"rasd:Description"`
	ElementName           *NillableElementMarshal               `xml:"rasd:ElementName,omitempty"`
	Generation            *NillableElementMarshal               `xml:"rasd:Generation"`
	HostResource          []*VirtualHardwareHostResourceMarshal `xml:"rasd:HostResource,omitempty"`
	InstanceID            int                                   `xml:"rasd:InstanceID"`
	Limit                 *NillableElementMarshal               `xml:"rasd:Limit"`
	MappingBehavior       *NillableElementMarshal               `xml:"rasd:MappingBehavior"`
	OtherResourceType     *NillableElementMarshal               `xml:"rasd:OtherResourceType"`
	Parent                *NillableElementMarshal               `xml:"rasd:Parent"`
	PoolID                *NillableElementMarshal               `xml:"rasd:PoolID"`
	Reservation           *NillableElementMarshal               `xml:"rasd:Reservation"`
	ResourceSubType       *NillableElementMarshal               `xml:"rasd:ResourceSubType"`
	ResourceType          *NillableElementMarshal               `xml:"rasd:ResourceType"`
	VirtualQuantity       *NillableElementMarshal               `xml:"rasd:VirtualQuantity"`
	VirtualQuantityUnits  *NillableElementMarshal               `xml:"rasd:VirtualQuantityUnits"`
	Weight                *NillableElementMarshal               `xml:"rasd:Weight"`

	CoresPerSocket *CoresPerSocketMarshal `xml:"vmw:CoresPerSocket,omitempty"`
	Link           []*types.Link          `xml:"Link,omitempty"`
}

type NillableElement struct {
	XmlnsXsi string `xml:"xsi,attr,omitempty"`
	XsiNil   bool   `xml:"xsi:nil,attr,omitempty"`
	Value    string `xml:",chardata"`
}

type NillableElementMarshal struct {
	XmlnsXsi string `xml:"xmlns:xsi,attr,omitempty"`
	XsiNil   string `xml:"xsi:nil,attr,omitempty"`
	Value    string `xml:",chardata"`
}

type VmSpecSection struct { //TODO: check
	Modified          *bool                   `xml:"Modified,attr,omitempty"`
	Info              string                  `xml:"Info"`
	OsType            string                  `xml:"OsType,omitempty"`            // The type of the OS. This parameter may be omitted when using the VmSpec to update the contents of an existing VM.
	NumCpus           *int                    `xml:"NumCpus,omitempty"`           // Number of CPUs. This parameter may be omitted when using the VmSpec to update the contents of an existing VM.
	NumCoresPerSocket *int                    `xml:"NumCoresPerSocket,omitempty"` // Number of cores among which to distribute CPUs in this virtual machine. This parameter may be omitted when using the VmSpec to update the contents of an existing VM.
	CpuResourceMhz    *types.CpuResourceMhz   `xml:"CpuResourceMhz,omitempty"`    // CPU compute resources. This parameter may be omitted when using the VmSpec to update the contents of an existing VM.
	MemoryResourceMb  *types.MemoryResourceMb `xml:"MemoryResourceMb"`            // Memory compute resources. This parameter may be omitted when using the VmSpec to update the contents of an existing VM.
	MediaSection      *types.MediaSection     `xml:"MediaSection,omitempty"`      // The media devices of this VM.
	DiskSection       *DiskSection            `xml:"DiskSection,omitempty"`       // virtual disks of this VM.
	HardwareVersion   *types.HardwareVersion  `xml:"HardwareVersion"`             // vSphere name of Virtual Hardware Version of this VM. Example: vmx-13 - This parameter may be omitted when using the VmSpec to update the contents of an existing VM.
	VmToolsVersion    string                  `xml:"VmToolsVersion,omitempty"`    // VMware tools version of this VM.
	VirtualCpuType    string                  `xml:"VirtualCpuType,omitempty"`    // The capabilities settings for this VM. This parameter may be omitted when using the VmSpec to update the contents of an existing VM.
	TimeSyncWithHost  *bool                   `xml:"TimeSyncWithHost,omitempty"`  // Synchronize the VM's time with the host.
}

type VmSpecSectionMarshal struct { //TODO: check
	Modified          *bool                   `xml:"Modified,attr,omitempty"`
	Info              string                  `xml:"ovf:Info"`
	OsType            string                  `xml:"OsType,omitempty"`            // The type of the OS. This parameter may be omitted when using the VmSpec to update the contents of an existing VM.
	NumCpus           *int                    `xml:"NumCpus,omitempty"`           // Number of CPUs. This parameter may be omitted when using the VmSpec to update the contents of an existing VM.
	NumCoresPerSocket *int                    `xml:"NumCoresPerSocket,omitempty"` // Number of cores among which to distribute CPUs in this virtual machine. This parameter may be omitted when using the VmSpec to update the contents of an existing VM.
	CpuResourceMhz    *types.CpuResourceMhz   `xml:"CpuResourceMhz,omitempty"`    // CPU compute resources. This parameter may be omitted when using the VmSpec to update the contents of an existing VM.
	MemoryResourceMb  *types.MemoryResourceMb `xml:"MemoryResourceMb"`            // Memory compute resources. This parameter may be omitted when using the VmSpec to update the contents of an existing VM.
	MediaSection      *types.MediaSection     `xml:"MediaSection,omitempty"`      // The media devices of this VM.
	DiskSection       *DiskSection            `xml:"DiskSection,omitempty"`       // virtual disks of this VM.
	HardwareVersion   *types.HardwareVersion  `xml:"HardwareVersion"`             // vSphere name of Virtual Hardware Version of this VM. Example: vmx-13 - This parameter may be omitted when using the VmSpec to update the contents of an existing VM.
	VmToolsVersion    string                  `xml:"VmToolsVersion,omitempty"`    // VMware tools version of this VM.
	VirtualCpuType    string                  `xml:"VirtualCpuType,omitempty"`    // The capabilities settings for this VM. This parameter may be omitted when using the VmSpec to update the contents of an existing VM.
	TimeSyncWithHost  *bool                   `xml:"TimeSyncWithHost,omitempty"`  // Synchronize the VM's time with the host.
}

type DiskSection struct {
	DiskSettings []*DiskSettings `xml:"DiskSettings"`
}

type DiskSettings struct {
	DiskId              string           `xml:"DiskId,omitempty"`              // Specifies a unique identifier for this disk in the scope of the corresponding VM. This element is optional when creating a VM, but if it is provided it should be unique. This element is mandatory when updating an existing disk.
	SizeMb              int64            `xml:"SizeMb"`                        // The size of the disk in MB.
	UnitNumber          int              `xml:"UnitNumber"`                    // The device number on the SCSI or IDE controller of the disk.
	BusNumber           int              `xml:"BusNumber"`                     //	The number of the SCSI or IDE controller itself.
	AdapterType         string           `xml:"AdapterType"`                   // The type of disk controller, e.g. IDE vs SCSI and if SCSI bus-logic vs LSI logic.
	ThinProvisioned     *bool            `xml:"ThinProvisioned,omitempty"`     // Specifies whether the disk storage is pre-allocated or allocated on demand.
	Disk                *types.Reference `xml:"Disk,omitempty"`                // Specifies reference to a named disk.
	StorageProfile      *types.Reference `xml:"StorageProfile,omitempty"`      // Specifies reference to a storage profile to be associated with the disk.
	OverrideVmDefault   bool             `xml:"overrideVmDefault"`             // Specifies that the disk storage profile overrides the VM's default storage profile.
	Iops                *int64           `xml:"iops,omitempty"`                // Specifies the IOPS for the disk.
	VirtualQuantity     *int64           `xml:"VirtualQuantity,omitempty"`     // The actual size of the disk.
	VirtualQuantityUnit string           `xml:"VirtualQuantityUnit,omitempty"` // The units in which VirtualQuantity is measured.
	Resizable           bool             `xml:"resizable"`
	Shareable           bool             `xml:"shareable"`
	SharingType         string           `xml:"sharingType"`
}

// TODO vcdtypes from CAPVCD

// Type for a single disk query result in records format.
// Reference: vCloud API 35.0 - QueryResultDiskRecordType
// https://code.vmware.com/apis/1046/vmware-cloud-director/doc/doc/types/QueryResultDiskRecordType.html
type DiskRecordType struct {
	HREF               string          `xml:"href,attr,omitempty"`
	Id                 string          `xml:"id,attr,omitempty"`
	Type               string          `xml:"type,attr,omitempty"`
	Name               string          `xml:"name,attr,omitempty"`
	Vdc                string          `xml:"vdc,attr,omitempty"`
	Description        string          `xml:"description,attr,omitempty"`
	SizeMB             int64           `xml:"sizeMb,attr,omitempty"`
	Iops               int64           `xml:"iops,attr,omitempty"`
	Encrypted          bool            `xml:"encrypted,attr,omitempty"`
	DataStore          string          `xml:"dataStore,attr,omitempty"`
	DataStoreName      string          `xml:"datastoreName,attr,omitempty"`
	OwnerName          string          `xml:"ownerName,attr,omitempty"`
	VdcName            string          `xml:"vdcName,attr,omitempty"`
	Task               string          `xml:"task,attr,omitempty"`
	StorageProfile     string          `xml:"storageProfile,attr,omitempty"`
	StorageProfileName string          `xml:"storageProfileName,attr,omitempty"`
	Status             string          `xml:"status,attr,omitempty"`
	BusType            string          `xml:"busType,attr,omitempty"`
	BusTypeDesc        string          `xml:"busTypeDesc,attr,omitempty"`
	BusSubType         string          `xml:"busSubType,attr,omitempty"`
	AttachedVmCount    int32           `xml:"attachedVmCount,attr,omitempty"`
	IsAttached         bool            `xml:"isAttached,attr,omitempty"`
	IsShareable        bool            `xml:"isShareable,attr,omitempty"`
	Link               []*types.Link   `xml:"Link,omitempty"`
	Metadata           *types.Metadata `xml:"Metadata,omitempty"`
}

// Represents an independent disk
// Reference: vCloud API 35.0 - DiskType
// https://code.vmware.com/apis/287/vcloud?h=Director#/doc/doc/types/DiskType.html
type Disk struct {
	HREF         string `xml:"href,attr,omitempty"`
	Type         string `xml:"type,attr,omitempty"`
	Id           string `xml:"id,attr,omitempty"`
	OperationKey string `xml:"operationKey,attr,omitempty"`
	Name         string `xml:"name,attr"`
	Status       int    `xml:"status,attr,omitempty"`
	SizeMb       int64  `xml:"sizeMb,attr"`
	Iops         int64  `xml:"iops,attr,omitempty"`
	Encrypted    bool   `xml:"encrypted,attr,omitempty"`
	BusType      string `xml:"busType,attr,omitempty"`
	BusSubType   string `xml:"busSubType,attr,omitempty"`
	Shareable    bool   `xml:"shareable,attr,omitempty"`

	Description     string                 `xml:"Description,omitempty"`
	Files           *types.FilesList       `xml:"Files,omitempty"`
	Link            []*types.Link          `xml:"Link,omitempty"`
	Owner           *types.Owner           `xml:"Owner,omitempty"`
	StorageProfile  *types.Reference       `xml:"StorageProfile,omitempty"`
	Tasks           *types.TasksInProgress `xml:"Tasks,omitempty"`
	VCloudExtension *types.VCloudExtension `xml:"VCloudExtension,omitempty"`
}

// DiskCreateParams Parameters for creating or updating an independent disk.
// Reference: vCloud API 35.0 - DiskCreateParamsType
// https://code.vmware.com/apis/1046/vmware-cloud-director/doc/doc/types/DiskCreateParamsType.html
type DiskCreateParams struct {
	XMLName         xml.Name               `xml:"DiskCreateParams"`
	Xmlns           string                 `xml:"xmlns,attr,omitempty"`
	Disk            *Disk                  `xml:"Disk"`
	Locality        *types.Reference       `xml:"Locality,omitempty"`
	VCloudExtension *types.VCloudExtension `xml:"VCloudExtension,omitempty"`
}

type QueryResultOrgVdcRecordType struct {
	Xmlns                          string `xml:"xmlns,attr,omitempty"`
	HREF                           string `xml:"href,attr,omitempty"`
	ID                             string `xml:"id,attr,omitempty"`
	Type                           string `xml:"type,attr,omitempty"`
	Name                           string `xml:"name,attr,omitempty"`
	Description                    string `xml:"description,attr,omitempty"`
	ComputeProviderScope           string `xml:"computeProviderScope,attr,omitempty"`
	NetworkProviderScope           string `xml:"networkProviderScope,attr,omitempty"`
	IsEnabled                      bool   `xml:"isEnabled,attr,omitempty"`
	CpuAllocationMhz               int64  `xml:"cpuAllocationMhz,attr,omitempty"`
	CpuLimitMhz                    int64  `xml:"cpuLimitMhz,attr,omitempty"`
	CpuUsedMhz                     int64  `xml:"cpuUsedMhz,attr,omitempty"`
	CpuReservedMhz                 int64  `xml:"cpuReservedMhz,attr,omitempty"`
	MemoryAllocationMB             int64  `xml:"memoryAllocationMB,attr,omitempty"`
	MemoryLimitMB                  int64  `xml:"memoryLimitMB,attr,omitempty"`
	MemoryUsedMB                   int64  `xml:"memoryUsedMB,attr,omitempty"`
	MemoryReservedMB               int64  `xml:"memoryReservedMB,attr,omitempty"`
	StorageLimitMB                 int64  `xml:"storageLimitMB,attr,omitempty"`
	StorageUsedMB                  int64  `xml:"storageUsedMB,attr,omitempty"`
	ProviderVdcName                string `xml:"providerVdcName,attr,omitempty"`
	ProviderVdc                    string `xml:"providerVdc,attr,omitempty"`
	OrgName                        string `xml:"orgName,attr,omitempty"`
	NumberOfVApps                  int32  `xml:"numberOfVApps,attr,omitempty"`
	NumberOfUnmanagedVApps         int32  `xml:"numberOfUnmanagedVApps,attr,omitempty"`
	NumberOfMedia                  int32  `xml:"numberOfMedia,attr,omitempty"`
	NumberOfDisks                  int32  `xml:"numberOfDisks,attr,omitempty"`
	NumberOfVAppTemplates          int32  `xml:"numberOfVAppTemplates,attr,omitempty"`
	IsBusy                         bool   `xml:"isBusy,attr,omitempty"`
	Status                         string `xml:"status,attr,omitempty"`
	NumberOfDatastores             int32  `xml:"numberOfDatastores,attr,omitempty"`
	NumberOfStorageProfiles        int32  `xml:"numberOfStorageProfiles,attr,omitempty"`
	NumberOfVMs                    int32  `xml:"numberOfVMs,attr,omitempty"`
	NumberOfRunningVMs             int32  `xml:"numberOfRunningVMs,attr,omitempty"`
	NetworkPoolUniversalId         string `xml:"networkPoolUniversalId,attr,omitempty"`
	NumberOfDeployedVApps          int32  `xml:"numberOfDeployedVApps,attr,omitempty"`
	NumberOfDeployedUnmanagedVApps int32  `xml:"numberOfDeployedUnmanagedVApps,attr,omitempty"`
	IsThinProvisioned              bool   `xml:"isThinProvisioned,attr,omitempty"`
	IsFastProvisioned              bool   `xml:"isFastProvisioned,attr,omitempty"`
}

type QueryResultCatalogRecordType struct {
	Xmlns                 string    `xml:"xmlns,attr,omitempty"`
	HREF                  string    `xml:"href,attr,omitempty"`
	ID                    string    `xml:"id,attr,omitempty"`
	Type                  string    `xml:"type,attr,omitempty"`
	Name                  string    `xml:"name,attr,omitempty"`
	IsPublished           string    `xml:"isPublished,attr,omitempty"`
	IsShared              bool      `xml:"isShared,attr,omitempty"`
	CreationDate          time.Time `xml:"creationDate,attr,omitempty"`
	OrgName               string    `xml:"orgName,attr,omitempty"`
	OwnerName             string    `xml:"ownerName,attr,omitempty"`
	NumberOfVAppTemplates int32     `xml:"numberOfVAppTemplates,attr,omitempty"`
	NumberOfMedia         int32     `xml:"numberOfMedia,attr,omitempty"`
	Owner                 string    `xml:"owner,attr,omitempty"`
}

type QueryResultRecordsType struct {
	Xmlns string `xml:"xmlns,attr,omitempty"`
	// Attributes
	HREF     string  `xml:"href,attr,omitempty"`     // The URI of the entity.
	Type     string  `xml:"type,attr,omitempty"`     // The MIME type of the entity.
	Name     string  `xml:"name,attr,omitempty"`     // The name of the entity.
	Page     int32   `xml:"page,attr,omitempty"`     // Page of the result set that this container holds. The first page is page number 1.
	PageSize int32   `xml:"pageSize,attr,omitempty"` // Page size, as a number of records or references.
	Total    float64 `xml:"total,attr,omitempty"`    // Total number of records or references in the container.
	// Elements
	Link          []*types.Link                   `xml:"Link,omitempty"`          // A reference to an entity or operation associated with this object.
	OrgVdcRecord  []*QueryResultOrgVdcRecordType  `xml:"OrgVdcRecord"`            // A record representing storage profiles
	CatalogRecord []*QueryResultCatalogRecordType `xml:"CatalogRecord,omitempty"` // A record representing Catalog records
}

// Represents a list of virtual machines
// Reference: vCloud API 35.0 - VmsType
// https://code.vmware.com/apis/1046/vmware-cloud-director/doc/doc/types/VmsType.html
type Vms struct {
	XMLName xml.Name `xml:"Vms"`
	Xmlns   string   `xml:"xmlns,attr,omitempty"`

	HREF string `xml:"href,attr"`
	Type string `xml:"type,attr"`

	// Elements
	Link            []*types.Link          `xml:"Link,omitempty"`
	VCloudExtension *types.VCloudExtension `xml:"VCloudExtension,omitempty"`
	VmReference     []*types.Reference     `xml:"VmReference,omitempty"`
}

type ComposeVAppWithVMs struct {
	XMLName xml.Name `xml:"ComposeVAppParams"`
	Ovf     string   `xml:"xmlns:ovf,attr"`
	Xsi     string   `xml:"xmlns:xsi,attr"`
	Xmlns   string   `xml:"xmlns,attr"`
	// Attributes
	Name        string `xml:"name,attr,omitempty"`        // Typically used to name or identify the subject of the request. For example, the name of the object being created or modified.
	Deploy      bool   `xml:"deploy,attr"`                // True if the vApp should be deployed at instantiation. Defaults to true.
	PowerOn     bool   `xml:"powerOn,attr"`               // True if the vApp should be powered-on at instantiation. Defaults to true.
	LinkedClone bool   `xml:"linkedClone,attr,omitempty"` // Reserved. Unimplemented.
	// Elements
	Description         string                               `xml:"Description,omitempty"`         // Optional description.
	VAppParent          *types.Reference                     `xml:"VAppParent,omitempty"`          // Reserved. Unimplemented.
	InstantiationParams *types.InstantiationParams           `xml:"InstantiationParams,omitempty"` // Instantiation parameters for the composed vApp.
	SourcedItemList     []*types.SourcedCompositionItemParam `xml:"SourcedItem,omitempty"`         // Composition item. One of: vApp vAppTemplate VM.
	AllEULAsAccepted    bool                                 `xml:"AllEULAsAccepted,omitempty"`    // True confirms acceptance of all EULAs in a vApp template. Instantiation fails if this element is missing, empty, or set to false and one or more EulaSection elements are present.
}
