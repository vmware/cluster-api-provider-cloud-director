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
	"os"
	"sigs.k8s.io/cluster-api/bootstrap/kubeadm/api/v1alpha4"
	"strings"
	"time"

	"k8s.io/klog"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha4"
	"sigs.k8s.io/controller-runtime/pkg/controller"

	"github.com/vmware/cluster-api-provider-cloud-director/pkg/config"
	"github.com/vmware/cluster-api-provider-cloud-director/pkg/vcdclient"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	infrastructurev1alpha4 "github.com/vmware/cluster-api-provider-cloud-director/api/v1alpha4"
	infrav1 "github.com/vmware/cluster-api-provider-cloud-director/api/v1alpha4"
	"github.com/vmware/cluster-api-provider-cloud-director/controllers"
	kcpv1 "sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1alpha4"
	//+kubebuilder:scaffold:imports
)

//go:embed release/version
var capVCDVersion string

var (
	myscheme = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	klog.InitFlags(nil)
	utilruntime.Must(scheme.AddToScheme(myscheme))

	utilruntime.Must(infrav1.AddToScheme(myscheme))

	utilruntime.Must(clusterv1.AddToScheme(myscheme))
	utilruntime.Must(kcpv1.AddToScheme(myscheme))
	utilruntime.Must(infrastructurev1alpha4.AddToScheme(myscheme))
	utilruntime.Must(v1alpha4.AddToScheme(myscheme))
	//+kubebuilder:scaffold:scheme
}

func getVcdClientFromConfig(inputMap map[string]interface{}) (*vcdclient.Client, error) {
	configFilePath := "/etc/kubernetes/vcloud/controller_manager_config.yaml"
	configReader, err := os.Open(configFilePath)
	if err != nil {
		return nil, fmt.Errorf("Unable to open file [%s]: [%v]", configFilePath, err)
	}
	defer configReader.Close()

	cloudConfig, err := config.ParseCloudConfig(configReader)
	if err != nil {
		return nil, fmt.Errorf("Unable to parse cloud config file [%s]: [%v]", configFilePath, err)
	}

	err = config.SetAuthorization(cloudConfig)
	if err != nil {
		return nil, fmt.Errorf("unable to set authorization in config: [%v]", err)
	}

	err = config.ValidateCloudConfig(cloudConfig)
	if err != nil {
		return nil, fmt.Errorf("error validating config: [%v]", err)
	}

	insecure := true
	oneArm := &vcdclient.OneArm{
		StartIPAddress: cloudConfig.LB.OneArm.StartIP,
		EndIPAddress:   cloudConfig.LB.OneArm.EndIP,
	}
	getVdcClient := true
	trimmedCapvcdVersion := strings.Trim(capVCDVersion, "\n")
	// TODO (Sahithi: Let this method take the cloudConfig as a param instead of individual properties)
	return vcdclient.NewVCDClientFromSecrets(
		cloudConfig.VCD.Host,
		cloudConfig.VCD.Org,
		cloudConfig.VCD.VDC,
		"",
		cloudConfig.VCD.VDCNetwork,
		cloudConfig.VCD.VIPSubnet,
		cloudConfig.VCD.UserOrg,
		cloudConfig.VCD.User,
		cloudConfig.VCD.Secret,
		cloudConfig.VCD.RefreshToken,
		insecure,
		cloudConfig.ClusterID,
		oneArm,
		cloudConfig.LB.Ports.HTTP,
		cloudConfig.LB.Ports.HTTPS,
		cloudConfig.LB.Ports.TCP,
		getVdcClient,
		cloudConfig.ManagementClusterRDEId,
		cloudConfig.ClusterResources.CsiVersion,
		cloudConfig.ClusterResources.CpiVersion,
		cloudConfig.ClusterResources.CniVersion,
		trimmedCapvcdVersion,
	)
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string
	var syncPeriod time.Duration
	var concurrency int

	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.DurationVar(&syncPeriod, "sync-period", 30*time.Second,
		"The minimum interval at which watched resources are reconciled (e.g. 15m)")
	flag.IntVar(&concurrency, "concurrency", 10,
		"The number of VCD machines to process simultaneously")

	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 myscheme,
		MetricsBindAddress:     metricsAddr,
		Port:                   9443,
		SyncPeriod:             &syncPeriod,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "cluster.x-k8s.io",
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}
	vcdClient, err := getVcdClientFromConfig(nil)
	if err != nil {
		panic(fmt.Errorf("unable to connect to vcd: [%v]", err))
	}

	ctx := context.Background()

	if err = (&controllers.VCDMachineReconciler{
		Client: mgr.GetClient(),
		// Scheme:    mgr.GetScheme(),
		VcdClient: vcdClient,
	}).SetupWithManager(ctx, mgr, controller.Options{
		MaxConcurrentReconciles: concurrency,
	}); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "VCDMachine")
		os.Exit(1)
	}

	if err = (&controllers.VCDClusterReconciler{
		Client:    mgr.GetClient(),
		Scheme:    mgr.GetScheme(),
		VcdClient: vcdClient,
	}).SetupWithManager(mgr, controller.Options{
		MaxConcurrentReconciles: concurrency,
	}); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "VCDCluster")
		os.Exit(1)
	}
	//+kubebuilder:scaffold:builder

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}

	err = vcdClient.SetIsManagementClusterInRDE(ctx)
	if err != nil {
		setupLog.Error(err, "unable to set isManagementCluster flag")
	} else {
		setupLog.Info("successfully set isManagementCLuster flag in RDE: [%s]", vcdClient.ManagementClusterRDEId)
	}
}
