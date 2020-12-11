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

package controllers

import (
	"context"
	"fmt"

	"github.com/Nerzal/gocloak/v7"
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
	if tn.Status.Subscriptions == nil {
		tn.Status.Subscriptions = make(map[string]crownlabsv1alpha1.SubscriptionStatus)
	}
	// inizialize keycloak subscription to pending
	tn.Status.Subscriptions["keycloak"] = crownlabsv1alpha1.SubscrPending

	klog.Infof("Reconciling tenant %s", req.Name)

	nsName := fmt.Sprintf("tenant-%s", tn.Name)
	ns := v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: nsName}}

	nsOpRes, err := ctrl.CreateOrUpdate(ctx, r.Client, &ns, func() error {
		updateTnNamespace(tn, &ns)
		return ctrl.SetControllerReference(&tn, &ns, r.Scheme)
	})
	if err != nil {
		klog.Errorf("Unable to create or update namespace of tenant %s", tn.Name)
		klog.Error(err)
		// update status of tenant with failed namespace creation
		tn.Status.PersonalNamespace.Created = false
		tn.Status.PersonalNamespace.Name = ""
		// return anyway the error to allow new reconcile, independently of outcome of status update
		if err := r.Status().Update(ctx, &tn); err != nil {
			klog.Error("Unable to update status after namespace creation failed", err)
		}
		return ctrl.Result{}, err
	}
	klog.Infof("Namespace %s for tenant %s %s", nsName, req.Name, nsOpRes)

	// update status of tenant with info about namespace, success
	tn.Status.PersonalNamespace.Created = true
	tn.Status.PersonalNamespace.Name = nsName

	// everything should went ok, update status before exiting reconcile
	if err := r.Status().Update(ctx, &tn); err != nil {
		// if status update fails, still try to reconcile later
		klog.Error("Unable to update status before exiting reconciler", err)
		return ctrl.Result{}, err
	}

	_, err = createOrUpdateKcUser(ctx, r.KcA, tn.Name, tn.Spec.FirstName, tn.Spec.LastName, tn.Spec.Email)
	if err != nil {
		klog.Errorf("Error when creating or updating user %s", tn.Name)
		klog.Error(err)
		return ctrl.Result{}, err
	}

	/*
		rolesToAdd := make([]gocloak.Role, len(tn.Spec.Workspaces))
		for i, ws := range tn.Spec.Workspaces {
			var wsRole string
			if ws.Role == crownlabsv1alpha1.Basic {
				wsRole = fmt.Sprintf("workspace-%s:user", ws.Name)
			} else if ws.Role == crownlabsv1alpha1.Admin {
				wsRole = fmt.Sprintf("workspace-%s:admin", ws.Name)
			}
			gotRole, err := r.KcA.Client.GetClientRole(ctx, r.KcA.Token.AccessToken, r.KcA.TargetRealm, r.KcA.TargetClientID, wsRole)
			if err != nil {
				klog.Errorf("Error when getting info on client role %s", wsRole)
				klog.Error(err)
				return ctrl.Result{}, err
			}
			rolesToAdd[i].ID = gotRole.ID
			rolesToAdd[i].Name = gotRole.Name
			klog.Info(*rolesToAdd[i].Name)
		}

		userCurrentRoles, err := r.KcA.Client.GetClientRolesByUserID(ctx, r.KcA.Token.AccessToken, r.KcA.TargetRealm, r.KcA.TargetClientID, *userID)
		if err != nil {
			klog.Errorf("Error when getting roles of user %s", tn.Name)
			klog.Error(err)
			return ctrl.Result{}, err
		}
		var rolesToDelete []gocloak.Role
		for _, v := range userCurrentRoles {
			if !contains(rolesToAdd, *v) {
				rolesToDelete = append(rolesToDelete, *v)
			}
		}

		// this is per se idempotent
		err = r.KcA.Client.DeleteClientRoleFromUser(ctx, r.KcA.Token.AccessToken, r.KcA.TargetRealm, r.KcA.TargetClientID, *userID, rolesToDelete)
		if err != nil {
			klog.Errorf("Error when adding user role %s", tn.Name)
			klog.Error(err)
			return ctrl.Result{}, err
		}

		// this is per se idempotent
		err = r.KcA.Client.AddClientRoleToUser(ctx, r.KcA.Token.AccessToken, r.KcA.TargetRealm, r.KcA.TargetClientID, *userID, rolesToAdd)
		if err != nil {
			klog.Errorf("Error when adding user role %s", tn.Name)
			klog.Error(err)
			return ctrl.Result{}, err
		}
	*/
	return ctrl.Result{}, nil
}

