package tenant_controller

import (
	"context"
	"errors"
	"strings"

	gocloak "github.com/Nerzal/gocloak/v7"
	"k8s.io/klog"
)

// KcActor contains the needed objects and infos to use keycloak functionalities
type KcActor struct {
	Client         gocloak.GoCloak
	Token          *gocloak.JWT
	TargetRealm    string
	TargetClientID string
}

// temp to add a new commit

func (kcA *KcActor) createKcRoles(ctx context.Context, rolesToCreate []string) error {
	for _, newRoleName := range rolesToCreate {
		if err := kcA.createKcRole(ctx, newRoleName); err != nil {
			klog.Error("Could not create user role", newRoleName)
			return err
		}
	}
	return nil
}

func (kcA *KcActor) createKcRole(ctx context.Context, newRoleName string) error {
	// check if keycloak role already esists

	role, err := kcA.Client.GetClientRole(ctx, kcA.Token.AccessToken, kcA.TargetRealm, kcA.TargetClientID, newRoleName)
	if err != nil && strings.Contains(err.Error(), "Could not find role") {
		// role didn't exist
		// need to create new role
		klog.Infof("Role didn't exist %s", newRoleName)
		tr := true
		createdRoleName, err := kcA.Client.CreateClientRole(ctx, kcA.Token.AccessToken, kcA.TargetRealm, kcA.TargetClientID, gocloak.Role{Name: &newRoleName, ClientRole: &tr})
		if err != nil {
			klog.Error("Error when creating role", err)
			return err
		}
		klog.Infof("Role created %s", createdRoleName)
		return nil
	} else if err != nil {
		klog.Error("Error when getting user role", err)
		return err
	} else if *role.Name == newRoleName {
		klog.Infof("Role already existed %s", newRoleName)
		return nil
	}
	klog.Errorf("Error when getting role %s", newRoleName)
	return errors.New("Something went wrong when getting a role")
}

func (kcA *KcActor) deleteKcRoles(ctx context.Context, rolesToDelete []string) error {

	for _, role := range rolesToDelete {
		if err := kcA.Client.DeleteClientRole(ctx, kcA.Token.AccessToken, kcA.TargetRealm, kcA.TargetClientID, role); err != nil {
			if !strings.Contains(err.Error(), "404") {
				klog.Error("Could not delete user role", role)
				return err
			}
		}
	}
	return nil
}
