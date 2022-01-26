# Cloud Director Setup

## Provider steps

### Avi controller, NSX-T Cloud Setup
The LoadBalancers fronting the Multimaster workload clusters will need a preconfigured Avi Controller, NSX-T Cloud and Avi Service Engine Group. This is a provider operation.
The Service Engine Group (SEG) should be created as Dedicated and one SEG should be allocated per Edge Gateway in order to ensure that Load Balancers used by Tenants are well-isolated from each other.
The LoadBalancer section of the Edge Gateway for a Tenant should be enabled, and the appropriate Service Engine Group(s) should be configured into the Edge Gateway. This will be used to create Virtual Services when a LoadBalancer request is made from Kubernetes.

### CAPVCD RDE registration and right bundles publishment
1. Bookkeeping of the CAPVCD based workload clusters is done through RDEs in VCD. Provider needs to [register the CAPVCD 
RDE schema with VCD](#capvcd_rde_schema).
2. Publish CAPVCD right bundle `vmware:capvcdCluster:1.0.0` to the tenant organizations
3. Ensure below rights are published to the chosen tenant organizations
    * User > Manage user's own API token
    * Organization VDC > Create a Shared Disk
    * vApp > Preserve all ExtraConfig Elements during OVA Import and Export
    * General > Manage Certificates Library

### Upload TKGm template
Upload the TKGm ovas using either VCD UI (or) `cse template upload` of CSE versions greater than or equal to 3.1.1

## Tenant Admin steps
* A ServiceEngineGroup needs to be added to the gateway of the OVDC within which the Kubernetes cluster is to be created. The overall steps to achieve that are documented at Enable Load Balancer on an NSX-T Data Center Edge Gateway
Create and publish the desired sizing policies on the chosen ovdc(s)
* Ensure the OVDC gateway has outbound access. If required, set an SNAT rule with the internal IP range of the VMs.
* Set up DNS on the desired virtual datacenter networks.
  <a name="user_role"></a>
* Tenant Admin Persona: Add the below mentioned rights to the Tenant administrator role. This persona will be used to 
  create the management cluster.
    * CAPVCD Cluster FullControl
    * User > Manage user's own API token
    * Organization VDC > Create a Shared Disk
    * vApp > Preserve all ExtraConfig Elements during OVA Import and Export
    * General > Manage Certificates Library
* Tenant User Persona: Create a role with the below mentioned rights in addition to the vApp Author privileges. Tenant users 
  eligible to deploy CAPVCD based workload clusters must be assigned with this role.
   * CAPVCD Cluster FullControl 
   * User > Manage user's own API token
   * Organization VDC > Create a Shared Disk
   * vApp > Preserve all ExtraConfig Elements during OVA Import and Export
   * General > Manage Certificates Library

<a name="capvcd_rde_schema"></a>
## Register CAPVCD RDE schema
Persona: Provider

Use one of the two ways to register CAPVCD RDE schema with VCD
1. CSE 3.1.2 can be used to register the schema 
    1. Update CSE config file with `register_capvcd_schema: True` under `service` section.
    2. Run either `cse install` (or) `cse upgrade`
2. Register the CAPVCD RDE schema through API call via Postman
   POST `https://<vcd>/cloudapi/1.0.0/entityTypes` with the below payload
```json
{
    "name": "CAPVCD Cluster",
    "description": "",
    "nss": "capvcdCluster",
    "version": "1.0.0",
    "inheritedVersion": null,
    "externalId": null,
    "schema": {
  "definitions": {
    "distribution": {
      "type": "object",
      "required": [
        "version"
      ],
      "properties": {
        "version": {
          "type": "string"
        }
      },
      "additionalProperties": true
    },
    "network": {
      "type": "object",
      "description": "The network-related settings for the cluster.",
      "properties": {
        "cni": {
          "type": "object",
          "description": "The CNI to use.",
          "properties": {
            "name": {
              "type": "string"
            }
          }
        },
        "pods": {
          "type": "object",
          "description": "The network settings for Kubernetes pods.",
          "properties": {
            "cidrBlocks": {
              "type": "array",
              "description": "Specifies a range of IP addresses to use for Kubernetes pods.",
              "items": {
                "type": "string"
              }
            }
          }
        },
        "services": {
          "type": "object",
          "description": "The network settings for Kubernetes services",
          "properties": {
            "cidrBlocks": {
              "type": "array",
              "description": "The range of IP addresses to use for Kubernetes services",
              "items": {
                "type": "string"
              }
            }
          }
        }
      }
    }
  },
  "type": "object",
  "required": [
    "kind",
    "spec",
    "metadata",
    "apiVersion"
  ],
  "properties": {
    "kind": {
      "enum": [
        "CAPVCDCluster"
      ],
      "type": "string",
      "description": "The kind of the Kubernetes cluster."
    },
    "spec": {
      "type": "object",
      "description": "The user specification of the desired state of the cluster.",
      "properties": {
        "topology": {
          "type": "object",
          "description": "Topology of the kubernetes cluster",
          "properties": {
            "controlPlane": {
              "type": "array",
              "description": "The desired control-plane state of the cluster. The properties \"sizingClass\" and \"storageProfile\" can be specified only during the cluster creation phase. These properties will no longer be modifiable in further update operations like \"resize\" and \"upgrade\".\n ",
              "items": {
                "count": {
                  "type": "integer",
                  "description": "Multi control plane is supported.",
                  "maximum": 100,
                  "minimum": 1
                },
                "sizingClass": {
                  "type": "string",
                  "description": "The compute sizing policy with which control-plane node needs to be provisioned in a given \"ovdc\". The specified sizing policy is expected to be pre-published to the given ovdc."
                },
                "templateName": {
                  "type": "string",
                  "description": "template name for the set of nodes"
                }
              },
              "additionalProperties": true
            },
            "workers": {
              "type": "array",
              "description": "The desired worker state of the cluster. The properties \"sizingClass\" and \"storageProfile\" can be specified only during the cluster creation phase. These properties will no longer be modifiable in further update operations like \"resize\" and \"upgrade\". Non uniform worker nodes in the clusters is not yet supported.",
              "items": {
                "count": {
                  "type": "integer",
                  "description": "Worker nodes can be scaled up and down.",
                  "maximum": 200,
                  "minimum": 0
                },
                "sizingClass": {
                  "type": "string",
                  "description": "The compute sizing policy with which worker nodes need to be provisioned in a given \"ovdc\". The specified sizing policy is expected to be pre-published to the given ovdc."
                },
                "templateName": {
                  "type": "string",
                  "description": "template name for the set of nodes"
                }
              },
              "additionalProperties": true
            }
          }
        },
        "settings": {
          "type": "object",
          "properties": {
            "ovdcNetwork": {
              "type": "string",
              "description": "Name of the Organization's virtual data center network"
            },
            "network": {
              "$ref": "#/definitions/network"
            }
          },
          "additionalProperties": true
        },
        "distribution": {
          "$ref": "#/definitions/distribution"
        },
        "capiYaml": {
          "type": "string",
          "description": "CAPI Yaml specification of the CAPVCD cluster"
        }
      },
      "additionalProperties": true
    },
    "status": {
      "type": "object",
      "x-vcloud-restricted": "protected",
      "description": "The current status of the cluster.",
      "properties": {
        "phase": {
          "type": "string"
        },
        "kubernetes": {
          "type": "string"
        },
        "network": {
          "$ref": "#/definitions/network"
        },
        "uid": {
          "type": "string",
          "description": "unique ID of the cluster"
        },
        "parentUid": {
          "type": "string",
          "description": "unique ID of the parent management cluster"
        },
        "isManagementCluster": {
          "type": "boolean",
          "description": "Does this RDE represent a management cluster?"
        },
        "clusterApiStatus": {
          "type": "object",
          "properties": {
            "phase": {
              "type": "string",
              "description": "The phase describing the control plane infrastructure deployment."
            },
            "apiEndpoints": {
              "type": "array",
              "description": "Control Plane load balancer endpoints",
              "items": {
                "host": {
                  "type": "string"
                },
                "port": {
                  "type": "integer"
                }
              }
            }
          }
        },
        "nodeStatus": {
          "additionalProperties": {
            "type": "string",
            "properties": {}
          }
        },
        "cni": {
          "type": "object",
          "description": "Information regarding the CNI used to deploy the cluster",
          "properties": {
            "name": {
              "type": "string",
              "description": "name of the CNI used in the cluster"
            },
            "version": {
              "type": "string",
              "description": "version of the CNI used in the cluster"
            }
          }
        },
        "csi": {
          "type": "object",
          "description": "details about CSI used in the cluster",
          "properties": {
            "version": {
              "type": "string",
              "description": "version of the CSI used"
            }
          }
        },
        "cpi": {
          "type": "object",
          "description": "details about CPI used in the cluster",
          "properties": {
            "version": {
              "type": "string",
              "description": "version of the CPI used"
            }
          }
        },
        "capvcdVersion": {
          "type": "string",
          "description": "version of the CAPVCD used to deploy the cluster"
        },
        "cloudProperties": {
          "type": "object",
          "description": "The details specific to Cloud Director in which the cluster is hosted.",
          "properties": {
            "orgName": {
              "type": "string",
              "description": "The name of the Organization in which cluster needs to be created or managed."
            },
            "virtualDataCenterName": {
              "type": "string",
              "description": "The name of the Organization Virtual data center in which the cluster need to be created or managed."
            },
            "ovdcNetworkName": {
              "type": "string",
              "description": "The name of the Organization Virtual data center network to which cluster is connected."
            },
            "site": {
              "type": "string",
              "description": "Fully Qualified Domain Name of the VCD site in which the cluster is deployed"
            }
          },
          "additionalProperties": true
        },
        "persistentVolumes": {
          "type": "array",
          "description": "VCD references to the list of persistent volumes.",
          "items": {
            "type": "string"
          }
        },
        "virtualIPs": {
          "type": "array",
          "description": "Array of virtual IPs consumed by the cluster.",
          "items": {
            "type": "string"
          }
        }
      },
      "additionalProperties": true
    },
    "metadata": {
      "type": "object",
      "required": [
        "orgName",
        "virtualDataCenterName",
        "name",
        "site"
      ],
      "properties": {
        "orgName": {
          "type": "string",
          "description": "The name of the Organization in which cluster needs to be created or managed."
        },
        "virtualDataCenterName": {
          "type": "string",
          "description": "The name of the Organization Virtual data center in which the cluster need to be created or managed."
        },
        "name": {
          "type": "string",
          "description": "The name of the cluster."
        },
        "site": {
          "type": "string",
          "description": "Fully Qualified Domain Name of the VCD site in which the cluster is deployed"
        }
      },
      "additionalProperties": true
    },
    "apiVersion": {
      "type": "string",
      "default": "capvcd.vmware.com/v1.0",
      "description": "The version of the payload format"
    }
  },
  "additionalProperties": true
},
    "vendor": "vmware",
    "interfaces": [
        "urn:vcloud:interface:vmware:k8s:1.0.0"
    ],
    "hooks": null,
    "readonly": false
}
```


