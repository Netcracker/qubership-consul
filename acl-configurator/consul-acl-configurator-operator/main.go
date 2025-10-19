// Copyright 2024-2025 NetCracker Technology Corporation
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"flag"
	"fmt"
	"github.com/Netcracker/consul-acl-configurator/consul-acl-configurator-operator/util"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"os"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"strings"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	qubershiporgv1 "github.com/Netcracker/consul-acl-configurator/consul-acl-configurator-operator/api/v1alpha1"
	"github.com/Netcracker/consul-acl-configurator/consul-acl-configurator-operator/controllers"
	//+kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(qubershiporgv1.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	ownNamespace := os.Getenv("NAMESPACE")
	if ownNamespace == "" {
		setupLog.Error(fmt.Errorf("the NAMESPACE environment variable must be set"), "unable to get the current namespace")
		os.Exit(1)
	}
	watchNamespaces, err := getWatchNamespace()
	if err != nil {
		setupLog.Error(err, "unable to get list of watched namespaces")
		os.Exit(1)
	}

	mgrOptions := ctrl.Options{
		Scheme:                  scheme,
		MetricsBindAddress:      metricsAddr,
		Port:                    9443,
		HealthProbeBindAddress:  probeAddr,
		LeaderElection:          enableLeaderElection,
		LeaderElectionID:        fmt.Sprintf("consulacls.%s.netcracker.com", ownNamespace),
		LeaderElectionNamespace: ownNamespace,
	}

	configureMgrNamespaces(&mgrOptions, watchNamespaces, ownNamespace)

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), mgrOptions)
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	if err = (&controllers.ConsulACLReconciler{
		Client:           mgr.GetClient(),
		Scheme:           mgr.GetScheme(),
		ResourceVersions: map[string]string{},
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ConsulACL")
		os.Exit(1)
	}

	customScheme := runtime.NewScheme()

	utilruntime.Must(clientgoscheme.AddToScheme(customScheme))

	GroupVersion := schema.GroupVersion{
		Group:   "netcracker.com",
		Version: "v1alpha1",
	}
	SchemeBuilder := runtime.NewSchemeBuilder(func(scheme *runtime.Scheme) error {
		scheme.AddKnownTypes(GroupVersion, &qubershiporgv1.ConsulACL{})
		return nil
	})

	AddToScheme := SchemeBuilder.AddToScheme
	err = AddToScheme(customScheme)
	if err != nil {
		panic(err)
	}
	if err = (&controllers.ConsulACLReconciler{
		Client:           mgr.GetClient(),
		Scheme:           customScheme,
		ResourceVersions: map[string]string{},
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ConsulACL")
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

	setupLog.Info("starting ConsulACL manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running ConsulACL manager")
		os.Exit(1)
	}
}

// getWatchNamespace returns the Namespace the operator should be watching for changes
func getWatchNamespace() (string, error) {
	// WatchNamespaceEnvVar is the constant for env variable WATCH_NAMESPACE
	// which specifies the Namespace to watch.
	// An empty value means the operator is running with cluster scope.
	var watchNamespaceEnvVar = "WATCH_NAMESPACE"

	ns, found := os.LookupEnv(watchNamespaceEnvVar)
	if !found {
		return "", fmt.Errorf("%s must be set", watchNamespaceEnvVar)
	}
	return ns, nil
}

func configureMgrNamespaces(mgrOptions *ctrl.Options, namespace string, ownNamespace string) {
	if namespace == "" || namespace == ownNamespace {
		mgrOptions.Namespace = namespace
	} else {
		namespaces := strings.Split(namespace, ",")
		if !util.Contains(ownNamespace, namespaces) {
			namespaces = append(namespaces, ownNamespace)
		}
		mgrOptions.NewCache = cache.MultiNamespacedCacheBuilder(namespaces)
	}
}
