package tenant_controller

import (
	"context"
	"errors"
	"fmt"
	"strings"

	gocloak "github.com/Nerzal/gocloak/v7"
	"k8s.io/klog"
)

type KcActor struct {
	Client         gocloak.GoCloak
	Token          *gocloak.JWT
	TargetRealm    string
	TargetClientID string
}

func createKcRoles(ctx context.Context, kcA *KcActor, rolesToCreate []string) error {
	for _, newRoleName := range rolesToCreate {
		if err := createKcRole(ctx, kcA, newRoleName); err != nil {
			klog.Error("Could not create user role", newRoleName)
			return err
		}
	}
	return nil
}

func createKcRole(ctx context.Context, kcA *KcActor, newRoleName string) error {
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

func deleteKcRoles(ctx context.Context, kcA *KcActor, rolesToDelete []string) error {

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

func createOrUpdateKcUser(ctx context.Context, kcA *KcActor, username string, firstName string, lastName string, email string) (*string, error) {

	tr := true
	fa := false
	reqActions := []string{"UPDATE_PASSWORD", "VERIFY_EMAIL"}
	emailActionLifespan := 60 * 60 * 24 * 30

	usersFound, err := kcA.Client.GetUsers(ctx, kcA.Token.AccessToken, kcA.TargetRealm, gocloak.GetUsersParams{Username: &username})

	if err != nil {
		klog.Errorf("Error when trying to find user %s", username)
		klog.Error(err)
		return nil, err
	} else if len(usersFound) == 0 {
		// no existing users found, create a new one
		klog.Infof("User %s did not exists", username)
		newUser := gocloak.User{
			Username:        &username,
			FirstName:       &firstName,
			LastName:        &lastName,
			Email:           &email,
			Enabled:         &tr,
			RequiredActions: &reqActions,
			EmailVerified:   &fa,
		}
		newUserID, err := kcA.Client.CreateUser(ctx, kcA.Token.AccessToken, kcA.TargetRealm, newUser)
		if err != nil {
			klog.Errorf("Error when creating user %s", username)
			klog.Error(err)
			return nil, err
		}
		// 30 days (in seconds) life span for email action
		err = kcA.Client.ExecuteActionsEmail(ctx, kcA.Token.AccessToken, kcA.TargetRealm, gocloak.ExecuteActionsEmail{
			UserID:   &newUserID,
			Lifespan: &emailActionLifespan,
			Actions:  &reqActions,
		})
		if err != nil {
			klog.Errorf("Error when sending email actions for user %s", username)
			klog.Error(err)
			return nil, err
		}

		klog.Infof("User %s created", username)
		return &newUserID, nil
	} else if len(usersFound) == 1 {
		oldUser := *usersFound[0]
		oldUser.FirstName = &firstName
		oldUser.LastName = &lastName
		resendVerif := false
		if *oldUser.Email != email {
			oldUser.EmailVerified = &fa
			resendVerif = true
		}
		oldUser.Email = &email
		err := kcA.Client.UpdateUser(ctx, kcA.Token.AccessToken, kcA.TargetRealm, oldUser)
		if err != nil {
			klog.Errorf("Error when updating user %s", username)
			klog.Error(err)
			return nil, err
		}
		if resendVerif {
			if err = kcA.Client.ExecuteActionsEmail(ctx, kcA.Token.AccessToken, kcA.TargetRealm, gocloak.ExecuteActionsEmail{
				UserID:   usersFound[0].ID,
				Lifespan: &emailActionLifespan,
				Actions:  &reqActions,
			}); err != nil {
				klog.Errorf("Error when sending email verification user %s", username)
				klog.Error(err)
				return nil, err
			}
			klog.Info("Sent user confirmation cause email has been updated")
		}
		klog.Infof("User %s updated", username)
		return usersFound[0].ID, nil
	}
	return nil, fmt.Errorf("Error when getting user %s", username)
}
