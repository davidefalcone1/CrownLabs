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
	"time"

	gocloak "github.com/Nerzal/gocloak/v7"
	"github.com/golang/mock/gomock"
	crownlabsv1alpha1 "github.com/netgroup-polito/CrownLabs/operators/api/v1alpha1"
	"github.com/netgroup-polito/CrownLabs/operators/pkg/tenant-controller/mocks"
	. "github.com/onsi/ginkgo"

	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("Tenant controller", func() {
	// Define utility constants for object names and testing timeouts/durations and intervals.
	var (
		tnName      = "mariorossi"
		tnFirstName = "mario"
		tnLastName  = "rossi"
		tnID        = "123456"
		tnEmail     = "mario.rossi@email.com"
		userID      = "userID"
		tr          = true
		fa          = false
	)

	const (
		tnNamespace = ""
		nsNamespace = ""
		timeout     = time.Second * 10
		interval    = time.Millisecond * 250
		nsName      = "tenant-mariorossi"
	)

	BeforeEach(func() {

		mockCtrl := gomock.NewController(GinkgoT())
		mKcClient = nil
		mKcClient = mocks.NewMockGoCloak(mockCtrl)
		kcA.Client = mKcClient

		// the user did not exist
		mKcClient.EXPECT().GetUsers(
			gomock.AssignableToTypeOf(context.Background()),
			gomock.Eq(kcAccessToken),
			gomock.Eq(kcTargetRealm),
			gomock.Eq(gocloak.GetUsersParams{Username: &tnName}),
		).Return([]*gocloak.User{}, nil).MinTimes(1).MaxTimes(2)

		mKcClient.EXPECT().CreateUser(
			gomock.AssignableToTypeOf(context.Background()),
			gomock.Eq(kcAccessToken),
			gomock.Eq(kcTargetRealm),
			gomock.Eq(
				gocloak.User{
					Username:      &tnName,
					FirstName:     &tnFirstName,
					LastName:      &tnLastName,
					Email:         &tnEmail,
					Enabled:       &tr,
					EmailVerified: &fa,
				}),
		).Return(userID, nil).MinTimes(1).MaxTimes(2)

		mKcClient.EXPECT().ExecuteActionsEmail(
			gomock.AssignableToTypeOf(context.Background()),
			gomock.Eq(kcAccessToken),
			gomock.Eq(kcTargetRealm),
			gomock.Eq(gocloak.ExecuteActionsEmail{
				UserID:   &userID,
				Lifespan: &emailActionLifespan,
				Actions:  &reqActions,
			})).Return(nil).MinTimes(1).MaxTimes(2)
	})

	It("Should create the related resources when creating a tenant", func() {

		By("By creating a tenant")
		ctx := context.Background()
		tn := &crownlabsv1alpha1.Tenant{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "crownlabs.polito.it/v1alpha1",
				Kind:       "Tenant",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      tnName,
				Namespace: tnNamespace,
			},
			Spec: crownlabsv1alpha1.TenantSpec{
				FirstName: tnFirstName,
				LastName:  tnLastName,
				ID:        tnID,
				Email:     tnEmail,
			},
		}
		Expect(k8sClient.Create(ctx, tn)).Should(Succeed())

		By("By checking that the tenant has been created")
		tnLookupKey := types.NamespacedName{Name: tnName, Namespace: tnNamespace}
		createdTn := &crownlabsv1alpha1.Tenant{}

		doesEventuallyExists(ctx, tnLookupKey, createdTn, BeTrue(), timeout, interval)

		By("By checking that the corresponding namespace has been created")

		nsLookupKey := types.NamespacedName{Name: nsName, Namespace: nsNamespace}
		createdNs := &v1.Namespace{}

		doesEventuallyExists(ctx, nsLookupKey, createdNs, BeTrue(), timeout, interval)

		By("By checking that the corresponding namespace has a controller reference pointing to the tenant")

		Expect(createdNs.OwnerReferences).Should(ContainElement(MatchFields(IgnoreExtras, Fields{"Name": Equal(tnName)})))
		Expect(createdNs.Labels).Should(HaveKeyWithValue("crownlabs.polito.it/type", "tenant"))

		By("By checking that the status of the tenant has been updated accordingly")

		Eventually(func() bool {
			err := k8sClient.Get(ctx, tnLookupKey, tn)
			if err != nil {
				return false
			}
			if !tn.Status.PersonalNamespace.Created || tn.Status.PersonalNamespace.Name != nsName {
				return false
			}
			return true
		}, timeout, interval).Should(BeTrue())
	})

})
