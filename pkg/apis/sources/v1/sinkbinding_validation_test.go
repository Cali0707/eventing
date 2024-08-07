/*
Copyright 2020 The Knative Authors

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

package v1

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"knative.dev/pkg/apis"
	duckv1 "knative.dev/pkg/apis/duck/v1"
	"knative.dev/pkg/tracker"
)

func TestSinkBindingValidation(t *testing.T) {
	tests := []struct {
		name string
		in   *SinkBinding
		want *apis.FieldError
	}{{
		name: "missing subject namespace",
		in: &SinkBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "matt",
				Namespace: "moore",
			},
			Spec: SinkBindingSpec{
				BindingSpec: duckv1.BindingSpec{
					Subject: tracker.Reference{
						APIVersion: "apps/v1",
						Kind:       "Deployment",
						Name:       "jeanne",
					},
				},
				SourceSpec: duckv1.SourceSpec{
					Sink: duckv1.Destination{
						Ref: &duckv1.KReference{
							APIVersion: "serving.knative.dev/v1",
							Kind:       "Service",
							Name:       "gemma",
							Namespace:  "namespace",
						},
					},
				},
			},
		},
		want: apis.ErrMissingField("spec.subject.namespace"),
	}, {
		name: "invalid subject namespace",
		in: &SinkBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "matt",
				Namespace: "moore",
			},
			Spec: SinkBindingSpec{
				BindingSpec: duckv1.BindingSpec{
					Subject: tracker.Reference{
						APIVersion: "apps/v1",
						Kind:       "Deployment",
						Name:       "jeanne",
						Namespace:  "lorefice",
					},
				},
				SourceSpec: duckv1.SourceSpec{
					Sink: duckv1.Destination{
						Ref: &duckv1.KReference{
							APIVersion: "serving.knative.dev/v1",
							Kind:       "Service",
							Name:       "gemma",
							Namespace:  "namespace",
						},
					},
				},
			},
		},
		want: apis.ErrInvalidValue("lorefice", "spec.subject.namespace"),
	}, {
		name: "missing sink information",
		in: &SinkBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "matt",
				Namespace: "moore",
			},
			Spec: SinkBindingSpec{
				BindingSpec: duckv1.BindingSpec{
					Subject: tracker.Reference{
						APIVersion: "apps/v1",
						Kind:       "Deployment",
						Name:       "jeanne",
						Namespace:  "moore",
					},
				},
				SourceSpec: duckv1.SourceSpec{
					Sink: duckv1.Destination{},
				},
			},
		},
		want: apis.ErrGeneric("expected at least one, got none", "spec.sink.ref", "spec.sink.uri"),
	}, {
		name: "invalid spec ceOverrides validation",
		in: &SinkBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "gabo",
				Namespace: "test",
			},
			Spec: SinkBindingSpec{
				BindingSpec: duckv1.BindingSpec{
					Subject: tracker.Reference{
						APIVersion: "apps/v1",
						Kind:       "Deployment",
						Name:       "jeanne",
						Namespace:  "test",
					},
				},
				SourceSpec: duckv1.SourceSpec{
					CloudEventOverrides: &duckv1.CloudEventOverrides{
						Extensions: map[string]string{"Invalid_type": "any value"},
					},
					Sink: duckv1.Destination{
						Ref: &duckv1.KReference{
							APIVersion: "serving.knative.dev/v1",
							Kind:       "Service",
							Name:       "gemma",
							Namespace:  "test",
						},
					},
				},
			},
		},
		want: apis.ErrInvalidKeyName(
			"Invalid_type",
			"spec.ceOverrides.extensions",
			"keys are expected to be alphanumeric",
		),
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := test.in.Validate(context.Background())
			if (test.want != nil) != (got != nil) {
				t.Errorf("Validation() = %v, wanted %v", got, test.want)
			} else if test.want != nil && test.want.Error() != got.Error() {
				t.Errorf("Validation() = %v, wanted %v", got, test.want)
			}
		})
	}
}
