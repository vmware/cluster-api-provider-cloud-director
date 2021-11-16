package vcdtypes

import (
	"encoding/xml"
	"github.com/vmware/go-vcloud-director/v2/types/v56"
	"time"
)

const (
	UNRESOLVED = iota
	RESOLVED
	DEPLOYED
	SUSPENDED
	POWERED_ON
	WAITING_FOR_INPUT
	UNKNOWN
	UNRECOGNIZED
	POWERED_OFF
	INCONSISTENT_STATE
	MIXED
	DESCRIPTOR_PENDING
	COPYING_CONTENTS
	DISK_CONTENTS_PENDING
	QUARANTINED
	QUARANTINE_EXPIRED
	REJECTED
	TRANSFER_TIMEOUT
	VAPP_UNDEPLOYED
	VAPP_PARTIALLY_DEPLOYED
	PARTIALLY_POWERED_OFF
	PARTIALLY_SUSPENDED
	FAILED_CREATION = -1
)

// LinkList represents a list of links
type LinkList []*types.Link

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

//type VcdComputePolicy struct {
//
//}
