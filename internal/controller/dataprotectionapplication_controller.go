/*
Copyright 2021.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"os"

	"github.com/go-logr/logr"
	routev1 "github.com/openshift/api/route/v1"
	security "github.com/openshift/api/security/v1"
	velerov1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
	oadpclient "github.com/openshift/oadp-operator/pkg/client"
)

// DataProtectionApplicationReconciler reconciles a DataProtectionApplication object
type DataProtectionApplicationReconciler struct {
	client.Client
	Scheme            *runtime.Scheme
	Log               logr.Logger
	Context           context.Context
	NamespacedName    types.NamespacedName
	EventRecorder     record.EventRecorder
	dpa               *oadpv1alpha1.DataProtectionApplication
	ClusterWideClient client.Client
}

var debugMode = os.Getenv("DEBUG") == "true"

//+kubebuilder:rbac:groups=oadp.openshift.io,resources=dataprotectionapplications,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=oadp.openshift.io,resources=dataprotectionapplications/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=oadp.openshift.io,resources=dataprotectionapplications/finalizers,verbs=update

//+kubebuilder:rbac:groups=config.openshift.io,resources=infrastructures,verbs=get;list;watch
//+kubebuilder:rbac:groups=cloudcredential.openshift.io,resources=credentialsrequests,verbs=get;create;update
//+kubebuilder:rbac:groups=oadp.openshift.io,resources=*,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=corev1;coordination.k8s.io,resources=secrets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=velero.io,resources=*,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=security.openshift.io,resources=securitycontextconstraints,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=security.openshift.io,resources=securitycontextconstraints,verbs=use,resourceNames=privileged
//+kubebuilder:rbac:groups="",resources=secrets;configmaps;pods;services;serviceaccounts;endpoints;persistentvolumeclaims;events,verbs=get;list;watch;create;update;patch;delete;deletecollection
//+kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch;create;update;patch
//+kubebuilder:rbac:groups=apps,resources=deployments;daemonsets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=route.openshift.io,resources=routes,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=monitoring.coreos.com,resources=servicemonitors,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main Kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *DataProtectionApplicationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	r.Log = log.FromContext(ctx)
	logger := r.Log.WithValues("dpa", req.NamespacedName)
	result := ctrl.Result{}
	// Set reconciler context + name
	r.Context = ctx
	r.NamespacedName = req.NamespacedName
	r.dpa = &oadpv1alpha1.DataProtectionApplication{}

	if err := r.Get(ctx, req.NamespacedName, r.dpa); err != nil {
		logger.Error(err, "unable to fetch DataProtectionApplication CR")
		return result, nil
	}

	// set client to pkg/client for use in non-reconcile functions
	oadpclient.SetClient(r.Client)

	_, err := ReconcileBatch(r.Log,
		r.ValidateDataProtectionCR,
		r.ReconcileFsRestoreHelperConfig,
		r.ReconcileBackupStorageLocations,
		r.ReconcileRegistrySecrets,
		r.ReconcileRegistries,
		r.ReconcileRegistrySVCs,
		r.ReconcileRegistryRoutes,
		r.ReconcileRegistryRouteConfigs,
		r.LabelVSLSecrets,
		r.ReconcileVolumeSnapshotLocations,
		r.ReconcileAzureWorkloadIdentitySecret,
		r.ReconcileVeleroDeployment,
		r.ReconcileNodeAgentConfigMap,
		r.ReconcileBackupRepositoryConfigMap,
		r.ReconcileRepositoryMaintenanceConfigMap,
		r.ReconcileNodeAgentDaemonset,
		r.ReconcileVeleroMetricsSVC,
		r.ReconcileNonAdminController,
	)

	if err != nil {
		apimeta.SetStatusCondition(&r.dpa.Status.Conditions,
			metav1.Condition{
				Type:    oadpv1alpha1.ConditionReconciled,
				Status:  metav1.ConditionFalse,
				Reason:  oadpv1alpha1.ReconciledReasonError,
				Message: err.Error(),
			},
		)

	} else {
		apimeta.SetStatusCondition(&r.dpa.Status.Conditions,
			metav1.Condition{
				Type:    oadpv1alpha1.ConditionReconciled,
				Status:  metav1.ConditionTrue,
				Reason:  oadpv1alpha1.ReconciledReasonComplete,
				Message: oadpv1alpha1.ReconcileCompleteMessage,
			},
		)
	}
	statusErr := r.Client.Status().Update(ctx, r.dpa)
	if err == nil { // Don't mask previous error
		err = statusErr
	}

	return ctrl.Result{}, err
}

// SetupWithManager sets up the controller with the Manager.
func (r *DataProtectionApplicationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&oadpv1alpha1.DataProtectionApplication{}).
		Owns(&appsv1.Deployment{}).
		Owns(&velerov1.BackupStorageLocation{}).
		Owns(&velerov1.VolumeSnapshotLocation{}).
		Owns(&appsv1.DaemonSet{}).
		Owns(&security.SecurityContextConstraints{}).
		Owns(&corev1.Service{}).
		Owns(&routev1.Route{}).
		Owns(&corev1.ConfigMap{}).
		Watches(&corev1.Secret{}, &labelHandler{}).
		WithEventFilter(veleroPredicate(r.Scheme)).
		Complete(r)
}

type labelHandler struct{}

func (l *labelHandler) Create(ctx context.Context, evt event.TypedCreateEvent[client.Object], q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	// check for the label & add it to the queue
	namespace := evt.Object.GetNamespace()
	dpaname := evt.Object.GetLabels()["dataprotectionapplication.name"]
	if evt.Object.GetLabels()[oadpv1alpha1.OadpOperatorLabel] == "" || dpaname == "" {
		return
	}

	q.Add(reconcile.Request{NamespacedName: types.NamespacedName{
		Name:      dpaname,
		Namespace: namespace,
	}})

}
func (l *labelHandler) Delete(ctx context.Context, evt event.TypedDeleteEvent[client.Object], q workqueue.TypedRateLimitingInterface[reconcile.Request]) {

	namespace := evt.Object.GetNamespace()
	dpaname := evt.Object.GetLabels()["dataprotectionapplication.name"]
	if evt.Object.GetLabels()[oadpv1alpha1.OadpOperatorLabel] == "" || dpaname == "" {
		return
	}
	q.Add(reconcile.Request{NamespacedName: types.NamespacedName{
		Name:      dpaname,
		Namespace: namespace,
	}})

}
func (l *labelHandler) Update(ctx context.Context, evt event.TypedUpdateEvent[client.Object], q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	namespace := evt.ObjectNew.GetNamespace()
	dpaname := evt.ObjectNew.GetLabels()["dataprotectionapplication.name"]
	if evt.ObjectNew.GetLabels()[oadpv1alpha1.OadpOperatorLabel] == "" || dpaname == "" {
		return
	}
	q.Add(reconcile.Request{NamespacedName: types.NamespacedName{
		Name:      dpaname,
		Namespace: namespace,
	}})

}
func (l *labelHandler) Generic(ctx context.Context, evt event.TypedGenericEvent[client.Object], q workqueue.TypedRateLimitingInterface[reconcile.Request]) {

	namespace := evt.Object.GetNamespace()
	dpaname := evt.Object.GetLabels()["dataprotectionapplication.name"]
	if evt.Object.GetLabels()[oadpv1alpha1.OadpOperatorLabel] == "" || dpaname == "" {
		return
	}
	q.Add(reconcile.Request{NamespacedName: types.NamespacedName{
		Name:      dpaname,
		Namespace: namespace,
	}})

}

type ReconcileFunc func(logr.Logger) (bool, error)

// reconcileBatch steps through a list of reconcile functions until one returns
// false or an error.
func ReconcileBatch(l logr.Logger, reconcileFuncs ...ReconcileFunc) (bool, error) {
	// TODO: #1127 DPAReconciler already have a logger, use it instead of passing to each reconcile functions
	for _, f := range reconcileFuncs {
		if cont, err := f(l); !cont || err != nil {
			return cont, err
		}
	}
	return true, nil
}