func (r *TenantReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&crownlabsv1alpha1.Tenant{}).
		Complete(r)
}

// updateTnNamespace updates the tenant namespace
func updateTnNamespace(tn crownlabsv1alpha1.Tenant, ns *v1.Namespace) {
	if ns.Labels == nil {
		ns.Labels = make(map[string]string)
	}
	ns.Labels["crownlabs.polito.it/type"] = "tenant"
	ns.Labels["crownlabs.polito.it/name"] = tn.Name
}

func createOrUpdateKcUser(ctx context.Context, kcA *KcActor, username string, firstName string, lastName string, email string) (*string, error) {

	tr := true
	fa := false
	reqActions := []string{"UPDATE_PASSWORD", "VERIFY_EMAIL"}

	usersFound, err := kcA.Client.GetUsers(ctx, kcA.Token.AccessToken, kcA.TargetRealm, gocloak.GetUsersParams{Username: &username})

	if err != nil {
		klog.Errorf("Error when trying to find user %s", username)
		klog.Error(err)
		return nil, err
	} else if len(usersFound) == 0 {
		newUser := gocloak.User{
			Username:        &username,
			FirstName:       &firstName,
			LastName:        &lastName,
			Email:           &email,
			Enabled:         &tr,
			RequiredActions: &reqActions,
			EmailVerified:   &fa,
		}
		// no users found, create it
		klog.Infof("User %s did not exists", username)
		newUserID, err := kcA.Client.CreateUser(ctx, kcA.Token.AccessToken, kcA.TargetRealm, newUser)
		if err != nil {
			klog.Errorf("Error when creating user %s", username)
			klog.Error(err)
			return nil, err
		}
		// 30 days (in seconds) life span for email action
		emailActionLifespan := 60 * 60 * 24 * 30
		err = kcA.Client.ExecuteActionsEmail(ctx, kcA.Token.AccessToken, kcA.TargetRealm, gocloak.ExecuteActionsEmail{
			UserID: &newUserID,
			// ClientID: &kcA.TargetClientID,
			Lifespan: &emailActionLifespan,
			Actions:  &[]string{"UPDATE_PASSWORD", "VERIFY_EMAIL"},
		})
		if err != nil {
			klog.Errorf("Error when sending email actions for user %s", username)
			klog.Error(err)
			return nil, err
		}

		klog.Infof("User %s created", username)
		return &newUserID, nil
	}
	//  else if len(usersFound) == 1 {
	// 	oldUser := *usersFound[0]
	// 	oldUser.FirstName = &firstName
	// 	oldUser.LastName = &lastName
	// 	if *oldUser.Email != email {
	// 		oldUser.Email = &email
	// 		oldUser.RequiredActions = &reqActions
	// 		oldUser.EmailVerified = &fa
	// 	}
	// 	err := kcA.Client.UpdateUser(ctx, kcA.Token.AccessToken, kcA.TargetRealm, oldUser)
	// 	if err != nil {
	// 		klog.Errorf("Error when updating user %s", username)
	// 		klog.Error(err)
	// 		return nil, err
	// 	}
	// 	klog.Infof("User %s updated", username)
	// 	return usersFound[0].ID, nil
	// }
	return nil, nil
}

func contains(arr []gocloak.Role, elm gocloak.Role) bool {
	for _, a := range arr {
		if *a.Name == *elm.Name {
			return true
		}
	}
	return false
}
