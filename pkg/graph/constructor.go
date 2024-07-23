/*
Copyright 2024 The Knative Authors

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

package graph

import (
	"context"
	"encoding/json"
	"fmt"

	apierrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	rest "k8s.io/client-go/rest"

	eventingv1 "knative.dev/eventing/pkg/apis/eventing/v1"
	eventingv1beta3 "knative.dev/eventing/pkg/apis/eventing/v1beta3"
	messagingv1 "knative.dev/eventing/pkg/apis/messaging/v1"
	eventingclient "knative.dev/eventing/pkg/client/clientset/versioned"
	duckv1 "knative.dev/pkg/apis/duck/v1"
)

type ConstructorConfig struct {
	RestConfig            rest.Config
	Namespaces            []string
	ShouldAddBroker       func(b eventingv1.Broker) bool
	ShouldAddChannel      func(c messagingv1.Channel) bool
	ShouldAddSource       func(s duckv1.Source) bool
	ShouldAddTrigger      func(t eventingv1.Trigger) bool
	ShouldAddSubscription func(s messagingv1.Subscription) bool
	ShouldAddEventType    func(et eventingv1beta3.EventType) bool
}

func ConstructGraph(ctx context.Context, config ConstructorConfig, filterFunc func(obj interface{}) bool) (*Graph, error) {
	eventingClient, err := eventingclient.NewForConfig(&config.RestConfig)
	if err != nil {
		return nil, err
	}

	g := NewGraph()

	for _, ns := range config.Namespaces {
		brokers, err := eventingClient.EventingV1().Brokers(ns).List(ctx, metav1.ListOptions{})
		if err != nil && !apierrs.IsNotFound(err) && !apierrs.IsUnauthorized(err) && !apierrs.IsForbidden(err) {
			return nil, err
		}

		if err == nil {
			for _, broker := range brokers.Items {
				if config.ShouldAddBroker(broker) {
					g.AddBroker(broker)
				}
			}
		}

		channels, err := eventingClient.MessagingV1().Channels(ns).List(ctx, metav1.ListOptions{})
		if err != nil && !apierrs.IsNotFound(err) && !apierrs.IsUnauthorized(err) && !apierrs.IsForbidden(err) {
			return nil, err
		}

		if err == nil {
			for _, channel := range channels.Items {
				if config.ShouldAddChannel(channel) {
					g.AddChannel(channel)
				}
			}
		}

		sources, err := getSources(ctx, config)
		if err != nil {
			return nil, err
		}

		for _, source := range sources {
			if config.ShouldAddSource(source) {
				g.AddSource(source)
			}
		}

		triggers, err := eventingClient.EventingV1().Triggers(ns).List(ctx, metav1.ListOptions{})
		if err != nil && !apierrs.IsNotFound(err) && !apierrs.IsUnauthorized(err) && !apierrs.IsForbidden(err) {
			return nil, err
		}

		if err == nil {
			for _, trigger := range triggers.Items {
				if config.ShouldAddTrigger(trigger) {
					err := g.AddTrigger(trigger)
					if err != nil {
						return nil, err
					}
				}
			}
		}

		subscriptions, err := eventingClient.MessagingV1().Subscriptions(ns).List(ctx, metav1.ListOptions{})
		if err != nil && !apierrs.IsNotFound(err) && !apierrs.IsUnauthorized(err) && !apierrs.IsForbidden(err) {
			return nil, err
		}

		if err == nil {
			for _, subscription := range subscriptions.Items {
				if config.ShouldAddSubscription(subscription) {
					err := g.AddSubscription(subscription)
					if err != nil {
						return nil, err
					}
				}
			}
		}

		eventTypes, err := eventingClient.EventingV1beta3().EventTypes(ns).List(ctx, metav1.ListOptions{})
		if err != nil && !apierrs.IsNotFound(err) && !apierrs.IsUnauthorized(err) && !apierrs.IsForbidden(err) {
			return nil, err
		}

		if err == nil {
			for _, eventType := range eventTypes.Items {
				if config.ShouldAddEventType(eventType) {
					err := g.AddEventType(eventType)
					if err != nil {
						return nil, err
					}
				}
			}
		}
	}

	return g, nil
}

func (g *Graph) AddBroker(broker eventingv1.Broker) {
	ref := &duckv1.KReference{
		Name:       broker.Name,
		Namespace:  broker.Namespace,
		APIVersion: "eventing.knative.dev/v1",
		Kind:       "Broker",
	}
	dest := &duckv1.Destination{Ref: ref}

	// check if this vertex already exists
	v := g.getOrCreateVertex(dest)

	if broker.Spec.Delivery == nil || broker.Spec.Delivery.DeadLetterSink == nil {
		// no DLS, we are done
		return
	}

	// broker has a DLS, we need to add an edge to that
	to := g.getOrCreateVertex(broker.Spec.Delivery.DeadLetterSink)

	v.AddEdge(to, dest, NoTransform{}, true)
}

func (g *Graph) AddChannel(channel messagingv1.Channel) {
	if channel.Kind == "" {
		channel.Kind = "Channel"
	}

	ref := &duckv1.KReference{
		Name:       channel.Name,
		Namespace:  channel.Namespace,
		APIVersion: "messaging.knative.dev/v1",
		Kind:       channel.Kind,
	}
	dest := &duckv1.Destination{Ref: ref}

	v := g.getOrCreateVertex(dest)

	if channel.Spec.Delivery == nil || channel.Spec.Delivery.DeadLetterSink == nil {
		// no DLS, we are done
		return
	}

	// channel has a DLS, we need to add an edge to that
	to := g.getOrCreateVertex(channel.Spec.Delivery.DeadLetterSink)

	v.AddEdge(to, dest, NoTransform{}, true)
}

func (g *Graph) AddEventType(et eventingv1beta3.EventType) error {
	ref := &duckv1.KReference{
		Name:       et.Name,
		Namespace:  et.Namespace,
		APIVersion: "eventing.knative.dev/v1beta3",
		Kind:       "EventType",
	}
	dest := &duckv1.Destination{Ref: ref}

	if et.Spec.Reference.Kind == "Subscription" || et.Spec.Reference.Kind == "Trigger" {
		outEdge := g.GetPrimaryOutEdgeWithRef(et.Spec.Reference)
		if outEdge == nil {
			return fmt.Errorf("trigger/subscription must have a primary outward edge, but had none")
		}

		outEdge.To().AddEdge(outEdge.From(), dest, EventTypeTransform{EventType: &et}, false)

		return nil
	}

	from := g.getOrCreateVertex(dest)
	to := g.getOrCreateVertex(&duckv1.Destination{Ref: et.Spec.Reference})

	from.AddEdge(to, dest, EventTypeTransform{EventType: &et}, false)

	return nil
}

func (g *Graph) AddSource(source duckv1.Source) {
	ref := &duckv1.KReference{
		Name:       source.Name,
		Namespace:  source.Namespace,
		APIVersion: source.APIVersion,
		Kind:       source.Kind,
	}
	dest := &duckv1.Destination{Ref: ref}

	v := g.getOrCreateVertex(dest)

	to := g.getOrCreateVertex(&source.Spec.Sink)

	v.AddEdge(to, dest, CloudEventOverridesTransform{Overrides: source.Spec.CloudEventOverrides}, true)
}

func (g *Graph) AddTrigger(trigger eventingv1.Trigger) error {
	brokerRef := &duckv1.KReference{
		Name:       trigger.Spec.Broker,
		Namespace:  trigger.Namespace,
		APIVersion: "eventing.knative.dev/v1",
		Kind:       "Broker",
	}
	brokerDest := &duckv1.Destination{Ref: brokerRef}
	broker, ok := g.vertices[makeComparableDestination(brokerDest)]
	if !ok {
		return fmt.Errorf("trigger refers to a non existent broker, can't add it to the graph")
	}

	triggerRef := &duckv1.KReference{
		Name:       trigger.Name,
		Namespace:  trigger.Namespace,
		APIVersion: "eventing.knative.dev/v1",
		Kind:       "Trigger",
	}
	triggerDest := &duckv1.Destination{Ref: triggerRef}

	to := g.getOrCreateVertex(&trigger.Spec.Subscriber)

	//TODO: the transform function should be set according to the trigger filter - there are multiple open issues to address this later
	broker.AddEdge(to, triggerDest, getTransformForTrigger(trigger), false)

	if trigger.Spec.Delivery == nil || trigger.Spec.Delivery.DeadLetterSink == nil {
		return nil
	}

	dls := g.getOrCreateVertex(trigger.Spec.Delivery.DeadLetterSink)

	broker.AddEdge(dls, triggerDest, NoTransform{}, true)

	return nil

}

func (g *Graph) AddSubscription(subscription messagingv1.Subscription) error {
	channelRef := &duckv1.KReference{
		Name:       subscription.Spec.Channel.Name,
		Namespace:  subscription.Namespace,
		APIVersion: subscription.Spec.Channel.APIVersion,
		Kind:       subscription.Spec.Channel.Kind,
	}
	channelDest := &duckv1.Destination{Ref: channelRef}
	channel, ok := g.vertices[makeComparableDestination(channelDest)]

	if !ok {
		return fmt.Errorf("subscription refers to a non existent channel, can't add it to the graph")
	}

	subscriptionRef := &duckv1.KReference{
		Name:       subscription.Name,
		Namespace:  subscription.Namespace,
		APIVersion: subscription.APIVersion,
		Kind:       "Subscription",
	}
	subscriptionDest := &duckv1.Destination{Ref: subscriptionRef}

	to := g.getOrCreateVertex(subscription.Spec.Subscriber)
	channel.AddEdge(to, subscriptionDest, NoTransform{}, false)

	// If the subscription has a reply field set, there should be another Edge struct.
	if subscription.Spec.Reply != nil {
		reply := g.getOrCreateVertex(subscription.Spec.Reply)
		to.AddEdge(reply, subscriptionDest, NoTransform{}, false)
	}

	// If the subscription has the deadLetterSink property set on the delivery field, then another Edge should be constructed.
	if subscription.Spec.Delivery == nil || subscription.Spec.Delivery.DeadLetterSink == nil {
		return nil
	}
	dls := g.getOrCreateVertex(subscription.Spec.Delivery.DeadLetterSink)
	channel.AddEdge(dls, subscriptionDest, NoTransform{}, true)

	return nil

}

func getSources(ctx context.Context, config ConstructorConfig) ([]duckv1.Source, error) {
	client, err := dynamic.NewForConfig(&config.RestConfig)
	if err != nil {
		return nil, err
	}

	sourceCRDs, err := client.Resource(
		schema.GroupVersionResource{
			Group:    "apiextentions.k8s.io",
			Version:  "v1",
			Resource: "customresourcedefinitions",
		},
	).List(ctx, metav1.ListOptions{LabelSelector: labels.Set{"duck.knative.dev/source": "true"}.String()})
	if err != nil {
		return nil, fmt.Errorf("unable to list source CRDs: %w", err)
	}

	duckSources := []duckv1.Source{}

	for i := range sourceCRDs.Items {
		sourceCrd := sourceCRDs.Items[i]
		sourceGVR, err := gvrFromUnstructured(&sourceCrd)
		if err != nil {
			continue
		}

		for _, ns := range config.Namespaces {
			sourcesList, err := client.Resource(sourceGVR).Namespace(ns).List(ctx, metav1.ListOptions{})
			if err != nil {
				continue
			}

			for i := range sourcesList.Items {
				unstructuredSource := sourcesList.Items[i]
				duckSource, err := duckSourceFromUnstructured(&unstructuredSource)
				if err == nil {
					duckSources = append(duckSources, duckSource)
				}
			}
		}
	}

	return duckSources, nil
}

func duckSourceFromUnstructured(u *unstructured.Unstructured) (duckv1.Source, error) {
	duckSource := duckv1.Source{}
	marshalled, err := u.MarshalJSON()
	if err != nil {
		return duckSource, err
	}

	err = json.Unmarshal(marshalled, &duckSource)
	return duckSource, err
}

func gvrFromUnstructured(u *unstructured.Unstructured) (schema.GroupVersionResource, error) {
	group, err := groupFromUnstructured(u)
	if err != nil {
		return schema.GroupVersionResource{}, err
	}

	version, err := versionFromUnstructured(u)
	if err != nil {
		return schema.GroupVersionResource{}, err
	}

	resource, err := resourceFromUnstructured(u)
	if err != nil {
		return schema.GroupVersionResource{}, err
	}

	return schema.GroupVersionResource{
		Group:    group,
		Version:  version,
		Resource: resource,
	}, nil
}

func groupFromUnstructured(u *unstructured.Unstructured) (string, error) {
	content := u.UnstructuredContent()
	group, found, err := unstructured.NestedString(content, "spec", "group")
	if !found || err != nil {
		return "", fmt.Errorf("can't find source kind from source CRD: %w", err)
	}

	return group, nil
}

func versionFromUnstructured(u *unstructured.Unstructured) (string, error) {
	content := u.UnstructuredContent()
	var version string
	versions, found, err := unstructured.NestedSlice(content, "spec", "versions")
	if !found || err != nil || len(versions) == 0 {
		version, found, err = unstructured.NestedString(content, "spec", "version")
		if !found || err != nil {
			return "", fmt.Errorf("can't find source version from source CRD: %w", err)
		}
	} else {
		for _, v := range versions {
			if vmap, ok := v.(map[string]interface{}); ok {
				if vmap["served"] == true {
					version = vmap["name"].(string)
					break
				}
			}
		}
	}

	if version == "" {
		return "", fmt.Errorf("can't find source version from source CRD: %w", err)
	}

	return version, nil
}

func resourceFromUnstructured(u *unstructured.Unstructured) (string, error) {
	content := u.UnstructuredContent()
	resource, found, err := unstructured.NestedString(content, "spec", "names", "plural")
	if !found || err != nil {
		return "", fmt.Errorf("can't find source resource from source CRD: %w", err)
	}

	return resource, nil
}

func getTransformForTrigger(trigger eventingv1.Trigger) Transform {
	if len(trigger.Spec.Filters) == 0 && trigger.Spec.Filter != nil {
		return &AttributesFilterTransform{Filter: trigger.Spec.Filter}
	}

	return NoTransform{}
}

func (g *Graph) getOrCreateVertex(dest *duckv1.Destination) *Vertex {
	v, ok := g.vertices[makeComparableDestination(dest)]
	if !ok {
		v = &Vertex{
			self:   dest,
			parent: g,
		}
		g.vertices[makeComparableDestination(dest)] = v
	}

	return v
}
