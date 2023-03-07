## RDE
RDEs offer persistence in VCD for otherwise CSE's K8S clusters. It contains two parts:
a. the latest desired state expressed by the user  
b. the current state of the cluster.

RDE Upgrade Steps:
1. Create new schema file under /schemas/schema_x_y_z.json
2. Locate the function ConvertToLatestRDEVersionFormat in the vcdcluster_controller.go. 
3. Ensure that you invoke the correct converter function to convert the source RDE version into the latest RDE version in use.
4. Update the existing converters (and/or) add new converters to open up the upgrade paths (For example ```convertFrom100Format()```):
- Provide an automatic conversion of the content in srcCapvcdEntity.entity.status.CAPVCD content to the latest RDE version format (types.CAPVCDStatus)
- Add the placeholder for any special conversion logic inside types.CAPVCDStatus. 
- Update the srcCapvcdEntity.entityType to the latest RDE version in use.
- Call the API update call to update CAPVCD entity and persist data into VCD.


## CAPVCD Upgrade
1. RDE schema registration: Ensure the correct runtime RDE version is chosen and register the appropriate schema if necessary.
2. Use [CAPVCD UPGRADE GUIDE](./UPGRADES.md) to upgrade CAPVCD to the desired version.
3. Upgrade existing RDE instances to the newer runtime RDE chosen by the server.