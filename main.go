/*
   Copyright 2021 VMware, Inc.
   SPDX-License-Identifier: Apache-2.0
*/

package main

import (
	"context"
	_ "embed"
	"flag"
	"fmt"
	"log"
	"os"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"time"

	"github.com/vmware/cluster-api-provider-cloud-director/release"

	"go.uber.org/zap/zapcore"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/klog"
	clusterv1beta1 "sigs.k8s.io/cluster-api/api/v1beta1"
	bootstrapv1beta1 "sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1beta1"
	addonsv1 "sigs.k8s.io/cluster-api/exp/addons/api/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/controller"

	infrav1alpha4 "github.com/vmware/cluster-api-provider-cloud-director/api/v1alpha4"
	infrav1beta1 "github.com/vmware/cluster-api-provider-cloud-director/api/v1beta1"
	infrav1beta2 "github.com/vmware/cluster-api-provider-cloud-director/api/v1beta2"
	infrav1beta3 "github.com/vmware/cluster-api-provider-cloud-director/api/v1beta3"
	"github.com/vmware/cluster-api-provider-cloud-director/controllers"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	kcpv1beta1 "sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1beta1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var (
	myscheme = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	klog.InitFlags(nil)
	utilruntime.Must(scheme.AddToScheme(myscheme))
	// We need both schemes in order to be able to handle v1alpha4 and v1beta1 Infra objects. We can remove the v1alpha4
	// when that version gets deprecated. However we will handle v1alpha4 objects by converting them to the v1beta1 hub.
	utilruntime.Must(infrav1alpha4.AddToScheme(myscheme))
	utilruntime.Must(infrav1beta1.AddToScheme(myscheme))
	utilruntime.Must(infrav1beta2.AddToScheme(myscheme))
	utilruntime.Must(infrav1beta3.AddToScheme(myscheme))
	// We only need the v1beta1 for core CAPI since their webhooks will convert v1alpha4 to v1beta1. We will handle all
	// core CAPI objects using v1beta1 using the available webhook conversion. Hence v1beta1 support in core CAPI is
	// mandatory.
	utilruntime.Must(clusterv1beta1.AddToScheme(myscheme))
	utilruntime.Must(kcpv1beta1.AddToScheme(myscheme))
	utilruntime.Must(bootstrapv1beta1.AddToScheme(myscheme))
	// We need the addonsv1 scheme in order to list the ClusterResourceSetBindings addon.
	utilruntime.Must(addonsv1.AddToScheme(myscheme))
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string
	var syncPeriod time.Duration
	var concurrency int
	var webhookPort int

	var useKubernetesHostEnvAsControlPlaneHost bool
	var useNormalVMsCreationInsteadTKG bool
	var resizeDiskBeforeNetworkReconciliation bool
	var passHostnameByGuestInfo bool
	var defaultNetworkModeForNewVM string

	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.DurationVar(&syncPeriod, "sync-period", 2*time.Minute,
		"The minimum interval at which watched resources are reconciled (e.g. 15m)")
	flag.IntVar(&concurrency, "concurrency", 10,
		"The number of VCD machines to process simultaneously")
	flag.IntVar(&webhookPort, "webhook-port", 9443,
		"The number of VCD webhook port")

	flag.BoolVar(&useKubernetesHostEnvAsControlPlaneHost, "use-kubernetes-host-env-as-control-plane-host", false,
		"Use KUBERNETES_SERVICE_HOST env as control plane host")
	flag.BoolVar(&useNormalVMsCreationInsteadTKG, "use-normal-vms-creation-instead-tkg", false,
		"Use normal VMs creation instead TKG vm")
	flag.BoolVar(&resizeDiskBeforeNetworkReconciliation, "resize-disk-before-network-reconciliation", false,
		"Resizing vm root disk before network reconciliation")
	flag.BoolVar(&passHostnameByGuestInfo, "pass-hostname-by-guest-info", false,
		"Pass hostname to vm by guest info")
	flag.StringVar(&defaultNetworkModeForNewVM, "default-network-mode-for-new-vm", "POOL",
		"The default network mode for new VM. Can be: POOL (default), DHCP, MANUAL")

	opts := zap.Options{
		Development: true,
		TimeEncoder: zapcore.ISO8601TimeEncoder, // ISO8601 Format: 2022-10-25T05:58:15.639Z
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	availableNetworksModes := map[string]struct{}{
		"POOL":   {},
		"DHCP":   {},
		"MANUAL": {},
	}
	if _, ok := availableNetworksModes[defaultNetworkModeForNewVM]; !ok {
		log.Fatal("Incorrect default-network-mode-for-new-vm. Can be POOL, DHCP or MANUAL")
	}

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))
	if release.Version == "" {
		setupLog.Error(fmt.Errorf("release.Version variable should not be empty"), "")
	}
	setupLog.Info("CAPVCD version", "version", release.Version)

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:             myscheme,
		MetricsBindAddress: metricsAddr,
		WebhookServer: webhook.NewServer(webhook.Options{
			Port: webhookPort,
		}),
		SyncPeriod:             &syncPeriod,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "capvcd-controller-manager-leader-election",
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	ctx := context.Background()

	if err = (&controllers.VCDMachineReconciler{
		Client: mgr.GetClient(),
		Params: controllers.VCDMachineReconcilerParams{
			UseNormalVms:                          useNormalVMsCreationInsteadTKG,
			ResizeDiskBeforeNetworkReconciliation: resizeDiskBeforeNetworkReconciliation,
			PassHostnameByGuestInfo:               passHostnameByGuestInfo,
		},
	}).SetupWithManager(ctx, mgr, controller.Options{
		MaxConcurrentReconciles: concurrency,
	}); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "VCDMachine")
		os.Exit(1)
	}

	if err = (&controllers.VCDClusterReconciler{
		Client:                                 mgr.GetClient(),
		Scheme:                                 mgr.GetScheme(),
		UseKubernetesHostEnvAsControlPlaneHost: useKubernetesHostEnvAsControlPlaneHost,
	}).SetupWithManager(mgr, controller.Options{
		MaxConcurrentReconciles: concurrency,
	}); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "VCDCluster")
		os.Exit(1)
	}

	if os.Getenv("ENABLE_WEBHOOKS") != "false" {
		if err = (&infrav1beta3.VCDCluster{}).SetupWebhookWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create webhook", "webhook", "VCDCluster")
			os.Exit(1)
		}
		if err = (&infrav1beta3.VCDMachine{}).SetupWebhookWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create webhook", "webhook", "VCDMachine")
			os.Exit(1)
		}
		if err = (&infrav1beta3.VCDMachineTemplate{}).SetupWebhookWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create webhook", "webhook", "VCDMachineTemplate")
			os.Exit(1)
		}
	}

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	// TODO: check if entity type [capvcdCluster:1.1.0] is already registered

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
