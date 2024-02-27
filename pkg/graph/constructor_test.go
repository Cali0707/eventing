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
	"github.com/stretchr/testify/assert"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	eventingduckv1 "knative.dev/eventing/pkg/apis/duck/v1"
	eventingv1 "knative.dev/eventing/pkg/apis/eventing/v1"
	"knative.dev/pkg/apis"
	duckv1 "knative.dev/pkg/apis/duck/v1"
)

var (
	sampleUri, _ = apis.ParseURL("https://knative.dev")
)

func TestAddBroker(t *testing.T) {
	brokerWithEdge := &Vertex{
		self: &duckv1.Destination{
			Ref: &duckv1.KReference{
				Name:       "my-broker",
				Namespace:  "default",
				APIVersion: "eventing.knative.dev/v1",
				Kind:       "Broker",
			},
		},
	}
	destinationWithEdge := &Vertex{
		self: &duckv1.Destination{
			URI: sampleUri,
		},
	}
	brokerWithEdge.AddEdge(destinationWithEdge, &duckv1.Destination{
		Ref: &duckv1.KReference{
			Name:       "my-broker",
			Namespace:  "default",
			APIVersion: "eventing.knative.dev/v1",
			Kind:       "Broker",
		},
	}, NoTransform)
	tests := []struct {
		name     string
		broker   eventingv1.Broker
		expected map[comparableDestination]*Vertex
	}{
		{
			name: "no DLS",
			broker: eventingv1.Broker{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-broker",
					Namespace: "default",
				},
			},
			expected: map[comparableDestination]*Vertex{
				{
					Ref: duckv1.KReference{
						Name:       "my-broker",
						Namespace:  "default",
						APIVersion: "eventing.knative.dev/v1",
						Kind:       "Broker",
					},
				}: {
					self: &duckv1.Destination{
						Ref: &duckv1.KReference{
							Name:       "my-broker",
							Namespace:  "default",
							APIVersion: "eventing.knative.dev/v1",
							Kind:       "Broker",
						},
					},
				},
			},
		}, {
			name: "DLS",
			broker: eventingv1.Broker{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-broker",
					Namespace: "default",
				},
				Spec: eventingv1.BrokerSpec{
					Delivery: &eventingduckv1.DeliverySpec{
						DeadLetterSink: &duckv1.Destination{
							URI: sampleUri,
						},
					},
				},
			},
			expected: map[comparableDestination]*Vertex{
				{
					Ref: duckv1.KReference{
						Name:       "my-broker",
						Namespace:  "default",
						APIVersion: "eventing.knative.dev/v1",
						Kind:       "Broker",
					},
				}: brokerWithEdge,
				{
					URI: *sampleUri,
				}: destinationWithEdge,
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			g := NewGraph()
			g.AddBroker(test.broker)
			assert.Len(t, g.vertices, len(test.expected))
			for k, expected := range test.expected {
				actual, ok := g.vertices[k]
				assert.True(t, ok)
				// assert.Equal can't do equality on function values, which edges have, so we need to do a more complicated check
				assert.Equal(t, actual.self, expected.self)
				assert.Len(t, actual.inEdges, len(expected.inEdges))
				assert.Subset(t, makeComparableEdges(actual.inEdges), makeComparableEdges(expected.inEdges))
				assert.Len(t, actual.outEdges, len(expected.outEdges))
				assert.Subset(t, makeComparableEdges(actual.outEdges), makeComparableEdges(expected.outEdges))
			}
		})
	}
}

func makeComparableEdges(edges []*Edge) []comparableEdge {
	res := make([]comparableEdge, len(edges))

	for _, e := range edges {
		res = append(res, comparableEdge{
			self: makeComparableDestination(e.self),
			to:   makeComparableDestination(e.to.self),
			from: makeComparableDestination(e.from.self),
		})
	}

	return res
}

type comparableEdge struct {
	self comparableDestination
	to   comparableDestination
	from comparableDestination
}
