# CAPVCD Developer guide

## TODO list at the beginning of each release
* Branching strategy 
  * Create feature branch if needed (Only feature pertinent commits need to go here). Feature branch needs to rebased periodically with the main.
  * All non-feature commits go to main branch and get cherry-picked to the chosen release-branches (eg: 0.5.x)  
* Bump up the `release/version` to newer CAPVCD version
* Bump up the RDE major version only if you are anticipating breaking changes (breaking ~ breaking backward compatibility)
  * Avoid minor/patch version bump ups, instead use `RDE.status.capvcd.version` property to upgrade individual section upgrades.
  * Read [RDE versioning guidelines](https://confluence.eng.vmware.com/display/VCD/RDE+upgrade+management+across+CAPVCD%2C+VKP%2C+CSI%2C+CPI#RDEupgrademanagementacrossCAPVCD,VKP,CSI,CPI-RDEversioningguidelines) for more details 
* Decide if CAPVCD API version needs to bumped up. If yes, run the steps at [CAPVCD upgrades](https://confluence.eng.vmware.com/display/VCD/CAPVCD+upgrades)

## Dev/Debug setup
Below steps enable us to use the KIND as a management cluster while running CAPVCD locally on laptop.

**Create KIND cluster and install Core CAPI pods**
* kind create cluster --config kind-cluster-with-extramounts.yaml
* kubectl cluster-info --context kind-kind
* kubectl config set-context kind-kind
* kubectl get po -A -owide
* clusterctl init --core cluster-api:v0.4.2 -b kubeadm:v0.4.2 -c kubeadm:v0.4.2

**Install CAPVCD CRDs and manifests**  
* Comment below items from `config/default/kustomization.yaml`
  * ``` 
    bases:
    - ../rbac
    - ../manager
    patchesStrategicMerge:
    - manager_webhook_patch.yaml
    ```
* Replace the IP address placeholder in `config/webhooks/devSetupService.yaml` to your laptop IP.
* Rename `config/webhooks/devSetupService.yaml` to `service.yaml`
* Run `kubectl apply -k config/default`

**Launch CAPVCD in debug mode locally**
* Find out the directory your debugger is running; just launch the main.go in debug mode. It would instantly fail with the 
  error that it is missing `tls.crt` at a particular path. The next step is to copy tls cert and key to that path.
* Copy the tls cert and key (required for the webhookserver handshake) into the folder where your debugger is running
  * `kubectl get secret -n capvcd-system webhook-server-cert -o yaml`
  * Decode(base64) the content of tls.crt and tls.key and copy them into files `tls.crt` and `tls.key` respectively, in the path mentioned above.
* Now, run the main.go in the debug mode

## Upgrades
* [Operator upgrades](https://confluence.eng.vmware.com/display/VCD/CAPVCD+upgrades)
* [RDE upgrades](https://confluence.eng.vmware.com/display/VCD/RDE+upgrade+management+across+CAPVCD%2C+VKP%2C+CSI%2C+CPI)

## RDE management

* [RDE management guidelines](https://confluence.eng.vmware.com/display/VCD/RDE+management+guidelines+for+VKP%2C+CAPVCD%2C+CSI%2C+CPI%2C+UI)
* [RDE versioning guidelines](https://confluence.eng.vmware.com/display/VCD/RDE+upgrade+management+across+CAPVCD%2C+VKP%2C+CSI%2C+CPI#RDEupgrademanagementacrossCAPVCD,VKP,CSI,CPI-RDEversioningguidelines)
* [RDE upgrades](https://confluence.eng.vmware.com/display/VCD/RDE+upgrade+management+across+CAPVCD%2C+VKP%2C+CSI%2C+CPI#RDEupgrademanagementacrossCAPVCD,VKP,CSI,CPI-SinglestoredversionofRDE(FINAL))
* During the release development cycle, on making any changes to CAPVCD schema, bump up the dev version at release/version. For example: 1.1.0.dev2








