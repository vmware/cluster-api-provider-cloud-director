# Setting up Cloud Director environment

## Provider steps

### NSX-T and Avi Setup
The LoadBalancers fronting the multi-controlplane workload clusters need a preconfigured Avi Controller, NSX-T Cloud and Avi Service Engine Group.
Refer to [load balancer set up](https://github.com/vmware/cloud-provider-for-cloud-director#provider-setup).

<a name="register_rde_schema"></a>
### Register Cluster API schema in Cloud Director
Using Postman, register the Cluster API RDE (Runtime Defined Entity Type) schema with Cloud Director. This is required for CAPVCD to create 
a logical object (RDE) in the Cloud Director for every workload cluster creation.
RDE (Runtime Defined Entity Type) in Cloud Director is analogous to CRD (Custom Resource Definition) in Kubernetes. 

POST `https://<vcd>/cloudapi/1.0.0/entityTypes`
Body: [payload](#capvcd_rde_schema)

As a side effect of the above schema registration, `vmware:capvcdCluster Entitlement` right bundle gets automatically created.

<a name="user_role"></a>  
### Publish the rights to the tenant organizations
1. Publish the `vmware:capvcdCluster Entitlement` right bundle to the desired tenant organizations
2. Below are the rights required for the Cluster API. Assign these rights to the desired tenant organizations
    * User > Manage user's own API token
    * vApp > Preserve ExtraConfig Elements during OVA Import and Export (follow the [KB](https://kb.vmware.com/s/article/2148573) to enable this right on VCD)
    * Gateway > View Gateway
    * Gateway Services > NAT Configure, LoadBalancer Configure
    * Rights from the default `vApp Author` role
    * Right 'Full Control: VMWARE:CAPVCDCLUSTER'
    * [Rights required for CPI](https://github.com/vmware/cloud-provider-for-cloud-director#additional-rights-for-cpi)
    * [Rights required for CSI](https://github.com/vmware/cloud-director-named-disk-csi-driver#additional-rights-for-csi)

### Upload VMware Tanzu Kubernetes Grid Kubernetes Templates
Import Ubuntu 20.04 Kubernetes OVAs from VMware Tanzu Kubernetes Grid Versions 1.4.3, 1.5.4 to VCD using VCD UI. 
These will serve as templates for Cluster API to create Kubernetes Clusters.

## Tenant Admin steps
* A ServiceEngineGroup needs to be added to the gateway of the OVDC within which the Kubernetes cluster is to be created. The overall steps to achieve that are documented at Enable Load Balancer on an NSX-T Data Center Edge Gateway
Create and publish the desired sizing policies on the chosen OVDC(s)
* Ensure the OVDC gateway has outbound access. If required, set an SNAT rule with the internal IP range of the VMs.
* Set up DNS on the desired virtual datacenter networks.
* Create tenant user role with the above mentioned [rights](#user_role)
* For tenant admins to perform cluster management functions, you must hold the rights as mentioned above. In addition to 
  that, you will need “Administrator View: VMWARE:CAPVCDCLUSTER” to view all the clusters in your organization. 
  Please contact your service provider if you cannot assign these rights to yourself.

<a name="capvcd_rde_schema"></a>
**Payload of the Cluster API schema**
* CAPVCD Schema Payload reference file: [CAPVCD Schema Entity Type](https://github.com/vmware/cluster-api-provider-cloud-director/tree/main/schema/entity_type.json)
* Note: The CAPVCD Schema Payload reference file contains the full body/payload required for registering CAPVCD Entity Type which also includes [schema](https://github.com/vmware/cluster-api-provider-cloud-director/tree/main/schema/schema_1_1_0.json) and other fields.

POST `https://<vcd>/cloudapi/1.0.0/entityTypes` with the provided payload:
```json
{
    "name": "CAPVCD Cluster",
    "description": "",
    "nss": "capvcdCluster",
    "version": "1.1.0",
    "inheritedVersion": null,
    "externalId": null,
    "schema": {
       "definitions": {
          "k8sNetwork": {
             "type": "object",
             "description": "The network-related settings for the cluster.",
             "properties": {
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
             "properties": {
                "capiYaml": {
                   "type": "string"
                },
                "yamlSet": {
                   "type": "array",
                   "items": {
                      "type": "string"
                   }
                }
             }
          },
          "metadata": {
             "type": "object",
             "properties": {
                "orgName": {
                   "type": "string",
                   "description": "The name of the Organization in which cluster needs to be created or managed."
                },
                "virtualDataCenterName": {
                   "type": "string",
                   "description": "The name of the Organization data center in which the cluster need to be created or managed."
                },
                "name": {
                   "type": "string",
                   "description": "The name of the cluster."
                },
                "site": {
                   "type": "string",
                   "description": "Fully Qualified Domain Name of the VCD site in which the cluster is deployed"
                }
             }
          },
          "status": {
             "type": "object",
             "x-vcloud-restricted": "protected",
             "properties": {
                "capvcd": {
                   "type": "object",
                   "properties": {
                      "phase": {
                         "type": "string"
                      },
                      "kubernetes": {
                         "type": "string"
                      },
                      "errorSet": {
                         "type": "array",
                         "items": {
                            "type": "object",
                            "properties": {}
                         }
                      },
                      "eventSet": {
                         "type": "array",
                         "items": {
                            "type": "object",
                            "properties": {}
                         }
                      },
                      "k8sNetwork": {
                         "$ref": "#/definitions/k8sNetwork"
                      },
                      "uid": {
                         "type": "string"
                      },
                      "parentUid": {
                         "type": "string"
                      },
                      "useAsManagementCluster": {
                         "type": "boolean"
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
                      "nodePool": {
                         "type": "array",
                         "items": {
                            "type": "object",
                            "properties": {
                               "name": {
                                  "type": "string",
                                  "description": "name of the node pool"
                               },
                               "sizingPolicy": {
                                  "type": "string",
                                  "description": "name of the sizing policy used by the node pool"
                               },
                               "placementPolicy": {
                                  "type": "string",
                                  "description": "name of the sizing policy used by the node pool"
                               },
                               "diskSizeMb": {
                                  "type": "integer",
                                  "description": "disk size of the VMs in the node pool in MB"
                               },
                               "nvidiaGpuEnabled": {
                                  "type": "boolean",
                                  "description": "boolean indicating if the node pools have nvidia GPU enabled"
                               },
                               "storageProfile": {
                                  "type": "string",
                                  "description": "storage profile used by the node pool"
                               },
                               "desiredReplicas": {
                                  "type": "integer",
                                  "description": "desired replica count of the nodes in the node pool"
                               },
                               "availableReplicas": {
                                  "type": "integer",
                                  "description": "number of available replicas in the node pool"
                               }
                            }
                         }
                      },
                      "clusterResourceSet": {
                         "properties": {}
                      },
                      "clusterResourceSetBindings": {
                         "type": "array",
                         "items": {
                            "type": "object",
                            "properties": {
                               "clusterResourceSetName": {
                                  "type": "string"
                               },
                               "kind": {
                                  "type": "string"
                               },
                               "name": {
                                  "type": "string"
                               },
                               "applied": {
                                  "type": "boolean"
                               },
                               "lastAppliedTime": {
                                  "type": "string"
                               }
                            }
                         }
                      },
                      "capvcdVersion": {
                         "type": "string"
                      },
                      "vcdProperties": {
                         "type": "object",
                         "properties": {
                            "organizations": {
                               "type": "array",
                               "items": {
                                  "type": "object",
                                  "properties": {
                                     "name": {
                                        "type": "string"
                                     },
                                     "id": {
                                        "type": "string"
                                     }
                                  }
                               }
                            },
                            "site": {
                               "type": "string"
                            },
                            "orgVdcs": {
                               "type": "array",
                               "items": {
                                  "type": "object",
                                  "properties": {
                                     "name": {
                                        "type": "string"
                                     },
                                     "id": {
                                        "type": "string"
                                     },
                                     "ovdcNetworkName": {
                                        "type": "string"
                                     }
                                  }
                               }
                            }
                         }
                      },
                      "upgrade": {
                         "type": "object",
                         "description": "determines the state of upgrade. If no upgrade is issued, only the existing version is stored.",
                         "properties": {
                            "current": {
                               "type": "object",
                               "properties": {
                                  "kubernetesVersion": {
                                     "type": "string",
                                     "description": "current kubernetes version of the cluster. If being upgraded, will represent target kubernetes version of the cluster."
                                  },
                                  "tkgVersion": {
                                     "type": "string",
                                     "description": "current TKG version of the cluster. If being upgraded, will represent the tarkget TKG version of the cluster."
                                  }
                               }
                            },
                            "previous": {
                               "type": "object",
                               "properties": {
                                  "kubernetesVersion": {
                                     "type": "string",
                                     "description": "the kubernetes version from which the cluster was upgraded from. If cluster upgrade is still in progress, the field will represent the source kubernetes version from which the cluster is being upgraded."
                                  },
                                  "tkgVersion": {
                                     "type": "string",
                                     "description": "the TKG version from which the cluster was upgraded from. If cluster upgrade is still in progress, the field will represent the source TKG versoin from which the cluster is being upgraded."
                                  }
                               }
                            },
                            "ready": {
                               "type": "boolean",
                               "description": "boolean indicating the status of the cluster upgrade."
                            }
                         }
                      },
                      "private": {
                         "type": "object",
                         "x-vcloud-restricted": "private",
                         "description": "Placeholder for the properties invisible to non-admin users.",
                         "properties": {
                            "kubeConfig": {
                               "type": "string",
                               "description": "Admin kube config to access the Kubernetes cluster."
                            }
                         }
                      },
                      "vcdResourceSet": {
                         "type": "array",
                         "items": {
                            "type": "object",
                            "properties": {}
                         }
                      },
                      "createdByVersion": {
                         "type": "string",
                         "description": "CAPVCD version used to create the cluster"
                      }
                   }
                }
             }
          }
       }
    },
    "vendor": "vmware",
    "interfaces": [
        "urn:vcloud:interface:vmware:k8s:1.0.0"
    ],
    "hooks": null,
    "readonly": false
}
```



