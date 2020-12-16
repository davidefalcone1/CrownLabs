/*


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

package tenant_controller

import (
	"context"
	"fmt"

	crownlabsv1alpha1 "github.com/netgroup-polito/CrownLabs/operators/api/v1alpha1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// TenantReconciler reconciles a Tenant object
type TenantReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	KcA    *KcActor
}

// +kubebuilder:rbac:groups=crownlabs.polito.it,resources=tenants,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=crownlabs.polito.it,resources=tenants/status,verbs=get;update;patch

// Reconcile reconciles the state of a tenant resource
func (r *TenantReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	var tn crownlabsv1alpha1.Tenant

	if err := r.Get(ctx, req.NamespacedName, &tn); err != nil {
		// reconcile was triggered by a delete request
		klog.Infof("Tenant %s deleted", req.Name)
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	var retrigErr error = nil
	if tn.Status.Subscriptions == nil {
		tn.Status.Subscriptions = make(map[string]crownlabsv1alpha1.SubscriptionStatus)
	}

	klog.Infof("Reconciling tenant %s", req.Name)

	nsName := fmt.Sprintf("tenant-%s", tn.Name)
	ns := v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: nsName}}

	nsOpRes, err := ctrl.CreateOrUpdate(ctx, r.Client, &ns, func() error {
		updateTnNamespace(&ns, tn.Name)
		return ctrl.SetControllerReference(&tn, &ns, r.Scheme)
	})
	if err != nil {
		klog.Errorf("Unable to create or update namespace of tenant %s", tn.Name)
		klog.Error(err)
		tn.Status.PersonalNamespace.Created = false
		tn.Status.PersonalNamespace.Name = ""
		retrigErr = err
	} else {
		klog.Infof("Namespace %s for tenant %s %s", nsName, req.Name, nsOpRes)
		tn.Status.PersonalNamespace.Created = true
		tn.Status.PersonalNamespace.Name = nsName
	}
	klog.Info("SHAKALAKA")
	_, err = createOrUpdateKcUser(ctx, r.KcA, tn.Name, tn.Spec.FirstName, tn.Spec.LastName, tn.Spec.Email)
	if err != nil {
		klog.Errorf("Error when creating or updating user %s", tn.Name)
		klog.Error(err)
		retrigErr = err
		tn.Status.Subscriptions["keycloak"] = crownlabsv1alpha1.SubscrFailed
	} else {
		tn.Status.Subscriptions["keycloak"] = crownlabsv1alpha1.SubscrOk
	}
	klog.Info("BOMBBAAA")

	if err := r.Status().Update(ctx, &tn); err != nil {
		// if status update fails, still try to reconcile later
		klog.Error("Unable to update status before exiting reconciler", err)
		retrigErr = err
	}

	return ctrl.Result{}, retrigErr
}

func (r *TenantReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&crownlabsv1alpha1.Tenant{}).
		Complete(r)
}

// updateTnNamespace updates the tenant namespace
func updateTnNamespace(ns *v1.Namespace, tnName string) {
	if ns.Labels == nil {
		ns.Labels = make(map[string]string)
	}
	ns.Labels["crownlabs.polito.it/type"] = "tenant"
	ns.Labels["crownlabs.polito.it/name"] = tnName
}
