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
	"fmt"

	eventingv1 "knative.dev/eventing/pkg/apis/eventing/v1"
	eventingv1beta3 "knative.dev/eventing/pkg/apis/eventing/v1beta3"
	messagingv1 "knative.dev/eventing/pkg/apis/messaging/v1"
	duckv1 "knative.dev/pkg/apis/duck/v1"
)

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

func (g *Graph) AddEventType(et *eventingv1beta3.EventType) error {
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

		outEdge.To().AddEdge(outEdge.From(), dest, EventTypeTransform{EventType: et}, false)

		return nil
	}

	from := g.getOrCreateVertex(dest)
	to := g.getOrCreateVertex(&duckv1.Destination{Ref: et.Spec.Reference})

	from.AddEdge(to, dest, EventTypeTransform{EventType: et}, false)

	return nil
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