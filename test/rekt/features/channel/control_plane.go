/*
Copyright 2021 The Knative Authors

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

package channel

import (
	"context"
	"encoding/json"
	"knative.dev/eventing/test/rekt/resources/subscription"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"knative.dev/pkg/apis"
	"knative.dev/pkg/apis/duck"
	"knative.dev/reconciler-test/pkg/environment"
	"knative.dev/reconciler-test/pkg/feature"
	"knative.dev/reconciler-test/pkg/resources/service"

	duckv1 "knative.dev/eventing/pkg/apis/duck/v1"
	"knative.dev/eventing/pkg/apis/messaging"
	"knative.dev/eventing/test/rekt/features/knconf"
	"knative.dev/eventing/test/rekt/resources/account_role"
	"knative.dev/eventing/test/rekt/resources/channel_impl"
	"knative.dev/eventing/test/rekt/resources/delivery"
)

func ControlPlaneConformance(channelName string) *feature.FeatureSet {
	fs := &feature.FeatureSet{
		Name: "Knative Channel Specification - Control Plane",
		Features: []*feature.Feature{
			ControlPlaneChannel(channelName),
		},
	}

	return fs
}

func ControlPlaneChannel(channelName string) *feature.Feature {
	f := feature.NewFeatureNamed("Conformance")

	f.Setup("Set Channel Name", setChannelableName(channelName))

	sacmName := feature.MakeRandomK8sName("channelable-manipulator")
	f.Setup("Create Service Account for Channelable Manipulator",
		account_role.Install(sacmName, account_role.AsChannelableManipulator))

	saarName := feature.MakeRandomK8sName("addressale-resolver")
	f.Setup("Create Service Account for Addressable Resolver",
		account_role.Install(saarName, account_role.AsAddressableResolver))

	f.Stable("Aggregated Channelable Manipulator ClusterRole").
		Must("Every CRD MUST create a corresponding ClusterRole, that will be aggregated into the channelable-manipulator ClusterRole."+
			"This ClusterRole MUST include permissions to create, get, list, watch, patch, update and delete the CRD's custom objects and their status. "+
			"Each channel MUST have the duck.knative.dev/channelable: \"true\" label on its channelable-manipulator ClusterRole.",
			serviceAccountIsChannelableManipulator(sacmName))

	f.Stable("Aggregated Addressable Resolver ClusterRole").
		Must("Every CRD MUST create a corresponding ClusterRole, that will be aggregated into the addressable-resolver ClusterRole. "+
			"This ClusterRole MUST include permissions to get, list, and watch the CRD's custom objects and their status. "+
			"Each channel MUST have the duck.knative.dev/addressable: \"true\" label on its addressable-resolver ClusterRole.",
			serviceAccountIsAddressableResolver(saarName))

	f.Stable("CustomResourceDefinition per Channel").
		Must("Each channel is namespaced", crdOfChannelIsNamespaced).
		Must("label of messaging.knative.dev/subscribable: true",
			crdOfChannelIsLabeled(messaging.SubscribableDuckVersionAnnotation, "true")).
		Must("label of duck.knative.dev/addressable: true",
			crdOfChannelIsLabeled(duck.AddressableDuckVersionLabel, "true")).
		Must("The category `channel`", crdOfChannelHasCategory("channel"))

	f.Stable("Annotation Requirements").
		Should("each instance SHOULD have annotation: messaging.knative.dev/subscribable: v1",
			channelHasAnnotations)
	subscriptionName := feature.MakeRandomK8sName("subscription")
	f.Setup("install subscriber", subscription.Install(subscriptionName, subscription.WithChannel(channel_impl.AsRef(channelName)), subscription.WithSubscriber(nil, "http://example.com", "")))

	f.Stable("Spec and Status Requirements for Subscribers").
		Must("Each channel CRD MUST contain an array of subscribers: spec.subscribers. "+
			"Each channel CRD MUST have a status subresource which contains [subscribers (as an array)]. "+
			"The ready field of the subscriber identified by its uid MUST be set to True when the subscription is ready to be processed.",
			channelAllowsSubscribersAndStatus(subscriptionName))
	// Special note for Channel tests: The array of subscribers MUST NOT be
	// set directly on the generic Channel custom object, but rather
	// appended to the backing channel by the subscription itself.

	f.Stable("Status Requirements").
		Must("observedGeneration MUST be populated if present",
			knconf.KResourceHasObservedGeneration(channel_impl.GVR(), channelName)).
		Should("SHOULD have in status observedGeneration. "+
			"SHOULD have in status conditions (as an array)",
			knconf.KResourceHasReadyInConditions(channel_impl.GVR(), channelName)).
		Should("status.conditions SHOULD indicate status transitions and error reasons if present",
			todo) // how to test this?

	cName := feature.MakeRandomK8sName("channel")
	sink := feature.MakeRandomK8sName("sink")

	f.Setup("install a service", service.Install(sink,
		service.WithSelectors(map[string]string{"app": "rekt"})))
	f.Setup("update Channel", channel_impl.Install(cName, delivery.WithDeadLetterSink(service.AsKReference(sink), "")))
	f.Setup("Channel goes ready", channel_impl.IsReady(cName))
	f.Setup("Channel is addressable", channel_impl.IsAddressable(cName))

	f.Requirement("Channel has dead letter sink URI in status", channel_impl.HasDeadLetterSinkURI(cName, channel_impl.GVR()))

	f.Stable("Channel Status").
		Must("When the channel instance is ready to receive events status.address.url MUST be populated. "+
			"Each Channel CRD MUST have a status subresource which contains [address]. "+
			"When the Channel instance is ready to receive events status.address.url status.addressable MUST be set to True",
			readyChannelIsAddressable).
		Should("Set the Channel status.deadLetterSinkURI if there is a valid spec.delivery.deadLetterSink defined",
			readyChannelWithDLSHaveStatusUpdated(cName))

	return f
}

func serviceAccountIsChannelableManipulator(name string) feature.StepFn {
	return func(ctx context.Context, t feature.T) {
		gvr := channel_impl.GVR()
		for _, verb := range []string{"get", "list", "watch", "update", "patch", "delete"} {
			ServiceAccountSubjectAccessReviewAllowedOrFail(ctx, t, gvr, "", name, verb)
			ServiceAccountSubjectAccessReviewAllowedOrFail(ctx, t, gvr, "status", name, verb)
		}
	}
}

func serviceAccountIsAddressableResolver(name string) feature.StepFn {
	return func(ctx context.Context, t feature.T) {
		gvr := channel_impl.GVR()
		for _, verb := range []string{"get", "list", "watch"} {
			ServiceAccountSubjectAccessReviewAllowedOrFail(ctx, t, gvr, "", name, verb)
			ServiceAccountSubjectAccessReviewAllowedOrFail(ctx, t, gvr, "status", name, verb)
		}
	}
}

func channelHasAnnotations(ctx context.Context, t feature.T) {
	ch := getChannelable(ctx, t)
	if version, found := ch.Annotations["messaging.knative.dev/subscribable"]; !found {
		t.Error(`expected annotations["messaging.knative.dev/subscribable"] to exist`)
	} else if version != "v1" {
		t.Error(`expected "messaging.knative.dev/subscribable" to be "v1", found`, version)
	}
}

func channelAllowsSubscribersAndStatus(subscriptionURI string) feature.StepFn {
	return func(ctx context.Context, t feature.T) {
		var channelable *duckv1.Channelable
		subscriptionURI, err := apis.ParseURL(subscriptionURI)
		interval, timeout := environment.PollTimingsFromContext(ctx)
		var want *duckv1.SubscriberSpec
		err = wait.PollImmediate(interval, timeout, func() (bool, error) {
			channelable = getChannelable(ctx, t)
			if channelable.Status.ObservedGeneration < channelable.Generation {
				// keep polling.
				return false, nil
			}

			if len(channelable.Status.Subscribers) == len(channelable.Spec.Subscribers) {
				for _, got := range channelable.Spec.Subscribers {

					// get the UID for the subscription we made
					if got.SubscriberURI == subscriptionURI {
						want = &got
					}
				}

				for _, got := range channelable.Status.Subscribers {
					// want should be Ready.
					if got.UID == want.UID && got.Ready == corev1.ConditionTrue {
						return true, nil
					}
				}
			}
			// keep polling.
			return false, nil
		})
		if err != nil {
			t.Fatalf("failed waiting for channel subscribers to sync", err)
		}

		if len(channelable.Spec.Subscribers) <= 0 {
			t.Errorf("subscriber was not saved")
		}

		if len(channelable.Status.Subscribers) != 1 {
			t.Error("Subscribers not in status.")
		} else {
			for _, got := range channelable.Status.Subscribers {
				// want should be Ready.
				if got.UID == want.UID {
					if want := corev1.ConditionTrue; got.Ready != want {
						t.Error("Expected subscriber to be %q, got %q", want, got.Ready)
					}
				}
			}
		}
	}
}

func readyChannelIsAddressable(ctx context.Context, t feature.T) {
	var ch *duckv1.Channelable

	// Poll for a ready channel.
	interval, timeout := environment.PollTimingsFromContext(ctx)
	err := wait.PollImmediate(interval, timeout, func() (bool, error) {
		ch = getChannelable(ctx, t)
		if c := ch.Status.GetCondition(apis.ConditionReady); c.IsTrue() {
			return true, nil
		}
		return false, nil
	})
	if err != nil {
		t.Fatalf("failed to get a ready channel", err)
	}

	// Confirm the channel is ready, and addressable.
	if c := ch.Status.GetCondition(apis.ConditionReady); c.IsTrue() {
		if ch.Status.Address.URL == nil {
			t.Errorf("channel is not addressable")
		}
		// Success!
	} else {
		t.Errorf("channel was not ready, reason: %s", ch.Status.GetCondition(apis.ConditionReady).Reason)
	}
}

func readyChannelWithDLSHaveStatusUpdated(name string) feature.StepFn {
	return func(ctx context.Context, t feature.T) {
		ch := getChannelableFromName(name, ctx, t)

		// Confirm the channel is ready, and has the status.deadLetterSinkURI set.
		if c := ch.Status.GetCondition(apis.ConditionReady); c.IsTrue() {
			if !ch.Status.DeliveryStatus.IsSet() {
				bytes, _ := json.MarshalIndent(ch, "", " ")
				t.Errorf("channel DLS not resolved but resource reported ready, state:\n%s", string(bytes))
			}
			// Success!
		} else {
			t.Errorf("channel was not ready, reason: %s", ch.Status.GetCondition(apis.ConditionReady).Reason)
		}
	}
}
