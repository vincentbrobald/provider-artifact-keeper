// SPDX-FileCopyrightText: 2026 Vincent Brobald
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/alecthomas/kingpin/v2"
	xpcontroller "github.com/crossplane/crossplane-runtime/v2/pkg/controller"
	"github.com/crossplane/crossplane-runtime/v2/pkg/feature"
	"github.com/crossplane/crossplane-runtime/v2/pkg/gate"
	"github.com/crossplane/crossplane-runtime/v2/pkg/logging"
	"github.com/crossplane/crossplane-runtime/v2/pkg/ratelimiter"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/customresourcesgate"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/v2/pkg/statemetrics"
	authv1 "k8s.io/api/authorization/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	"brobald.be/provider-artifact-keeper/apis"
	"brobald.be/provider-artifact-keeper/internal/controller"
	"brobald.be/provider-artifact-keeper/internal/version"
)

func main() {
	var (
		app              = kingpin.New(filepath.Base(os.Args[0]), "Crossplane provider for Artifact Keeper").DefaultEnvars()
		debug            = app.Flag("debug", "Run with debug logging.").Short('d').Bool()
		syncPeriod       = app.Flag("sync", "Controller manager sync period, e.g. 1h.").Short('s').Default("1h").Duration()
		pollInterval     = app.Flag("poll", "How often each resource is checked for drift.").Default("10m").Duration()
		pollStateMetric  = app.Flag("poll-state-metric", "State metric recording interval.").Default("5s").Duration()
		leaderElection   = app.Flag("leader-election", "Use leader election.").Short('l').Default("false").OverrideDefaultFromEnvar("LEADER_ELECTION").Bool()
		maxReconcileRate = app.Flag("max-reconcile-rate", "Global maximum reconciles per second.").Default("10").Int()
		metricsAddress   = app.Flag("metrics-bind-address", "Address of the metrics server.").Default(":8080").Envar("METRICS_BIND_ADDRESS").String()
	)
	kingpin.MustParse(app.Parse(os.Args[1:]))

	zl := zap.New(zap.UseDevMode(*debug))
	log := logging.NewLogrLogger(zl.WithName("provider-artifact-keeper"))
	if *debug {
		ctrl.SetLogger(zl)
	}
	log.Info("Starting", "version", version.Version, "poll", pollInterval.String())

	cfg, err := ctrl.GetConfig()
	kingpin.FatalIfError(err, "Cannot get API server rest config")

	scheme := runtime.NewScheme()
	kingpin.FatalIfError(clientgoscheme.AddToScheme(scheme), "Cannot add client-go scheme")
	kingpin.FatalIfError(apis.AddToScheme(scheme), "Cannot add the provider's APIs")
	kingpin.FatalIfError(apiextensionsv1.AddToScheme(scheme), "Cannot add CustomResourceDefinition")
	kingpin.FatalIfError(authv1.AddToScheme(scheme), "Cannot add authorization APIs")

	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme:           scheme,
		LeaderElection:   *leaderElection,
		LeaderElectionID: "crossplane-leader-election-provider-artifact-keeper",
		Cache: cache.Options{
			SyncPeriod: syncPeriod,
			ByObject: map[client.Object]cache.ByObject{
				&apiextensionsv1.CustomResourceDefinition{}: {Transform: customresourcesgate.TransformStripCRDSchema},
			},
		},
		Metrics:                    metricsserver.Options{BindAddress: *metricsAddress},
		LeaderElectionResourceLock: resourcelock.LeasesResourceLock,
		LeaseDuration:              func() *time.Duration { d := 60 * time.Second; return &d }(),
		RenewDeadline:              func() *time.Duration { d := 50 * time.Second; return &d }(),
	})
	kingpin.FatalIfError(err, "Cannot create controller manager")

	mrMetrics := managed.NewMRMetricRecorder()
	stateMetrics := statemetrics.NewMRStateMetrics()
	metrics.Registry.MustRegister(mrMetrics)
	metrics.Registry.MustRegister(stateMetrics)

	o := xpcontroller.Options{
		Logger:                  log,
		GlobalRateLimiter:       ratelimiter.NewGlobal(*maxReconcileRate),
		PollInterval:            *pollInterval,
		MaxConcurrentReconciles: *maxReconcileRate,
		Features:                &feature.Flags{},
		MetricOptions: &xpcontroller.MetricOptions{
			PollStateMetricInterval: *pollStateMetric,
			MRMetrics:               mrMetrics,
			MRStateMetrics:          stateMetrics,
		},
	}

	canWatch, err := canWatchCRDs(context.TODO(), mgr)
	kingpin.FatalIfError(err, "SafeStart precheck failed")
	if canWatch {
		o.Gate = new(gate.Gate[schema.GroupVersionKind])
		kingpin.FatalIfError(customresourcesgate.Setup(mgr, xpcontroller.Options{Logger: log, Gate: o.Gate, MaxConcurrentReconciles: 1}), "Cannot set up the CRD gate")
		kingpin.FatalIfError(controller.SetupGated(mgr, o), "Cannot set up controllers")
	} else {
		log.Info("No RBAC to watch CRDs: safe-start disabled, every controller starts now")
		kingpin.FatalIfError(controller.Setup(mgr, o), "Cannot set up controllers")
	}
	kingpin.FatalIfError(mgr.Start(ctrl.SetupSignalHandler()), "Cannot start controller manager")
}

func canWatchCRDs(ctx context.Context, mgr manager.Manager) (bool, error) {
	for _, verb := range []string{"get", "list", "watch"} {
		sar := &authv1.SelfSubjectAccessReview{Spec: authv1.SelfSubjectAccessReviewSpec{
			ResourceAttributes: &authv1.ResourceAttributes{Group: "apiextensions.k8s.io", Resource: "customresourcedefinitions", Verb: verb},
		}}
		if err := mgr.GetClient().Create(ctx, sar); err != nil {
			return false, err
		}
		if !sar.Status.Allowed {
			return false, nil
		}
	}
	return true, nil
}
