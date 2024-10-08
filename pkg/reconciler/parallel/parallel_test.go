/*
Copyright 2019 The Knative Authors

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

package parallel

import (
	"context"
	"fmt"
	"testing"

	eventingv1 "knative.dev/eventing/pkg/apis/eventing/v1"

	fakeeventingclient "knative.dev/eventing/pkg/client/injection/client/fake"
	fakedynamicclient "knative.dev/pkg/injection/clients/dynamicclient/fake"
	"knative.dev/pkg/tracker"

	"knative.dev/eventing/pkg/client/injection/reconciler/flows/v1/parallel"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgotesting "k8s.io/client-go/testing"

	eventingduckv1 "knative.dev/eventing/pkg/apis/duck/v1"
	eventingv1alpha1 "knative.dev/eventing/pkg/apis/eventing/v1alpha1"
	"knative.dev/eventing/pkg/apis/feature"
	"knative.dev/eventing/pkg/client/injection/ducks/duck/v1/channelable"
	"knative.dev/eventing/pkg/duck"
	"knative.dev/pkg/apis"
	duckv1 "knative.dev/pkg/apis/duck/v1"
	"knative.dev/pkg/configmap"
	"knative.dev/pkg/controller"
	"knative.dev/pkg/logging"
	logtesting "knative.dev/pkg/logging/testing"
	. "knative.dev/pkg/reconciler/testing"

	v1 "knative.dev/eventing/pkg/apis/flows/v1"

	messagingv1 "knative.dev/eventing/pkg/apis/messaging/v1"
	"knative.dev/eventing/pkg/reconciler/parallel/resources"
	. "knative.dev/eventing/pkg/reconciler/testing/v1"
)

const (
	testNS                 = "test-namespace"
	parallelName           = "test-parallel"
	replyChannelName       = "reply-channel"
	parallelGeneration     = 79
	readyEventPolicyName   = "test-event-policy-ready"
	unreadyEventPolicyName = "test-event-policy-unready"
)

var (
	subscriberGVK = metav1.GroupVersionKind{
		Group:   "messaging.knative.dev",
		Version: "v1",
		Kind:    "Subscription",
	}

	parallelGVK = metav1.GroupVersionKind{
		Group:   "flows.knative.dev",
		Version: "v1",
		Kind:    "Parallel",
	}

	channelV1GVK = metav1.GroupVersionKind{
		Group:   "messaging.knative.dev",
		Version: "v1",
		Kind:    "InMemoryChannel",
	}
)

func TestAllBranches(t *testing.T) {
	pKey := testNS + "/" + parallelName
	imc := &messagingv1.ChannelTemplateSpec{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "messaging.knative.dev/v1",
			Kind:       "InMemoryChannel",
		},
		Spec: &runtime.RawExtension{Raw: []byte("{}")},
	}

	table := TableTest{
		{
			Name: "bad workqueue key",
			// Make sure Reconcile handles bad keys.
			Key: "too/many/parts",
		}, {
			Name: "key not found",
			// Make sure Reconcile handles good keys that don't exist.
			Key: "foo/not-found",
		}, { // TODO: there is a bug in the controller, it will query for ""
			//			Name: "trigger key not found ",
			//			Objects: []runtime.Object{
			//				NewTrigger(triggerName, testNS),
			//			},
			//			Key:     "foo/incomplete",
			//			WantErr: true,
			//			WantEvents: []string{
			//				Eventf(corev1.EventTypeWarning, "ChannelReferenceFetchFailed", "Failed to validate spec.channel exists: s \"\" not found"),
			//			},
		}, {
			Name: "deleting",
			Key:  pKey,
			Objects: []runtime.Object{
				NewFlowsParallel(parallelName, testNS,
					WithInitFlowsParallelConditions,
					WithFlowsParallelDeleted)},
			WantErr: false,
		}, {
			Name: "single branch, no filter",
			Key:  pKey,
			Objects: []runtime.Object{
				NewFlowsParallel(parallelName, testNS,
					WithInitFlowsParallelConditions,
					WithFlowsParallelGeneration(parallelGeneration),
					WithFlowsParallelChannelTemplateSpec(imc),
					WithFlowsParallelBranches([]v1.ParallelBranch{
						{Subscriber: createSubscriber(0)},
					}))},
			WantErr: false,
			WantCreates: []runtime.Object{
				createChannel(parallelName),
				createBranchChannel(parallelName, 0),
				resources.NewFilterSubscription(0, NewFlowsParallel(parallelName, testNS, WithFlowsParallelChannelTemplateSpec(imc), WithFlowsParallelBranches([]v1.ParallelBranch{
					{Subscriber: createSubscriber(0)},
				}))),
				resources.NewSubscription(0, NewFlowsParallel(parallelName, testNS, WithFlowsParallelChannelTemplateSpec(imc), WithFlowsParallelBranches([]v1.ParallelBranch{
					{Subscriber: createSubscriber(0)},
				}))),
			},
			WantStatusUpdates: []clientgotesting.UpdateActionImpl{{
				Object: NewFlowsParallel(parallelName, testNS,
					WithInitFlowsParallelConditions,
					WithFlowsParallelGeneration(parallelGeneration),
					WithFlowsParallelStatusObservedGeneration(parallelGeneration),
					WithFlowsParallelChannelTemplateSpec(imc),
					WithFlowsParallelBranches([]v1.ParallelBranch{{Subscriber: createSubscriber(0)}}),
					WithFlowsParallelChannelsNotReady("ChannelsNotReady", "Channels are not ready yet, or there are none"),
					WithFlowsParallelAddressableNotReady("emptyAddress", "addressable is nil"),
					WithFlowsParallelSubscriptionsNotReady("SubscriptionsNotReady", "Subscriptions are not ready yet, or there are none"),
					WithFlowsParallelIngressChannelStatus(createParallelChannelStatus(parallelName, corev1.ConditionFalse)),
					WithFlowsParallelEventPoliciesReadyBecauseOIDCDisabled(),
					WithFlowsParallelBranchStatuses([]v1.ParallelBranchStatus{{
						FilterSubscriptionStatus: createParallelFilterSubscriptionStatus(parallelName, 0, corev1.ConditionFalse),
						FilterChannelStatus:      createParallelBranchChannelStatus(parallelName, 0, corev1.ConditionFalse),
						SubscriptionStatus:       createParallelSubscriptionStatus(parallelName, 0, corev1.ConditionFalse),
					}})),
			}},
		}, {
			Name: "single branch, with filter",
			Key:  pKey,
			Objects: []runtime.Object{
				NewFlowsParallel(parallelName, testNS,
					WithInitFlowsParallelConditions,
					WithFlowsParallelChannelTemplateSpec(imc),
					WithFlowsParallelBranches([]v1.ParallelBranch{
						{Filter: createFilter(0), Subscriber: createSubscriber(0)},
					}))},
			WantErr: false,
			WantCreates: []runtime.Object{
				createChannel(parallelName),
				createBranchChannel(parallelName, 0),
				resources.NewFilterSubscription(0, NewFlowsParallel(parallelName, testNS, WithFlowsParallelChannelTemplateSpec(imc), WithFlowsParallelBranches([]v1.ParallelBranch{
					{Filter: createFilter(0), Subscriber: createSubscriber(0)},
				}))),
				resources.NewSubscription(0, NewFlowsParallel(parallelName, testNS, WithFlowsParallelChannelTemplateSpec(imc), WithFlowsParallelBranches([]v1.ParallelBranch{
					{Filter: createFilter(0), Subscriber: createSubscriber(0)},
				}))),
			},
			WantStatusUpdates: []clientgotesting.UpdateActionImpl{{
				Object: NewFlowsParallel(parallelName, testNS,
					WithInitFlowsParallelConditions,
					WithFlowsParallelChannelTemplateSpec(imc),
					WithFlowsParallelBranches([]v1.ParallelBranch{{Filter: createFilter(0), Subscriber: createSubscriber(0)}}),
					WithFlowsParallelChannelsNotReady("ChannelsNotReady", "Channels are not ready yet, or there are none"),
					WithFlowsParallelAddressableNotReady("emptyAddress", "addressable is nil"),
					WithFlowsParallelSubscriptionsNotReady("SubscriptionsNotReady", "Subscriptions are not ready yet, or there are none"),
					WithFlowsParallelIngressChannelStatus(createParallelChannelStatus(parallelName, corev1.ConditionFalse)),
					WithFlowsParallelEventPoliciesReadyBecauseOIDCDisabled(),
					WithFlowsParallelBranchStatuses([]v1.ParallelBranchStatus{{
						FilterSubscriptionStatus: createParallelFilterSubscriptionStatus(parallelName, 0, corev1.ConditionFalse),
						FilterChannelStatus:      createParallelBranchChannelStatus(parallelName, 0, corev1.ConditionFalse),
						SubscriptionStatus:       createParallelSubscriptionStatus(parallelName, 0, corev1.ConditionFalse),
					}})),
			}},
		}, {
			Name: "single branch, with filter, with delivery",
			Key:  pKey,
			Objects: []runtime.Object{
				NewFlowsParallel(parallelName, testNS,
					WithInitFlowsParallelConditions,
					WithFlowsParallelChannelTemplateSpec(imc),
					WithFlowsParallelBranches([]v1.ParallelBranch{
						{Filter: createFilter(0), Subscriber: createSubscriber(0), Delivery: createDelivery(subscriberGVK, "dlc", testNS)},
					}))},
			WantErr: false,
			WantCreates: []runtime.Object{
				createChannel(parallelName),
				createBranchChannel(parallelName, 0),
				resources.NewFilterSubscription(0, NewFlowsParallel(parallelName, testNS, WithFlowsParallelChannelTemplateSpec(imc), WithFlowsParallelBranches([]v1.ParallelBranch{
					{Filter: createFilter(0), Subscriber: createSubscriber(0)},
				}))),
				resources.NewSubscription(0, NewFlowsParallel(parallelName, testNS, WithFlowsParallelChannelTemplateSpec(imc), WithFlowsParallelBranches([]v1.ParallelBranch{
					{Filter: createFilter(0), Subscriber: createSubscriber(0), Delivery: createDelivery(subscriberGVK, "dlc", testNS)},
				}))),
			},
			WantStatusUpdates: []clientgotesting.UpdateActionImpl{{
				Object: NewFlowsParallel(parallelName, testNS,
					WithInitFlowsParallelConditions,
					WithFlowsParallelChannelTemplateSpec(imc),
					WithFlowsParallelBranches([]v1.ParallelBranch{{Filter: createFilter(0), Subscriber: createSubscriber(0), Delivery: createDelivery(subscriberGVK, "dlc", testNS)}}),
					WithFlowsParallelChannelsNotReady("ChannelsNotReady", "Channels are not ready yet, or there are none"),
					WithFlowsParallelAddressableNotReady("emptyAddress", "addressable is nil"),
					WithFlowsParallelSubscriptionsNotReady("SubscriptionsNotReady", "Subscriptions are not ready yet, or there are none"),
					WithFlowsParallelIngressChannelStatus(createParallelChannelStatus(parallelName, corev1.ConditionFalse)),
					WithFlowsParallelEventPoliciesReadyBecauseOIDCDisabled(),
					WithFlowsParallelBranchStatuses([]v1.ParallelBranchStatus{{
						FilterSubscriptionStatus: createParallelFilterSubscriptionStatus(parallelName, 0, corev1.ConditionFalse),
						FilterChannelStatus:      createParallelBranchChannelStatus(parallelName, 0, corev1.ConditionFalse),
						SubscriptionStatus:       createParallelSubscriptionStatus(parallelName, 0, corev1.ConditionFalse),
					}})),
			}},
		}, {
			Name: "single branch, no filter, with global reply",
			Key:  pKey,
			Objects: []runtime.Object{
				NewFlowsParallel(parallelName, testNS,
					WithInitFlowsParallelConditions,
					WithFlowsParallelChannelTemplateSpec(imc),
					WithFlowsParallelReply(createReplyChannel(replyChannelName)),
					WithFlowsParallelBranches([]v1.ParallelBranch{
						{Subscriber: createSubscriber(0)},
					}))},
			WantErr: false,
			WantCreates: []runtime.Object{
				createChannel(parallelName),
				createBranchChannel(parallelName, 0),
				resources.NewFilterSubscription(0, NewFlowsParallel(parallelName, testNS, WithFlowsParallelChannelTemplateSpec(imc), WithFlowsParallelBranches([]v1.ParallelBranch{
					{Subscriber: createSubscriber(0)},
				}))),
				resources.NewSubscription(0, NewFlowsParallel(parallelName, testNS, WithFlowsParallelChannelTemplateSpec(imc), WithFlowsParallelBranches([]v1.ParallelBranch{
					{Subscriber: createSubscriber(0)},
				}), WithFlowsParallelReply(createReplyChannel(replyChannelName)))),
			},
			WantStatusUpdates: []clientgotesting.UpdateActionImpl{{
				Object: NewFlowsParallel(parallelName, testNS,
					WithInitFlowsParallelConditions,
					WithFlowsParallelChannelTemplateSpec(imc),
					WithFlowsParallelBranches([]v1.ParallelBranch{
						{Subscriber: createSubscriber(0)},
					}),
					WithFlowsParallelReply(createReplyChannel(replyChannelName)),
					WithFlowsParallelAddressableNotReady("emptyAddress", "addressable is nil"),
					WithFlowsParallelChannelsNotReady("ChannelsNotReady", "Channels are not ready yet, or there are none"),
					WithFlowsParallelSubscriptionsNotReady("SubscriptionsNotReady", "Subscriptions are not ready yet, or there are none"),
					WithFlowsParallelIngressChannelStatus(createParallelChannelStatus(parallelName, corev1.ConditionFalse)),
					WithFlowsParallelEventPoliciesReadyBecauseOIDCDisabled(),
					WithFlowsParallelBranchStatuses([]v1.ParallelBranchStatus{{
						FilterSubscriptionStatus: createParallelFilterSubscriptionStatus(parallelName, 0, corev1.ConditionFalse),
						FilterChannelStatus:      createParallelBranchChannelStatus(parallelName, 0, corev1.ConditionFalse),
						SubscriptionStatus:       createParallelSubscriptionStatus(parallelName, 0, corev1.ConditionFalse),
					}})),
			}},
		}, {
			Name: "single branch with reply, no filter, with case and global reply",
			Key:  pKey,
			Objects: []runtime.Object{
				NewFlowsParallel(parallelName, testNS,
					WithInitFlowsParallelConditions,
					WithFlowsParallelChannelTemplateSpec(imc),
					WithFlowsParallelReply(createReplyChannel(replyChannelName)),
					WithFlowsParallelBranches([]v1.ParallelBranch{
						{Subscriber: createSubscriber(0), Reply: createBranchReplyChannel(0)},
					}))},
			WantErr: false,
			WantCreates: []runtime.Object{
				createChannel(parallelName),
				createBranchChannel(parallelName, 0),
				resources.NewFilterSubscription(0, NewFlowsParallel(parallelName, testNS, WithFlowsParallelChannelTemplateSpec(imc), WithFlowsParallelBranches([]v1.ParallelBranch{
					{Subscriber: createSubscriber(0)},
				}))),
				resources.NewSubscription(0, NewFlowsParallel(parallelName, testNS, WithFlowsParallelChannelTemplateSpec(imc), WithFlowsParallelBranches([]v1.ParallelBranch{
					{Subscriber: createSubscriber(0), Reply: createBranchReplyChannel(0)},
				}), WithFlowsParallelReply(createReplyChannel(replyChannelName)))),
			},
			WantStatusUpdates: []clientgotesting.UpdateActionImpl{{
				Object: NewFlowsParallel(parallelName, testNS,
					WithInitFlowsParallelConditions,
					WithFlowsParallelChannelTemplateSpec(imc),
					WithFlowsParallelBranches([]v1.ParallelBranch{
						{Subscriber: createSubscriber(0), Reply: createBranchReplyChannel(0)},
					}),
					WithFlowsParallelReply(createReplyChannel(replyChannelName)),
					WithFlowsParallelAddressableNotReady("emptyAddress", "addressable is nil"),
					WithFlowsParallelChannelsNotReady("ChannelsNotReady", "Channels are not ready yet, or there are none"),
					WithFlowsParallelSubscriptionsNotReady("SubscriptionsNotReady", "Subscriptions are not ready yet, or there are none"),
					WithFlowsParallelIngressChannelStatus(createParallelChannelStatus(parallelName, corev1.ConditionFalse)),
					WithFlowsParallelEventPoliciesReadyBecauseOIDCDisabled(),
					WithFlowsParallelBranchStatuses([]v1.ParallelBranchStatus{{
						FilterSubscriptionStatus: createParallelFilterSubscriptionStatus(parallelName, 0, corev1.ConditionFalse),
						FilterChannelStatus:      createParallelBranchChannelStatus(parallelName, 0, corev1.ConditionFalse),
						SubscriptionStatus:       createParallelSubscriptionStatus(parallelName, 0, corev1.ConditionFalse),
					}})),
			}},
		}, {
			Name: "two branches, no filters",
			Key:  pKey,
			Objects: []runtime.Object{
				NewFlowsParallel(parallelName, testNS,
					WithInitFlowsParallelConditions,
					WithFlowsParallelChannelTemplateSpec(imc),
					WithFlowsParallelBranches([]v1.ParallelBranch{
						{Subscriber: createSubscriber(0)},
						{Subscriber: createSubscriber(1)},
					}))},
			WantErr: false,
			WantCreates: []runtime.Object{
				createChannel(parallelName),
				createBranchChannel(parallelName, 0),
				createBranchChannel(parallelName, 1),
				resources.NewFilterSubscription(0, NewFlowsParallel(parallelName, testNS, WithFlowsParallelChannelTemplateSpec(imc), WithFlowsParallelBranches([]v1.ParallelBranch{
					{Subscriber: createSubscriber(0)},
					{Subscriber: createSubscriber(1)},
				}))),
				resources.NewSubscription(0, NewFlowsParallel(parallelName, testNS, WithFlowsParallelChannelTemplateSpec(imc), WithFlowsParallelBranches([]v1.ParallelBranch{
					{Subscriber: createSubscriber(0)},
					{Subscriber: createSubscriber(1)},
				}))),
				resources.NewFilterSubscription(1, NewFlowsParallel(parallelName, testNS, WithFlowsParallelChannelTemplateSpec(imc), WithFlowsParallelBranches([]v1.ParallelBranch{
					{Subscriber: createSubscriber(0)},
					{Subscriber: createSubscriber(1)},
				}))),
				resources.NewSubscription(1, NewFlowsParallel(parallelName, testNS, WithFlowsParallelChannelTemplateSpec(imc), WithFlowsParallelBranches([]v1.ParallelBranch{
					{Subscriber: createSubscriber(0)},
					{Subscriber: createSubscriber(1)},
				})))},
			WantStatusUpdates: []clientgotesting.UpdateActionImpl{{
				Object: NewFlowsParallel(parallelName, testNS,
					WithInitFlowsParallelConditions,
					WithFlowsParallelChannelTemplateSpec(imc),
					WithFlowsParallelBranches([]v1.ParallelBranch{
						{Subscriber: createSubscriber(0)},
						{Subscriber: createSubscriber(1)},
					}),
					WithFlowsParallelChannelsNotReady("ChannelsNotReady", "Channels are not ready yet, or there are none"),
					WithFlowsParallelAddressableNotReady("emptyAddress", "addressable is nil"),
					WithFlowsParallelSubscriptionsNotReady("SubscriptionsNotReady", "Subscriptions are not ready yet, or there are none"),
					WithFlowsParallelIngressChannelStatus(createParallelChannelStatus(parallelName, corev1.ConditionFalse)),
					WithFlowsParallelEventPoliciesReadyBecauseOIDCDisabled(),
					WithFlowsParallelBranchStatuses([]v1.ParallelBranchStatus{
						{
							FilterSubscriptionStatus: createParallelFilterSubscriptionStatus(parallelName, 0, corev1.ConditionFalse),
							FilterChannelStatus:      createParallelBranchChannelStatus(parallelName, 0, corev1.ConditionFalse),
							SubscriptionStatus:       createParallelSubscriptionStatus(parallelName, 0, corev1.ConditionFalse),
						},
						{
							FilterSubscriptionStatus: createParallelFilterSubscriptionStatus(parallelName, 1, corev1.ConditionFalse),
							FilterChannelStatus:      createParallelBranchChannelStatus(parallelName, 1, corev1.ConditionFalse),
							SubscriptionStatus:       createParallelSubscriptionStatus(parallelName, 1, corev1.ConditionFalse),
						}})),
			}},
		}, {
			Name: "two branches with global reply",
			Key:  pKey,
			Objects: []runtime.Object{
				NewFlowsParallel(parallelName, testNS,
					WithInitFlowsParallelConditions,
					WithFlowsParallelChannelTemplateSpec(imc),
					WithFlowsParallelReply(createReplyChannel(replyChannelName)),
					WithFlowsParallelBranches([]v1.ParallelBranch{
						{Subscriber: createSubscriber(0)},
						{Subscriber: createSubscriber(1)},
					}))},
			WantErr: false,
			WantCreates: []runtime.Object{
				createChannel(parallelName),
				createBranchChannel(parallelName, 0),
				createBranchChannel(parallelName, 1),
				resources.NewFilterSubscription(0, NewFlowsParallel(parallelName, testNS, WithFlowsParallelChannelTemplateSpec(imc), WithFlowsParallelBranches([]v1.ParallelBranch{
					{Subscriber: createSubscriber(0)},
					{Subscriber: createSubscriber(1)},
				}))),
				resources.NewSubscription(0, NewFlowsParallel(parallelName, testNS, WithFlowsParallelChannelTemplateSpec(imc), WithFlowsParallelBranches([]v1.ParallelBranch{
					{Subscriber: createSubscriber(0)},
					{Subscriber: createSubscriber(1)},
				}), WithFlowsParallelReply(createReplyChannel(replyChannelName)))),
				resources.NewFilterSubscription(1, NewFlowsParallel(parallelName, testNS, WithFlowsParallelChannelTemplateSpec(imc), WithFlowsParallelBranches([]v1.ParallelBranch{
					{Subscriber: createSubscriber(0)},
					{Subscriber: createSubscriber(1)},
				}))),
				resources.NewSubscription(1, NewFlowsParallel(parallelName, testNS, WithFlowsParallelChannelTemplateSpec(imc), WithFlowsParallelBranches([]v1.ParallelBranch{
					{Subscriber: createSubscriber(0)},
					{Subscriber: createSubscriber(1)},
				}), WithFlowsParallelReply(createReplyChannel(replyChannelName)))),
			},
			WantStatusUpdates: []clientgotesting.UpdateActionImpl{{
				Object: NewFlowsParallel(parallelName, testNS,
					WithInitFlowsParallelConditions,
					WithFlowsParallelReply(createReplyChannel(replyChannelName)),
					WithFlowsParallelChannelTemplateSpec(imc),
					WithFlowsParallelBranches([]v1.ParallelBranch{
						{Subscriber: createSubscriber(0)},
						{Subscriber: createSubscriber(1)},
					}),
					WithFlowsParallelChannelsNotReady("ChannelsNotReady", "Channels are not ready yet, or there are none"),
					WithFlowsParallelAddressableNotReady("emptyAddress", "addressable is nil"),
					WithFlowsParallelSubscriptionsNotReady("SubscriptionsNotReady", "Subscriptions are not ready yet, or there are none"),
					WithFlowsParallelIngressChannelStatus(createParallelChannelStatus(parallelName, corev1.ConditionFalse)),
					WithFlowsParallelEventPoliciesReadyBecauseOIDCDisabled(),
					WithFlowsParallelBranchStatuses([]v1.ParallelBranchStatus{
						{
							FilterSubscriptionStatus: createParallelFilterSubscriptionStatus(parallelName, 0, corev1.ConditionFalse),
							FilterChannelStatus:      createParallelBranchChannelStatus(parallelName, 0, corev1.ConditionFalse),
							SubscriptionStatus:       createParallelSubscriptionStatus(parallelName, 0, corev1.ConditionFalse),
						},
						{
							FilterSubscriptionStatus: createParallelFilterSubscriptionStatus(parallelName, 1, corev1.ConditionFalse),
							FilterChannelStatus:      createParallelBranchChannelStatus(parallelName, 1, corev1.ConditionFalse),
							SubscriptionStatus:       createParallelSubscriptionStatus(parallelName, 1, corev1.ConditionFalse),
						}})),
			}},
		},
		{
			Name: "single branch, no filter, update subscription",
			Key:  pKey,
			Objects: []runtime.Object{
				NewFlowsParallel(parallelName, testNS,
					WithInitFlowsParallelConditions,
					WithFlowsParallelChannelTemplateSpec(imc),
					WithFlowsParallelBranches([]v1.ParallelBranch{
						{Subscriber: createSubscriber(1)},
					})),
				resources.NewSubscription(0, NewFlowsParallel(parallelName, testNS,
					WithFlowsParallelChannelTemplateSpec(imc),
					WithFlowsParallelBranches([]v1.ParallelBranch{
						{Subscriber: createSubscriber(0)},
					})))},
			WantErr: false,
			WantUpdates: []clientgotesting.UpdateActionImpl{
				{
					ActionImpl: clientgotesting.ActionImpl{
						Namespace: testNS,
						Resource:  v1.SchemeGroupVersion.WithResource("subscriptions"),
					},
					Object: resources.NewSubscription(0, NewFlowsParallel(parallelName, testNS,
						WithFlowsParallelChannelTemplateSpec(imc),
						WithFlowsParallelBranches([]v1.ParallelBranch{
							{Subscriber: createSubscriber(1)},
						}))),
				},
			},
			WantCreates: []runtime.Object{
				createChannel(parallelName),
				createBranchChannel(parallelName, 0),
				resources.NewFilterSubscription(0, NewFlowsParallel(parallelName, testNS, WithFlowsParallelChannelTemplateSpec(imc), WithFlowsParallelBranches([]v1.ParallelBranch{
					{Subscriber: createSubscriber(1)},
				}))),
			},
			WantStatusUpdates: []clientgotesting.UpdateActionImpl{{
				Object: NewFlowsParallel(parallelName, testNS,
					WithInitFlowsParallelConditions,
					WithFlowsParallelChannelTemplateSpec(imc),
					WithFlowsParallelBranches([]v1.ParallelBranch{{Subscriber: createSubscriber(1)}}),
					WithFlowsParallelChannelsNotReady("ChannelsNotReady", "Channels are not ready yet, or there are none"),
					WithFlowsParallelAddressableNotReady("emptyAddress", "addressable is nil"),
					WithFlowsParallelSubscriptionsNotReady("SubscriptionsNotReady", "Subscriptions are not ready yet, or there are none"),
					WithFlowsParallelIngressChannelStatus(createParallelChannelStatus(parallelName, corev1.ConditionFalse)),
					WithFlowsParallelEventPoliciesReadyBecauseOIDCDisabled(),
					WithFlowsParallelBranchStatuses([]v1.ParallelBranchStatus{{
						FilterSubscriptionStatus: createParallelFilterSubscriptionStatus(parallelName, 0, corev1.ConditionFalse),
						FilterChannelStatus:      createParallelBranchChannelStatus(parallelName, 0, corev1.ConditionFalse),
						SubscriptionStatus:       createParallelSubscriptionStatus(parallelName, 0, corev1.ConditionFalse),
					}})),
			}},
		},
		{
			Name: "two branches, update: remove one branch",
			Key:  pKey,
			Objects: []runtime.Object{
				NewFlowsParallel(parallelName, testNS,
					WithInitFlowsParallelConditions,
					WithFlowsParallelChannelTemplateSpec(imc),
					WithFlowsParallelBranches([]v1.ParallelBranch{
						{Subscriber: createSubscriber(0)},
					}),
					WithFlowsParallelChannelsNotReady("ChannelsNotReady", "Channels are not ready yet, or there are none"),
					WithFlowsParallelAddressableNotReady("emptyAddress", "addressable is nil"),
					WithFlowsParallelSubscriptionsNotReady("SubscriptionsNotReady", "Subscriptions are not ready yet, or there are none"),
					WithFlowsParallelIngressChannelStatus(createParallelChannelStatus(parallelName, corev1.ConditionFalse)),
					WithFlowsParallelBranchStatuses([]v1.ParallelBranchStatus{
						{
							FilterSubscriptionStatus: createParallelFilterSubscriptionStatus(parallelName, 0, corev1.ConditionFalse),
							FilterChannelStatus:      createParallelBranchChannelStatus(parallelName, 0, corev1.ConditionFalse),
							SubscriptionStatus:       createParallelSubscriptionStatus(parallelName, 0, corev1.ConditionFalse),
						},
						{
							FilterSubscriptionStatus: createParallelFilterSubscriptionStatus(parallelName, 1, corev1.ConditionFalse),
							FilterChannelStatus:      createParallelBranchChannelStatus(parallelName, 1, corev1.ConditionFalse),
							SubscriptionStatus:       createParallelSubscriptionStatus(parallelName, 1, corev1.ConditionFalse),
						},
					})),

				createChannel(parallelName),
				createBranchChannel(parallelName, 0),
				createBranchChannel(parallelName, 1),

				resources.NewSubscription(0, NewFlowsParallel(parallelName, testNS,
					WithFlowsParallelChannelTemplateSpec(imc),
					WithFlowsParallelBranches([]v1.ParallelBranch{
						{Subscriber: createSubscriber(0)},
						{Subscriber: createSubscriber(1)},
					}))),
				resources.NewFilterSubscription(0, NewFlowsParallel(parallelName, testNS,
					WithFlowsParallelChannelTemplateSpec(imc),
					WithFlowsParallelBranches([]v1.ParallelBranch{
						{Subscriber: createSubscriber(0)},
						{Subscriber: createSubscriber(1)},
					}))),

				resources.NewSubscription(1, NewFlowsParallel(parallelName, testNS,
					WithFlowsParallelChannelTemplateSpec(imc),
					WithFlowsParallelBranches([]v1.ParallelBranch{
						{Subscriber: createSubscriber(0)},
						{Subscriber: createSubscriber(1)},
					}))),
				resources.NewFilterSubscription(1, NewFlowsParallel(parallelName, testNS,
					WithFlowsParallelChannelTemplateSpec(imc),
					WithFlowsParallelBranches([]v1.ParallelBranch{
						{Subscriber: createSubscriber(0)},
						{Subscriber: createSubscriber(1)},
					}))),
			},
			WantErr: false,
			WantDeletes: []clientgotesting.DeleteActionImpl{
				{
					ActionImpl: clientgotesting.ActionImpl{
						Namespace: testNS,
						Resource:  v1.SchemeGroupVersion.WithResource("subscriptions"),
					},
					Name: resources.ParallelSubscriptionName(parallelName, 1),
				}, {
					ActionImpl: clientgotesting.ActionImpl{
						Namespace: testNS,
						Resource:  v1.SchemeGroupVersion.WithResource("subscriptions"),
					},
					Name: resources.ParallelFilterSubscriptionName(parallelName, 1),
				}, {
					ActionImpl: clientgotesting.ActionImpl{
						Namespace: testNS,
						Resource:  v1.SchemeGroupVersion.WithResource("inmemorychannels"),
					},
					Name: resources.ParallelBranchChannelName(parallelName, 1),
				},
			},
			WantStatusUpdates: []clientgotesting.UpdateActionImpl{{
				Object: NewFlowsParallel(parallelName, testNS,
					WithInitFlowsParallelConditions,
					WithFlowsParallelChannelTemplateSpec(imc),
					WithFlowsParallelBranches([]v1.ParallelBranch{
						{Subscriber: createSubscriber(0)},
					}),
					WithFlowsParallelChannelsNotReady("ChannelsNotReady", "Channels are not ready yet, or there are none"),
					WithFlowsParallelAddressableNotReady("emptyAddress", "addressable is nil"),
					WithFlowsParallelSubscriptionsNotReady("SubscriptionsNotReady", "Subscriptions are not ready yet, or there are none"),
					WithFlowsParallelIngressChannelStatus(createParallelChannelStatus(parallelName, corev1.ConditionFalse)),
					WithFlowsParallelEventPoliciesReadyBecauseOIDCDisabled(),
					WithFlowsParallelBranchStatuses([]v1.ParallelBranchStatus{{
						FilterSubscriptionStatus: createParallelFilterSubscriptionStatus(parallelName, 0, corev1.ConditionFalse),
						FilterChannelStatus:      createParallelBranchChannelStatus(parallelName, 0, corev1.ConditionFalse),
						SubscriptionStatus:       createParallelSubscriptionStatus(parallelName, 0, corev1.ConditionFalse),
					}})),
			}},
		}, {
			Name: "Should provision applying EventPolicies",
			Key:  pKey,
			Objects: []runtime.Object{
				NewFlowsParallel(parallelName, testNS,
					WithInitFlowsParallelConditions,
					WithFlowsParallelChannelTemplateSpec(imc),
					WithFlowsParallelBranches([]v1.ParallelBranch{
						{Filter: createFilter(0), Subscriber: createSubscriber(0)},
					})),
				NewEventPolicy(readyEventPolicyName, testNS,
					WithReadyEventPolicyCondition,
					WithEventPolicyToRef(parallelGVK, parallelName),
				),
			},
			WantErr: false,
			WantCreates: []runtime.Object{
				createChannel(parallelName),
				createBranchChannel(parallelName, 0),
				resources.NewFilterSubscription(0, NewFlowsParallel(parallelName, testNS, WithFlowsParallelChannelTemplateSpec(imc), WithFlowsParallelBranches([]v1.ParallelBranch{
					{Filter: createFilter(0), Subscriber: createSubscriber(0)},
				}))),
				resources.NewSubscription(0, NewFlowsParallel(parallelName, testNS, WithFlowsParallelChannelTemplateSpec(imc), WithFlowsParallelBranches([]v1.ParallelBranch{
					{Filter: createFilter(0), Subscriber: createSubscriber(0)},
				}))),
			},
			WantStatusUpdates: []clientgotesting.UpdateActionImpl{{
				Object: NewFlowsParallel(parallelName, testNS,
					WithInitFlowsParallelConditions,
					WithFlowsParallelChannelTemplateSpec(imc),
					WithFlowsParallelBranches([]v1.ParallelBranch{{Filter: createFilter(0), Subscriber: createSubscriber(0)}}),
					WithFlowsParallelChannelsNotReady("ChannelsNotReady", "Channels are not ready yet, or there are none"),
					WithFlowsParallelAddressableNotReady("emptyAddress", "addressable is nil"),
					WithFlowsParallelSubscriptionsNotReady("SubscriptionsNotReady", "Subscriptions are not ready yet, or there are none"),
					WithFlowsParallelIngressChannelStatus(createParallelChannelStatus(parallelName, corev1.ConditionFalse)),
					WithFlowsParallelEventPoliciesReady(),
					WithFlowsParallelEventPoliciesListed(readyEventPolicyName),
					WithFlowsParallelBranchStatuses([]v1.ParallelBranchStatus{{
						FilterSubscriptionStatus: createParallelFilterSubscriptionStatus(parallelName, 0, corev1.ConditionFalse),
						FilterChannelStatus:      createParallelBranchChannelStatus(parallelName, 0, corev1.ConditionFalse),
						SubscriptionStatus:       createParallelSubscriptionStatus(parallelName, 0, corev1.ConditionFalse),
					}})),
			}},
		}, {
			Name: "Should mark as NotReady on unready EventPolicies",
			Key:  pKey,
			Objects: []runtime.Object{
				NewFlowsParallel(parallelName, testNS,
					WithInitFlowsParallelConditions,
					WithFlowsParallelChannelTemplateSpec(imc),
					WithFlowsParallelBranches([]v1.ParallelBranch{
						{Filter: createFilter(0), Subscriber: createSubscriber(0)},
					})),
				NewEventPolicy(unreadyEventPolicyName, testNS,
					WithUnreadyEventPolicyCondition("", ""),
					WithEventPolicyToRef(parallelGVK, parallelName),
				),
			},
			WantErr: false,
			WantCreates: []runtime.Object{
				createChannel(parallelName),
				createBranchChannel(parallelName, 0),
				resources.NewFilterSubscription(0, NewFlowsParallel(parallelName, testNS, WithFlowsParallelChannelTemplateSpec(imc), WithFlowsParallelBranches([]v1.ParallelBranch{
					{Filter: createFilter(0), Subscriber: createSubscriber(0)},
				}))),
				resources.NewSubscription(0, NewFlowsParallel(parallelName, testNS, WithFlowsParallelChannelTemplateSpec(imc), WithFlowsParallelBranches([]v1.ParallelBranch{
					{Filter: createFilter(0), Subscriber: createSubscriber(0)},
				}))),
			},
			WantStatusUpdates: []clientgotesting.UpdateActionImpl{{
				Object: NewFlowsParallel(parallelName, testNS,
					WithInitFlowsParallelConditions,
					WithFlowsParallelChannelTemplateSpec(imc),
					WithFlowsParallelBranches([]v1.ParallelBranch{{Filter: createFilter(0), Subscriber: createSubscriber(0)}}),
					WithFlowsParallelChannelsNotReady("ChannelsNotReady", "Channels are not ready yet, or there are none"),
					WithFlowsParallelAddressableNotReady("emptyAddress", "addressable is nil"),
					WithFlowsParallelSubscriptionsNotReady("SubscriptionsNotReady", "Subscriptions are not ready yet, or there are none"),
					WithFlowsParallelIngressChannelStatus(createParallelChannelStatus(parallelName, corev1.ConditionFalse)),
					WithFlowsParallelEventPoliciesNotReady("EventPoliciesNotReady", fmt.Sprintf("event policies %s are not ready", unreadyEventPolicyName)),
					WithFlowsParallelBranchStatuses([]v1.ParallelBranchStatus{{
						FilterSubscriptionStatus: createParallelFilterSubscriptionStatus(parallelName, 0, corev1.ConditionFalse),
						FilterChannelStatus:      createParallelBranchChannelStatus(parallelName, 0, corev1.ConditionFalse),
						SubscriptionStatus:       createParallelSubscriptionStatus(parallelName, 0, corev1.ConditionFalse),
					}})),
			}},
		}, {
			Name: "should list only Ready EventPolicies",
			Key:  pKey,
			Objects: []runtime.Object{
				NewFlowsParallel(parallelName, testNS,
					WithInitFlowsParallelConditions,
					WithFlowsParallelChannelTemplateSpec(imc),
					WithFlowsParallelBranches([]v1.ParallelBranch{
						{Filter: createFilter(0), Subscriber: createSubscriber(0)},
					})),
				NewEventPolicy(unreadyEventPolicyName, testNS,
					WithUnreadyEventPolicyCondition("", ""),
					WithEventPolicyToRef(parallelGVK, parallelName),
				),
				NewEventPolicy(readyEventPolicyName, testNS,
					WithReadyEventPolicyCondition,
					WithEventPolicyToRef(parallelGVK, parallelName),
				),
			},
			WantErr: false,
			WantCreates: []runtime.Object{
				createChannel(parallelName),
				createBranchChannel(parallelName, 0),
				resources.NewFilterSubscription(0, NewFlowsParallel(parallelName, testNS, WithFlowsParallelChannelTemplateSpec(imc), WithFlowsParallelBranches([]v1.ParallelBranch{
					{Filter: createFilter(0), Subscriber: createSubscriber(0)},
				}))),
				resources.NewSubscription(0, NewFlowsParallel(parallelName, testNS, WithFlowsParallelChannelTemplateSpec(imc), WithFlowsParallelBranches([]v1.ParallelBranch{
					{Filter: createFilter(0), Subscriber: createSubscriber(0)},
				}))),
			},
			WantStatusUpdates: []clientgotesting.UpdateActionImpl{{
				Object: NewFlowsParallel(parallelName, testNS,
					WithInitFlowsParallelConditions,
					WithFlowsParallelChannelTemplateSpec(imc),
					WithFlowsParallelBranches([]v1.ParallelBranch{{Filter: createFilter(0), Subscriber: createSubscriber(0)}}),
					WithFlowsParallelChannelsNotReady("ChannelsNotReady", "Channels are not ready yet, or there are none"),
					WithFlowsParallelAddressableNotReady("emptyAddress", "addressable is nil"),
					WithFlowsParallelSubscriptionsNotReady("SubscriptionsNotReady", "Subscriptions are not ready yet, or there are none"),
					WithFlowsParallelIngressChannelStatus(createParallelChannelStatus(parallelName, corev1.ConditionFalse)),
					WithFlowsParallelEventPoliciesNotReady("EventPoliciesNotReady", fmt.Sprintf("event policies %s are not ready", unreadyEventPolicyName)),
					WithFlowsParallelEventPoliciesListed(readyEventPolicyName),
					WithFlowsParallelBranchStatuses([]v1.ParallelBranchStatus{{
						FilterSubscriptionStatus: createParallelFilterSubscriptionStatus(parallelName, 0, corev1.ConditionFalse),
						FilterChannelStatus:      createParallelBranchChannelStatus(parallelName, 0, corev1.ConditionFalse),
						SubscriptionStatus:       createParallelSubscriptionStatus(parallelName, 0, corev1.ConditionFalse),
					}})),
			}},
		}, {
			Name: "AuthZ Enabled with single branch, with filter, no EventPolicies",
			Key:  pKey,
			Objects: []runtime.Object{
				NewFlowsParallel(parallelName, testNS,
					WithInitFlowsParallelConditions,
					WithFlowsParallelChannelTemplateSpec(imc),
					WithFlowsParallelBranches([]v1.ParallelBranch{
						{Filter: createFilter(0), Subscriber: createSubscriber(0)},
					}))},
			WantErr: false,
			WantCreates: []runtime.Object{
				createChannel(parallelName),
				createBranchChannel(parallelName, 0),
				resources.NewFilterSubscription(0, NewFlowsParallel(parallelName, testNS, WithFlowsParallelChannelTemplateSpec(imc), WithFlowsParallelBranches([]v1.ParallelBranch{
					{Filter: createFilter(0), Subscriber: createSubscriber(0)},
				}))),
				resources.NewSubscription(0, NewFlowsParallel(parallelName, testNS, WithFlowsParallelChannelTemplateSpec(imc), WithFlowsParallelBranches([]v1.ParallelBranch{
					{Filter: createFilter(0), Subscriber: createSubscriber(0)},
				}))),
				makeEventPolicy(parallelName, resources.ParallelBranchChannelName(parallelName, 0), 0),
			},
			WantStatusUpdates: []clientgotesting.UpdateActionImpl{{
				Object: NewFlowsParallel(parallelName, testNS,
					WithInitFlowsParallelConditions,
					WithFlowsParallelChannelTemplateSpec(imc),
					WithFlowsParallelBranches([]v1.ParallelBranch{{Filter: createFilter(0), Subscriber: createSubscriber(0)}}),
					WithFlowsParallelChannelsNotReady("ChannelsNotReady", "Channels are not ready yet, or there are none"),
					WithFlowsParallelAddressableNotReady("emptyAddress", "addressable is nil"),
					WithFlowsParallelSubscriptionsNotReady("SubscriptionsNotReady", "Subscriptions are not ready yet, or there are none"),
					WithFlowsParallelIngressChannelStatus(createParallelChannelStatus(parallelName, corev1.ConditionFalse)),
					WithFlowsParallelEventPoliciesReadyBecauseNoPolicyAndOIDCEnabled(),
					WithFlowsParallelBranchStatuses([]v1.ParallelBranchStatus{{
						FilterSubscriptionStatus: createParallelFilterSubscriptionStatus(parallelName, 0, corev1.ConditionFalse),
						FilterChannelStatus:      createParallelBranchChannelStatus(parallelName, 0, corev1.ConditionFalse),
						SubscriptionStatus:       createParallelSubscriptionStatus(parallelName, 0, corev1.ConditionFalse),
					}})),
			}},
			Ctx: feature.ToContext(context.Background(), feature.Flags{
				feature.OIDCAuthentication:       feature.Enabled,
				feature.AuthorizationDefaultMode: feature.AuthorizationAllowSameNamespace,
			}),
		}, {
			Name: "AuthZ Enabled with single branch, with filter, with Parallel EventPolicy",
			Key:  pKey,
			Objects: []runtime.Object{
				NewFlowsParallel(parallelName, testNS,
					WithInitFlowsParallelConditions,
					WithFlowsParallelChannelTemplateSpec(imc),
					WithFlowsParallelBranches([]v1.ParallelBranch{
						{Filter: createFilter(0), Subscriber: createSubscriber(0)},
					})),
				NewEventPolicy(readyEventPolicyName, testNS,
					WithReadyEventPolicyCondition,
					WithEventPolicyToRef(parallelGVK, parallelName),
				),
			},
			WantErr: false,
			WantCreates: []runtime.Object{
				createChannel(parallelName),
				createBranchChannel(parallelName, 0),
				resources.NewFilterSubscription(0, NewFlowsParallel(parallelName, testNS, WithFlowsParallelChannelTemplateSpec(imc), WithFlowsParallelBranches([]v1.ParallelBranch{
					{Filter: createFilter(0), Subscriber: createSubscriber(0)},
				}))),
				resources.NewSubscription(0, NewFlowsParallel(parallelName, testNS, WithFlowsParallelChannelTemplateSpec(imc), WithFlowsParallelBranches([]v1.ParallelBranch{
					{Filter: createFilter(0), Subscriber: createSubscriber(0)},
				}))),
				makeEventPolicy(parallelName, resources.ParallelBranchChannelName(parallelName, 0), 0),
				makeIngressChannelEventPolicy(parallelName, resources.ParallelChannelName(parallelName), readyEventPolicyName),
			},
			WantStatusUpdates: []clientgotesting.UpdateActionImpl{{
				Object: NewFlowsParallel(parallelName, testNS,
					WithInitFlowsParallelConditions,
					WithFlowsParallelChannelTemplateSpec(imc),
					WithFlowsParallelBranches([]v1.ParallelBranch{{Filter: createFilter(0), Subscriber: createSubscriber(0)}}),
					WithFlowsParallelChannelsNotReady("ChannelsNotReady", "Channels are not ready yet, or there are none"),
					WithFlowsParallelAddressableNotReady("emptyAddress", "addressable is nil"),
					WithFlowsParallelSubscriptionsNotReady("SubscriptionsNotReady", "Subscriptions are not ready yet, or there are none"),
					WithFlowsParallelIngressChannelStatus(createParallelChannelStatus(parallelName, corev1.ConditionFalse)),
					WithFlowsParallelEventPoliciesReady(),
					WithFlowsParallelEventPoliciesListed(readyEventPolicyName),
					WithFlowsParallelBranchStatuses([]v1.ParallelBranchStatus{{
						FilterSubscriptionStatus: createParallelFilterSubscriptionStatus(parallelName, 0, corev1.ConditionFalse),
						FilterChannelStatus:      createParallelBranchChannelStatus(parallelName, 0, corev1.ConditionFalse),
						SubscriptionStatus:       createParallelSubscriptionStatus(parallelName, 0, corev1.ConditionFalse),
					}})),
			}},
			Ctx: feature.ToContext(context.Background(), feature.Flags{
				feature.OIDCAuthentication:       feature.Enabled,
				feature.AuthorizationDefaultMode: feature.AuthorizationAllowSameNamespace,
			}),
		}, {
			Name: "AuthZ Enabled two branches, no filters, no EventPolicy",
			Key:  pKey,
			Objects: []runtime.Object{
				NewFlowsParallel(parallelName, testNS,
					WithInitFlowsParallelConditions,
					WithFlowsParallelChannelTemplateSpec(imc),
					WithFlowsParallelBranches([]v1.ParallelBranch{
						{Subscriber: createSubscriber(0)},
						{Subscriber: createSubscriber(1)},
					}))},
			WantErr: false,
			WantCreates: []runtime.Object{
				createChannel(parallelName),
				createBranchChannel(parallelName, 0),
				createBranchChannel(parallelName, 1),
				resources.NewFilterSubscription(0, NewFlowsParallel(parallelName, testNS, WithFlowsParallelChannelTemplateSpec(imc), WithFlowsParallelBranches([]v1.ParallelBranch{
					{Subscriber: createSubscriber(0)},
					{Subscriber: createSubscriber(1)},
				}))),
				resources.NewSubscription(0, NewFlowsParallel(parallelName, testNS, WithFlowsParallelChannelTemplateSpec(imc), WithFlowsParallelBranches([]v1.ParallelBranch{
					{Subscriber: createSubscriber(0)},
					{Subscriber: createSubscriber(1)},
				}))),
				resources.NewFilterSubscription(1, NewFlowsParallel(parallelName, testNS, WithFlowsParallelChannelTemplateSpec(imc), WithFlowsParallelBranches([]v1.ParallelBranch{
					{Subscriber: createSubscriber(0)},
					{Subscriber: createSubscriber(1)},
				}))),
				resources.NewSubscription(1, NewFlowsParallel(parallelName, testNS, WithFlowsParallelChannelTemplateSpec(imc), WithFlowsParallelBranches([]v1.ParallelBranch{
					{Subscriber: createSubscriber(0)},
					{Subscriber: createSubscriber(1)},
				}))),
				makeEventPolicy(parallelName, resources.ParallelBranchChannelName(parallelName, 0), 0),
				makeEventPolicy(parallelName, resources.ParallelBranchChannelName(parallelName, 1), 1),
			},
			WantStatusUpdates: []clientgotesting.UpdateActionImpl{{
				Object: NewFlowsParallel(parallelName, testNS,
					WithInitFlowsParallelConditions,
					WithFlowsParallelChannelTemplateSpec(imc),
					WithFlowsParallelBranches([]v1.ParallelBranch{
						{Subscriber: createSubscriber(0)},
						{Subscriber: createSubscriber(1)},
					}),
					WithFlowsParallelChannelsNotReady("ChannelsNotReady", "Channels are not ready yet, or there are none"),
					WithFlowsParallelAddressableNotReady("emptyAddress", "addressable is nil"),
					WithFlowsParallelSubscriptionsNotReady("SubscriptionsNotReady", "Subscriptions are not ready yet, or there are none"),
					WithFlowsParallelIngressChannelStatus(createParallelChannelStatus(parallelName, corev1.ConditionFalse)),
					WithFlowsParallelEventPoliciesReadyBecauseNoPolicyAndOIDCEnabled(),
					WithFlowsParallelBranchStatuses([]v1.ParallelBranchStatus{
						{
							FilterSubscriptionStatus: createParallelFilterSubscriptionStatus(parallelName, 0, corev1.ConditionFalse),
							FilterChannelStatus:      createParallelBranchChannelStatus(parallelName, 0, corev1.ConditionFalse),
							SubscriptionStatus:       createParallelSubscriptionStatus(parallelName, 0, corev1.ConditionFalse),
						},
						{
							FilterSubscriptionStatus: createParallelFilterSubscriptionStatus(parallelName, 1, corev1.ConditionFalse),
							FilterChannelStatus:      createParallelBranchChannelStatus(parallelName, 1, corev1.ConditionFalse),
							SubscriptionStatus:       createParallelSubscriptionStatus(parallelName, 1, corev1.ConditionFalse),
						}})),
			}},
			Ctx: feature.ToContext(context.Background(), feature.Flags{
				feature.OIDCAuthentication:       feature.Enabled,
				feature.AuthorizationDefaultMode: feature.AuthorizationAllowSameNamespace,
			}),
		}, {
			Name: "AuthZ Enabled two branches, no filters, with EventPolicy",
			Key:  pKey,
			Objects: []runtime.Object{
				NewFlowsParallel(parallelName, testNS,
					WithInitFlowsParallelConditions,
					WithFlowsParallelChannelTemplateSpec(imc),
					WithFlowsParallelBranches([]v1.ParallelBranch{
						{Subscriber: createSubscriber(0)},
						{Subscriber: createSubscriber(1)},
					})),
				NewEventPolicy(readyEventPolicyName, testNS,
					WithReadyEventPolicyCondition,
					WithEventPolicyToRef(parallelGVK, parallelName),
				),
			},
			WantErr: false,
			WantCreates: []runtime.Object{
				createChannel(parallelName),
				createBranchChannel(parallelName, 0),
				createBranchChannel(parallelName, 1),
				resources.NewFilterSubscription(0, NewFlowsParallel(parallelName, testNS, WithFlowsParallelChannelTemplateSpec(imc), WithFlowsParallelBranches([]v1.ParallelBranch{
					{Subscriber: createSubscriber(0)},
					{Subscriber: createSubscriber(1)},
				}))),
				resources.NewSubscription(0, NewFlowsParallel(parallelName, testNS, WithFlowsParallelChannelTemplateSpec(imc), WithFlowsParallelBranches([]v1.ParallelBranch{
					{Subscriber: createSubscriber(0)},
					{Subscriber: createSubscriber(1)},
				}))),
				resources.NewFilterSubscription(1, NewFlowsParallel(parallelName, testNS, WithFlowsParallelChannelTemplateSpec(imc), WithFlowsParallelBranches([]v1.ParallelBranch{
					{Subscriber: createSubscriber(0)},
					{Subscriber: createSubscriber(1)},
				}))),
				resources.NewSubscription(1, NewFlowsParallel(parallelName, testNS, WithFlowsParallelChannelTemplateSpec(imc), WithFlowsParallelBranches([]v1.ParallelBranch{
					{Subscriber: createSubscriber(0)},
					{Subscriber: createSubscriber(1)},
				}))),
				makeEventPolicy(parallelName, resources.ParallelBranchChannelName(parallelName, 0), 0),
				makeEventPolicy(parallelName, resources.ParallelBranchChannelName(parallelName, 1), 1),
				makeIngressChannelEventPolicy(parallelName, resources.ParallelChannelName(parallelName), readyEventPolicyName),
			},
			WantStatusUpdates: []clientgotesting.UpdateActionImpl{{
				Object: NewFlowsParallel(parallelName, testNS,
					WithInitFlowsParallelConditions,
					WithFlowsParallelChannelTemplateSpec(imc),
					WithFlowsParallelBranches([]v1.ParallelBranch{
						{Subscriber: createSubscriber(0)},
						{Subscriber: createSubscriber(1)},
					}),
					WithFlowsParallelChannelsNotReady("ChannelsNotReady", "Channels are not ready yet, or there are none"),
					WithFlowsParallelAddressableNotReady("emptyAddress", "addressable is nil"),
					WithFlowsParallelSubscriptionsNotReady("SubscriptionsNotReady", "Subscriptions are not ready yet, or there are none"),
					WithFlowsParallelIngressChannelStatus(createParallelChannelStatus(parallelName, corev1.ConditionFalse)),
					WithFlowsParallelEventPoliciesReady(),
					WithFlowsParallelEventPoliciesListed(readyEventPolicyName),
					WithFlowsParallelBranchStatuses([]v1.ParallelBranchStatus{
						{
							FilterSubscriptionStatus: createParallelFilterSubscriptionStatus(parallelName, 0, corev1.ConditionFalse),
							FilterChannelStatus:      createParallelBranchChannelStatus(parallelName, 0, corev1.ConditionFalse),
							SubscriptionStatus:       createParallelSubscriptionStatus(parallelName, 0, corev1.ConditionFalse),
						},
						{
							FilterSubscriptionStatus: createParallelFilterSubscriptionStatus(parallelName, 1, corev1.ConditionFalse),
							FilterChannelStatus:      createParallelBranchChannelStatus(parallelName, 1, corev1.ConditionFalse),
							SubscriptionStatus:       createParallelSubscriptionStatus(parallelName, 1, corev1.ConditionFalse),
						}})),
			}},
			Ctx: feature.ToContext(context.Background(), feature.Flags{
				feature.OIDCAuthentication:       feature.Enabled,
				feature.AuthorizationDefaultMode: feature.AuthorizationAllowSameNamespace,
			}),
		},
		{
			Name: "two branches, update: remove one branch, with AuthZ enabled and parallel doesn't have EventPolicies",
			Key:  pKey,
			Objects: []runtime.Object{
				NewFlowsParallel(parallelName, testNS,
					WithInitFlowsParallelConditions,
					WithFlowsParallelChannelTemplateSpec(imc),
					WithFlowsParallelBranches([]v1.ParallelBranch{
						{Subscriber: createSubscriber(0)},
					}),
					WithFlowsParallelChannelsNotReady("ChannelsNotReady", "Channels are not ready yet, or there are none"),
					WithFlowsParallelAddressableNotReady("emptyAddress", "addressable is nil"),
					WithFlowsParallelSubscriptionsNotReady("SubscriptionsNotReady", "Subscriptions are not ready yet, or there are none"),
					WithFlowsParallelIngressChannelStatus(createParallelChannelStatus(parallelName, corev1.ConditionFalse)),
					WithFlowsParallelBranchStatuses([]v1.ParallelBranchStatus{
						{
							FilterSubscriptionStatus: createParallelFilterSubscriptionStatus(parallelName, 0, corev1.ConditionFalse),
							FilterChannelStatus:      createParallelBranchChannelStatus(parallelName, 0, corev1.ConditionFalse),
							SubscriptionStatus:       createParallelSubscriptionStatus(parallelName, 0, corev1.ConditionFalse),
						},
						{
							FilterSubscriptionStatus: createParallelFilterSubscriptionStatus(parallelName, 1, corev1.ConditionFalse),
							FilterChannelStatus:      createParallelBranchChannelStatus(parallelName, 1, corev1.ConditionFalse),
							SubscriptionStatus:       createParallelSubscriptionStatus(parallelName, 1, corev1.ConditionFalse),
						},
					})),

				createChannel(parallelName),
				createBranchChannel(parallelName, 0),
				createBranchChannel(parallelName, 1),

				resources.NewSubscription(0, NewFlowsParallel(parallelName, testNS,
					WithFlowsParallelChannelTemplateSpec(imc),
					WithFlowsParallelBranches([]v1.ParallelBranch{
						{Subscriber: createSubscriber(0)},
						{Subscriber: createSubscriber(1)},
					}))),
				resources.NewFilterSubscription(0, NewFlowsParallel(parallelName, testNS,
					WithFlowsParallelChannelTemplateSpec(imc),
					WithFlowsParallelBranches([]v1.ParallelBranch{
						{Subscriber: createSubscriber(0)},
						{Subscriber: createSubscriber(1)},
					}))),

				resources.NewSubscription(1, NewFlowsParallel(parallelName, testNS,
					WithFlowsParallelChannelTemplateSpec(imc),
					WithFlowsParallelBranches([]v1.ParallelBranch{
						{Subscriber: createSubscriber(0)},
						{Subscriber: createSubscriber(1)},
					}))),
				resources.NewFilterSubscription(1, NewFlowsParallel(parallelName, testNS,
					WithFlowsParallelChannelTemplateSpec(imc),
					WithFlowsParallelBranches([]v1.ParallelBranch{
						{Subscriber: createSubscriber(0)},
						{Subscriber: createSubscriber(1)},
					}))),

				makeEventPolicy(parallelName, resources.ParallelBranchChannelName(parallelName, 0), 0),
				makeEventPolicy(parallelName, resources.ParallelBranchChannelName(parallelName, 1), 1),
			},
			WantErr: false,
			WantDeletes: []clientgotesting.DeleteActionImpl{
				{
					ActionImpl: clientgotesting.ActionImpl{
						Namespace: testNS,
						Resource:  v1.SchemeGroupVersion.WithResource("subscriptions"),
					},
					Name: resources.ParallelSubscriptionName(parallelName, 1),
				}, {
					ActionImpl: clientgotesting.ActionImpl{
						Namespace: testNS,
						Resource:  v1.SchemeGroupVersion.WithResource("subscriptions"),
					},
					Name: resources.ParallelFilterSubscriptionName(parallelName, 1),
				}, {
					ActionImpl: clientgotesting.ActionImpl{
						Namespace: testNS,
						Resource:  v1.SchemeGroupVersion.WithResource("inmemorychannels"),
					},
					Name: resources.ParallelBranchChannelName(parallelName, 1),
				}, {
					ActionImpl: clientgotesting.ActionImpl{
						Namespace: testNS,
						Resource:  v1.SchemeGroupVersion.WithResource("eventpolicies"),
					},
					Name: resources.ParallelEventPolicyName(parallelName, resources.ParallelBranchChannelName(parallelName, 1)),
				},
			},
			WantStatusUpdates: []clientgotesting.UpdateActionImpl{{
				Object: NewFlowsParallel(parallelName, testNS,
					WithInitFlowsParallelConditions,
					WithFlowsParallelChannelTemplateSpec(imc),
					WithFlowsParallelBranches([]v1.ParallelBranch{
						{Subscriber: createSubscriber(0)},
					}),
					WithFlowsParallelChannelsNotReady("ChannelsNotReady", "Channels are not ready yet, or there are none"),
					WithFlowsParallelAddressableNotReady("emptyAddress", "addressable is nil"),
					WithFlowsParallelSubscriptionsNotReady("SubscriptionsNotReady", "Subscriptions are not ready yet, or there are none"),
					WithFlowsParallelIngressChannelStatus(createParallelChannelStatus(parallelName, corev1.ConditionFalse)),
					WithFlowsParallelEventPoliciesReadyBecauseNoPolicyAndOIDCEnabled(),
					WithFlowsParallelBranchStatuses([]v1.ParallelBranchStatus{{
						FilterSubscriptionStatus: createParallelFilterSubscriptionStatus(parallelName, 0, corev1.ConditionFalse),
						FilterChannelStatus:      createParallelBranchChannelStatus(parallelName, 0, corev1.ConditionFalse),
						SubscriptionStatus:       createParallelSubscriptionStatus(parallelName, 0, corev1.ConditionFalse),
					}})),
			}},
			Ctx: feature.ToContext(context.Background(), feature.Flags{
				feature.OIDCAuthentication:       feature.Enabled,
				feature.AuthorizationDefaultMode: feature.AuthorizationAllowSameNamespace,
			}),
		}, {
			Name: "three branches, update: remove one branch, with AuthZ enabled and parallel doesn't have EventPolicies",
			Key:  pKey,
			Objects: []runtime.Object{
				NewFlowsParallel(parallelName, testNS,
					WithInitFlowsParallelConditions,
					WithFlowsParallelChannelTemplateSpec(imc),
					WithFlowsParallelBranches([]v1.ParallelBranch{
						{Subscriber: createSubscriber(0)},
						{Subscriber: createSubscriber(1)},
					}),
					WithFlowsParallelChannelsNotReady("ChannelsNotReady", "Channels are not ready yet, or there are none"),
					WithFlowsParallelAddressableNotReady("emptyAddress", "addressable is nil"),
					WithFlowsParallelSubscriptionsNotReady("SubscriptionsNotReady", "Subscriptions are not ready yet, or there are none"),
					WithFlowsParallelIngressChannelStatus(createParallelChannelStatus(parallelName, corev1.ConditionFalse)),
					WithFlowsParallelBranchStatuses([]v1.ParallelBranchStatus{
						{
							FilterSubscriptionStatus: createParallelFilterSubscriptionStatus(parallelName, 0, corev1.ConditionFalse),
							FilterChannelStatus:      createParallelBranchChannelStatus(parallelName, 0, corev1.ConditionFalse),
							SubscriptionStatus:       createParallelSubscriptionStatus(parallelName, 0, corev1.ConditionFalse),
						},
						{
							FilterSubscriptionStatus: createParallelFilterSubscriptionStatus(parallelName, 1, corev1.ConditionFalse),
							FilterChannelStatus:      createParallelBranchChannelStatus(parallelName, 1, corev1.ConditionFalse),
							SubscriptionStatus:       createParallelSubscriptionStatus(parallelName, 1, corev1.ConditionFalse),
						},
						{
							FilterSubscriptionStatus: createParallelFilterSubscriptionStatus(parallelName, 2, corev1.ConditionFalse),
							FilterChannelStatus:      createParallelBranchChannelStatus(parallelName, 2, corev1.ConditionFalse),
							SubscriptionStatus:       createParallelSubscriptionStatus(parallelName, 2, corev1.ConditionFalse),
						},
					})),

				createChannel(parallelName),
				createBranchChannel(parallelName, 0),
				createBranchChannel(parallelName, 1),
				createBranchChannel(parallelName, 2),

				resources.NewSubscription(0, NewFlowsParallel(parallelName, testNS,
					WithFlowsParallelChannelTemplateSpec(imc),
					WithFlowsParallelBranches([]v1.ParallelBranch{
						{Subscriber: createSubscriber(0)},
						{Subscriber: createSubscriber(1)},
						{Subscriber: createSubscriber(2)},
					}))),
				resources.NewFilterSubscription(0, NewFlowsParallel(parallelName, testNS,
					WithFlowsParallelChannelTemplateSpec(imc),
					WithFlowsParallelBranches([]v1.ParallelBranch{
						{Subscriber: createSubscriber(0)},
						{Subscriber: createSubscriber(1)},
						{Subscriber: createSubscriber(2)},
					}))),

				resources.NewSubscription(1, NewFlowsParallel(parallelName, testNS,
					WithFlowsParallelChannelTemplateSpec(imc),
					WithFlowsParallelBranches([]v1.ParallelBranch{
						{Subscriber: createSubscriber(0)},
						{Subscriber: createSubscriber(1)},
						{Subscriber: createSubscriber(2)},
					}))),
				resources.NewFilterSubscription(1, NewFlowsParallel(parallelName, testNS,
					WithFlowsParallelChannelTemplateSpec(imc),
					WithFlowsParallelBranches([]v1.ParallelBranch{
						{Subscriber: createSubscriber(0)},
						{Subscriber: createSubscriber(1)},
						{Subscriber: createSubscriber(2)},
					}))),
				resources.NewSubscription(2, NewFlowsParallel(parallelName, testNS,
					WithFlowsParallelChannelTemplateSpec(imc),
					WithFlowsParallelBranches([]v1.ParallelBranch{
						{Subscriber: createSubscriber(0)},
						{Subscriber: createSubscriber(1)},
						{Subscriber: createSubscriber(2)},
					}))),
				resources.NewFilterSubscription(2, NewFlowsParallel(parallelName, testNS,
					WithFlowsParallelChannelTemplateSpec(imc),
					WithFlowsParallelBranches([]v1.ParallelBranch{
						{Subscriber: createSubscriber(0)},
						{Subscriber: createSubscriber(1)},
						{Subscriber: createSubscriber(2)},
					}))),
				makeEventPolicy(parallelName, resources.ParallelBranchChannelName(parallelName, 0), 0),
				makeEventPolicy(parallelName, resources.ParallelBranchChannelName(parallelName, 1), 1),
				makeEventPolicy(parallelName, resources.ParallelBranchChannelName(parallelName, 2), 2),
			},
			WantErr: false,
			WantDeletes: []clientgotesting.DeleteActionImpl{
				{
					ActionImpl: clientgotesting.ActionImpl{
						Namespace: testNS,
						Resource:  v1.SchemeGroupVersion.WithResource("subscriptions"),
					},
					Name: resources.ParallelSubscriptionName(parallelName, 2),
				}, {
					ActionImpl: clientgotesting.ActionImpl{
						Namespace: testNS,
						Resource:  v1.SchemeGroupVersion.WithResource("subscriptions"),
					},
					Name: resources.ParallelFilterSubscriptionName(parallelName, 2),
				}, {
					ActionImpl: clientgotesting.ActionImpl{
						Namespace: testNS,
						Resource:  v1.SchemeGroupVersion.WithResource("inmemorychannels"),
					},
					Name: resources.ParallelBranchChannelName(parallelName, 2),
				}, {
					ActionImpl: clientgotesting.ActionImpl{
						Namespace: testNS,
						Resource:  v1.SchemeGroupVersion.WithResource("eventpolicies"),
					},
					Name: resources.ParallelEventPolicyName(parallelName, resources.ParallelBranchChannelName(parallelName, 2)),
				},
			},
			WantStatusUpdates: []clientgotesting.UpdateActionImpl{{
				Object: NewFlowsParallel(parallelName, testNS,
					WithInitFlowsParallelConditions,
					WithFlowsParallelChannelTemplateSpec(imc),
					WithFlowsParallelBranches([]v1.ParallelBranch{
						{Subscriber: createSubscriber(0)},
						{Subscriber: createSubscriber(1)},
					}),
					WithFlowsParallelChannelsNotReady("ChannelsNotReady", "Channels are not ready yet, or there are none"),
					WithFlowsParallelAddressableNotReady("emptyAddress", "addressable is nil"),
					WithFlowsParallelSubscriptionsNotReady("SubscriptionsNotReady", "Subscriptions are not ready yet, or there are none"),
					WithFlowsParallelIngressChannelStatus(createParallelChannelStatus(parallelName, corev1.ConditionFalse)),
					WithFlowsParallelEventPoliciesReadyBecauseNoPolicyAndOIDCEnabled(),
					WithFlowsParallelBranchStatuses([]v1.ParallelBranchStatus{{
						FilterSubscriptionStatus: createParallelFilterSubscriptionStatus(parallelName, 0, corev1.ConditionFalse),
						FilterChannelStatus:      createParallelBranchChannelStatus(parallelName, 0, corev1.ConditionFalse),
						SubscriptionStatus:       createParallelSubscriptionStatus(parallelName, 0, corev1.ConditionFalse),
					}, {
						FilterSubscriptionStatus: createParallelFilterSubscriptionStatus(parallelName, 1, corev1.ConditionFalse),
						FilterChannelStatus:      createParallelBranchChannelStatus(parallelName, 1, corev1.ConditionFalse),
						SubscriptionStatus:       createParallelSubscriptionStatus(parallelName, 1, corev1.ConditionFalse),
					}})),
			}},
			Ctx: feature.ToContext(context.Background(), feature.Flags{
				feature.OIDCAuthentication:       feature.Enabled,
				feature.AuthorizationDefaultMode: feature.AuthorizationAllowSameNamespace,
			}),
		}, {
			Name: "Parallel Event Policy Deleted, corresponding Ingress Channel Policy should be deleted",
			Key:  pKey,
			Objects: []runtime.Object{
				NewFlowsParallel(parallelName, testNS,
					WithInitFlowsParallelConditions,
					WithFlowsParallelChannelTemplateSpec(imc),
					WithFlowsParallelBranches([]v1.ParallelBranch{
						{Filter: createFilter(0), Subscriber: createSubscriber(0)},
					})),
				NewEventPolicy(readyEventPolicyName, testNS,
					WithReadyEventPolicyCondition,
					WithEventPolicyToRef(parallelGVK, parallelName),
				),
				makeEventPolicy(parallelName, resources.ParallelBranchChannelName(parallelName, 0), 0),
				makeIngressChannelEventPolicy(parallelName, resources.ParallelChannelName(parallelName), readyEventPolicyName),
				makeIngressChannelEventPolicy(parallelName, resources.ParallelChannelName(parallelName), readyEventPolicyName+"-1"),
			},
			WantErr: false,
			WantCreates: []runtime.Object{
				createChannel(parallelName),
				createBranchChannel(parallelName, 0),
				resources.NewFilterSubscription(0, NewFlowsParallel(parallelName, testNS, WithFlowsParallelChannelTemplateSpec(imc), WithFlowsParallelBranches([]v1.ParallelBranch{
					{Filter: createFilter(0), Subscriber: createSubscriber(0)},
				}))),
				resources.NewSubscription(0, NewFlowsParallel(parallelName, testNS, WithFlowsParallelChannelTemplateSpec(imc), WithFlowsParallelBranches([]v1.ParallelBranch{
					{Filter: createFilter(0), Subscriber: createSubscriber(0)},
				}))),
			},
			WantDeletes: []clientgotesting.DeleteActionImpl{
				{
					ActionImpl: clientgotesting.ActionImpl{
						Namespace: testNS,
						Resource:  v1.SchemeGroupVersion.WithResource("eventpolicies"),
					},
					Name: resources.ParallelEventPolicyName(parallelName, readyEventPolicyName+"-1"),
				},
			},
			WantStatusUpdates: []clientgotesting.UpdateActionImpl{{
				Object: NewFlowsParallel(parallelName, testNS,
					WithInitFlowsParallelConditions,
					WithFlowsParallelChannelTemplateSpec(imc),
					WithFlowsParallelBranches([]v1.ParallelBranch{{Filter: createFilter(0), Subscriber: createSubscriber(0)}}),
					WithFlowsParallelChannelsNotReady("ChannelsNotReady", "Channels are not ready yet, or there are none"),
					WithFlowsParallelAddressableNotReady("emptyAddress", "addressable is nil"),
					WithFlowsParallelSubscriptionsNotReady("SubscriptionsNotReady", "Subscriptions are not ready yet, or there are none"),
					WithFlowsParallelIngressChannelStatus(createParallelChannelStatus(parallelName, corev1.ConditionFalse)),
					WithFlowsParallelEventPoliciesReady(),
					WithFlowsParallelEventPoliciesListed(readyEventPolicyName),
					WithFlowsParallelBranchStatuses([]v1.ParallelBranchStatus{{
						FilterSubscriptionStatus: createParallelFilterSubscriptionStatus(parallelName, 0, corev1.ConditionFalse),
						FilterChannelStatus:      createParallelBranchChannelStatus(parallelName, 0, corev1.ConditionFalse),
						SubscriptionStatus:       createParallelSubscriptionStatus(parallelName, 0, corev1.ConditionFalse),
					}})),
			}},
			Ctx: feature.ToContext(context.Background(), feature.Flags{
				feature.OIDCAuthentication:       feature.Enabled,
				feature.AuthorizationDefaultMode: feature.AuthorizationAllowSameNamespace,
			}),
		}, {
			Name: "Parallel with multiple event policies",
			Key:  pKey,
			Objects: []runtime.Object{
				NewFlowsParallel(parallelName, testNS,
					WithInitFlowsParallelConditions,
					WithFlowsParallelChannelTemplateSpec(imc),
					WithFlowsParallelBranches([]v1.ParallelBranch{
						{Filter: createFilter(0), Subscriber: createSubscriber(0)},
					})),
				NewEventPolicy(readyEventPolicyName, testNS,
					WithReadyEventPolicyCondition,
					WithEventPolicyToRef(parallelGVK, parallelName),
				),
				NewEventPolicy(readyEventPolicyName+"-1", testNS,
					WithReadyEventPolicyCondition,
					WithEventPolicyToRef(parallelGVK, parallelName),
				),
				NewEventPolicy(readyEventPolicyName+"-2", testNS,
					WithReadyEventPolicyCondition,
					WithEventPolicyToRef(parallelGVK, parallelName),
				),
			},
			WantErr: false,
			WantCreates: []runtime.Object{
				createChannel(parallelName),
				createBranchChannel(parallelName, 0),
				resources.NewFilterSubscription(0, NewFlowsParallel(parallelName, testNS, WithFlowsParallelChannelTemplateSpec(imc), WithFlowsParallelBranches([]v1.ParallelBranch{
					{Filter: createFilter(0), Subscriber: createSubscriber(0)},
				}))),
				resources.NewSubscription(0, NewFlowsParallel(parallelName, testNS, WithFlowsParallelChannelTemplateSpec(imc), WithFlowsParallelBranches([]v1.ParallelBranch{
					{Filter: createFilter(0), Subscriber: createSubscriber(0)},
				}))),
				makeEventPolicy(parallelName, resources.ParallelBranchChannelName(parallelName, 0), 0),
				makeIngressChannelEventPolicy(parallelName, resources.ParallelChannelName(parallelName), readyEventPolicyName),
				makeIngressChannelEventPolicy(parallelName, resources.ParallelChannelName(parallelName), readyEventPolicyName+"-1"),
				makeIngressChannelEventPolicy(parallelName, resources.ParallelChannelName(parallelName), readyEventPolicyName+"-2"),
			},
			WantStatusUpdates: []clientgotesting.UpdateActionImpl{{
				Object: NewFlowsParallel(parallelName, testNS,
					WithInitFlowsParallelConditions,
					WithFlowsParallelChannelTemplateSpec(imc),
					WithFlowsParallelBranches([]v1.ParallelBranch{{Filter: createFilter(0), Subscriber: createSubscriber(0)}}),
					WithFlowsParallelChannelsNotReady("ChannelsNotReady", "Channels are not ready yet, or there are none"),
					WithFlowsParallelAddressableNotReady("emptyAddress", "addressable is nil"),
					WithFlowsParallelSubscriptionsNotReady("SubscriptionsNotReady", "Subscriptions are not ready yet, or there are none"),
					WithFlowsParallelIngressChannelStatus(createParallelChannelStatus(parallelName, corev1.ConditionFalse)),
					WithFlowsParallelEventPoliciesReady(),
					WithFlowsParallelEventPoliciesListed(readyEventPolicyName, readyEventPolicyName+"-1", readyEventPolicyName+"-2"),
					WithFlowsParallelBranchStatuses([]v1.ParallelBranchStatus{{
						FilterSubscriptionStatus: createParallelFilterSubscriptionStatus(parallelName, 0, corev1.ConditionFalse),
						FilterChannelStatus:      createParallelBranchChannelStatus(parallelName, 0, corev1.ConditionFalse),
						SubscriptionStatus:       createParallelSubscriptionStatus(parallelName, 0, corev1.ConditionFalse),
					}})),
			}},
			Ctx: feature.ToContext(context.Background(), feature.Flags{
				feature.OIDCAuthentication:       feature.Enabled,
				feature.AuthorizationDefaultMode: feature.AuthorizationAllowSameNamespace,
			}),
		}, {
			Name: "Propagates Filter of Parallels EventPolicy to ingress channels EventPolicy",
			Key:  pKey,
			Objects: []runtime.Object{
				NewFlowsParallel(parallelName, testNS,
					WithInitFlowsParallelConditions,
					WithFlowsParallelChannelTemplateSpec(imc),
					WithFlowsParallelBranches([]v1.ParallelBranch{
						{Filter: createFilter(0), Subscriber: createSubscriber(0)},
					})),
				NewEventPolicy(readyEventPolicyName, testNS,
					WithReadyEventPolicyCondition,
					WithEventPolicyToRef(parallelGVK, parallelName),
					WithEventPolicyFilter(eventingv1.SubscriptionsAPIFilter{
						CESQL: "true",
					}),
				),
			},
			WantErr: false,
			WantCreates: []runtime.Object{
				createChannel(parallelName),
				createBranchChannel(parallelName, 0),
				resources.NewFilterSubscription(0, NewFlowsParallel(parallelName, testNS, WithFlowsParallelChannelTemplateSpec(imc), WithFlowsParallelBranches([]v1.ParallelBranch{
					{Filter: createFilter(0), Subscriber: createSubscriber(0)},
				}))),
				resources.NewSubscription(0, NewFlowsParallel(parallelName, testNS, WithFlowsParallelChannelTemplateSpec(imc), WithFlowsParallelBranches([]v1.ParallelBranch{
					{Filter: createFilter(0), Subscriber: createSubscriber(0)},
				}))),
				makeEventPolicy(parallelName, resources.ParallelBranchChannelName(parallelName, 0), 0),
				makeIngressChannelEventPolicy(parallelName, resources.ParallelChannelName(parallelName), readyEventPolicyName,
					WithEventPolicyFilter(eventingv1.SubscriptionsAPIFilter{
						CESQL: "true",
					})),
			},
			WantStatusUpdates: []clientgotesting.UpdateActionImpl{{
				Object: NewFlowsParallel(parallelName, testNS,
					WithInitFlowsParallelConditions,
					WithFlowsParallelChannelTemplateSpec(imc),
					WithFlowsParallelBranches([]v1.ParallelBranch{{Filter: createFilter(0), Subscriber: createSubscriber(0)}}),
					WithFlowsParallelChannelsNotReady("ChannelsNotReady", "Channels are not ready yet, or there are none"),
					WithFlowsParallelAddressableNotReady("emptyAddress", "addressable is nil"),
					WithFlowsParallelSubscriptionsNotReady("SubscriptionsNotReady", "Subscriptions are not ready yet, or there are none"),
					WithFlowsParallelIngressChannelStatus(createParallelChannelStatus(parallelName, corev1.ConditionFalse)),
					WithFlowsParallelEventPoliciesReady(),
					WithFlowsParallelEventPoliciesListed(readyEventPolicyName),
					WithFlowsParallelBranchStatuses([]v1.ParallelBranchStatus{{
						FilterSubscriptionStatus: createParallelFilterSubscriptionStatus(parallelName, 0, corev1.ConditionFalse),
						FilterChannelStatus:      createParallelBranchChannelStatus(parallelName, 0, corev1.ConditionFalse),
						SubscriptionStatus:       createParallelSubscriptionStatus(parallelName, 0, corev1.ConditionFalse),
					}})),
			}},
			Ctx: feature.ToContext(context.Background(), feature.Flags{
				feature.OIDCAuthentication:       feature.Enabled,
				feature.AuthorizationDefaultMode: feature.AuthorizationAllowSameNamespace,
			}),
		},
	}

	logger := logtesting.TestLogger(t)
	table.Test(t, MakeFactory(func(ctx context.Context, listers *Listers, cmw configmap.Watcher) controller.Reconciler {
		ctx = channelable.WithDuck(ctx)
		r := &Reconciler{
			parallelLister:     listers.GetParallelLister(),
			channelableTracker: duck.NewListableTrackerFromTracker(ctx, channelable.Get, tracker.New(func(types.NamespacedName) {}, 0)),
			subscriptionLister: listers.GetSubscriptionLister(),
			eventingClientSet:  fakeeventingclient.Get(ctx),
			dynamicClientSet:   fakedynamicclient.Get(ctx),
			eventPolicyLister:  listers.GetEventPolicyLister(),
		}
		return parallel.NewReconciler(ctx, logging.FromContext(ctx),
			fakeeventingclient.Get(ctx), listers.GetParallelLister(),
			controller.GetEventRecorder(ctx), r)
	}, false, logger))
}

func createBranchReplyChannel(caseNumber int) *duckv1.Destination {
	return &duckv1.Destination{
		Ref: &duckv1.KReference{
			APIVersion: "messaging.knative.dev/v1",
			Kind:       "InMemoryChannel",
			Name:       fmt.Sprintf("%s-case-%d", replyChannelName, caseNumber),
			Namespace:  testNS,
		},
	}
}

func createReplyChannel(channelName string) *duckv1.Destination {
	return &duckv1.Destination{
		Ref: &duckv1.KReference{
			APIVersion: "messaging.knative.dev/v1",
			Kind:       "InMemoryChannel",
			Name:       channelName,
			Namespace:  testNS,
		},
	}
}

func createChannel(parallelName string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "messaging.knative.dev/v1",
			"kind":       "InMemoryChannel",
			"metadata": map[string]interface{}{
				"creationTimestamp": nil,
				"namespace":         testNS,
				"name":              resources.ParallelChannelName(parallelName),
				"ownerReferences": []interface{}{
					map[string]interface{}{
						"apiVersion":         "flows.knative.dev/v1",
						"blockOwnerDeletion": true,
						"controller":         true,
						"kind":               "Parallel",
						"name":               parallelName,
						"uid":                "",
					},
				},
			},
			"spec": map[string]interface{}{},
		},
	}
}

func createBranchChannel(parallelName string, caseNumber int) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "messaging.knative.dev/v1",
			"kind":       "InMemoryChannel",
			"metadata": map[string]interface{}{
				"creationTimestamp": nil,
				"namespace":         testNS,
				"name":              resources.ParallelBranchChannelName(parallelName, caseNumber),
				"ownerReferences": []interface{}{
					map[string]interface{}{
						"apiVersion":         "flows.knative.dev/v1",
						"blockOwnerDeletion": true,
						"controller":         true,
						"kind":               "Parallel",
						"name":               parallelName,
						"uid":                "",
					},
				},
			},
			"spec": map[string]interface{}{},
		},
	}
}

func createParallelBranchChannelStatus(parallelName string, caseNumber int, status corev1.ConditionStatus) v1.ParallelChannelStatus {
	return v1.ParallelChannelStatus{
		Channel: corev1.ObjectReference{
			APIVersion: "messaging.knative.dev/v1",
			Kind:       "InMemoryChannel",
			Name:       resources.ParallelBranchChannelName(parallelName, caseNumber),
			Namespace:  testNS,
		},
		ReadyCondition: apis.Condition{
			Type:    apis.ConditionReady,
			Status:  status,
			Reason:  "NotAddressable",
			Message: "Channel is not addressable",
		},
	}
}

func createParallelChannelStatus(parallelName string, status corev1.ConditionStatus) v1.ParallelChannelStatus {
	return v1.ParallelChannelStatus{
		Channel: corev1.ObjectReference{
			APIVersion: "messaging.knative.dev/v1",
			Kind:       "InMemoryChannel",
			Name:       resources.ParallelChannelName(parallelName),
			Namespace:  testNS,
		},
		ReadyCondition: apis.Condition{
			Type:    apis.ConditionReady,
			Status:  status,
			Reason:  "NotAddressable",
			Message: "Channel is not addressable",
		},
	}
}

func createParallelFilterSubscriptionStatus(parallelName string, caseNumber int, status corev1.ConditionStatus) v1.ParallelSubscriptionStatus {
	return v1.ParallelSubscriptionStatus{
		Subscription: corev1.ObjectReference{
			APIVersion: "messaging.knative.dev/v1",
			Kind:       "Subscription",
			Name:       resources.ParallelFilterSubscriptionName(parallelName, caseNumber),
			Namespace:  testNS,
		},
	}
}

func createParallelSubscriptionStatus(parallelName string, caseNumber int, status corev1.ConditionStatus) v1.ParallelSubscriptionStatus {
	return v1.ParallelSubscriptionStatus{
		Subscription: corev1.ObjectReference{
			APIVersion: "messaging.knative.dev/v1",
			Kind:       "Subscription",
			Name:       resources.ParallelSubscriptionName(parallelName, caseNumber),
			Namespace:  testNS,
		},
	}
}

func createSubscriber(caseNumber int) duckv1.Destination {
	uri := apis.HTTP(fmt.Sprintf("example.com/%d", caseNumber))
	return duckv1.Destination{
		URI: uri,
	}
}

func createFilter(caseNumber int) *duckv1.Destination {
	uri := apis.HTTP(fmt.Sprintf("example.com/filter-%d", caseNumber))
	return &duckv1.Destination{
		URI: uri,
	}
}

func apiVersion(gvk metav1.GroupVersionKind) string {
	groupVersion := gvk.Version
	if gvk.Group != "" {
		groupVersion = gvk.Group + "/" + gvk.Version
	}
	return groupVersion
}

func createDelivery(gvk metav1.GroupVersionKind, name, namespace string) *eventingduckv1.DeliverySpec {
	return &eventingduckv1.DeliverySpec{
		DeadLetterSink: &duckv1.Destination{
			Ref: &duckv1.KReference{
				APIVersion: apiVersion(gvk),
				Kind:       gvk.Kind,
				Name:       name,
				Namespace:  namespace,
			},
		},
	}
}

func makeEventPolicy(parallelName, channelName string, branch int, opts ...EventPolicyOption) *eventingv1alpha1.EventPolicy {
	ep := NewEventPolicy(resources.ParallelEventPolicyName(parallelName, channelName), testNS,
		WithEventPolicyToRef(channelV1GVK, channelName),
		// from a subscription
		WithEventPolicyFrom(subscriberGVK, resources.ParallelFilterSubscriptionName(parallelName, branch), testNS),
		WithEventPolicyOwnerReferences([]metav1.OwnerReference{
			{
				APIVersion: "flows.knative.dev/v1",
				Kind:       "Parallel",
				Name:       parallelName,
			},
		}...),
		WithEventPolicyLabels(resources.LabelsForParallelChannelsEventPolicy(parallelName)),
	)

	for _, opt := range opts {
		opt(ep)
	}

	return ep
}

func makeIngressChannelEventPolicy(parallelName, channelName, parallelEventPolicyName string, opts ...EventPolicyOption) *eventingv1alpha1.EventPolicy {
	ep := NewEventPolicy(resources.ParallelEventPolicyName(parallelName, parallelEventPolicyName), testNS,
		WithEventPolicyToRef(channelV1GVK, channelName),
		// from a subscription
		WithEventPolicyOwnerReferences([]metav1.OwnerReference{
			{
				APIVersion: "flows.knative.dev/v1",
				Kind:       "Parallel",
				Name:       parallelName,
			}, {
				APIVersion: "eventing.knative.dev/v1alpha1",
				Kind:       "EventPolicy",
				Name:       parallelEventPolicyName,
			},
		}...),
		WithEventPolicyLabels(resources.LabelsForParallelChannelsEventPolicy(parallelName)),
	)

	for _, opt := range opts {
		opt(ep)
	}

	return ep
}
