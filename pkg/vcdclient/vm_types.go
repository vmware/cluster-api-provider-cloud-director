package vcdclient

import (
	"encoding/xml"
	"github.com/vmware/go-vcloud-director/v2/types/v56"
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
