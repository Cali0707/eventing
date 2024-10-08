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

package v1alpha1

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	eventingv1 "knative.dev/eventing/pkg/apis/eventing/v1"
	"knative.dev/eventing/pkg/apis/feature"
	"knative.dev/pkg/apis"
	"knative.dev/pkg/ptr"
)

func TestEventPolicySpecValidationWithOIDCAuthenticationFeatureFlagDisabled(t *testing.T) {
	tests := []struct {
		name string
		ep   *EventPolicy
		want *apis.FieldError
	}{
		{
			name: "valid, from.sub exactly '*'",
			ep: &EventPolicy{
				Spec: EventPolicySpec{
					From: []EventPolicySpecFrom{{
						Sub: ptr.String("*"),
					}},
				},
			},
			want: func() *apis.FieldError {
				return apis.ErrGeneric("oidc-authentication feature not enabled")
			}(),
		},
		{
			name: "invalid, missing from.ref and from.sub",
			ep: &EventPolicy{
				Spec: EventPolicySpec{
					From: []EventPolicySpecFrom{{}},
				},
			},
			want: func() *apis.FieldError {
				return apis.ErrGeneric("oidc-authentication feature not enabled")
			}(),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := feature.ToContext(context.TODO(), feature.Flags{
				feature.OIDCAuthentication: feature.Disabled,
			})
			ctx = apis.WithinCreate(ctx)
			got := test.ep.Validate(ctx)
			if diff := cmp.Diff(test.want.Error(), got.Error()); diff != "" {
				t.Errorf("%s: Validate EventPolicySpec (-want, +got) = %v", test.name, diff)
			}
		})
	}
}

func TestEventPolicySpecValidationWithOIDCAuthenticationFeatureFlagEnabled(t *testing.T) {
	tests := []struct {
		name string
		ep   *EventPolicy
		want *apis.FieldError
	}{
		{
			name: "valid, empty",
			ep: &EventPolicy{
				Spec: EventPolicySpec{},
			},
			want: func() *apis.FieldError {
				return nil
			}(),
		},
		{
			name: "invalid, missing from.ref and from.sub",
			ep: &EventPolicy{
				Spec: EventPolicySpec{
					From: []EventPolicySpecFrom{{}},
				},
			},
			want: func() *apis.FieldError {
				return apis.ErrMissingOneOf("ref", "sub").ViaFieldIndex("from", 0).ViaField("spec")
			}(),
		},
		{
			name: "invalid, from.ref missing name",
			ep: &EventPolicy{
				Spec: EventPolicySpec{
					From: []EventPolicySpecFrom{{
						Ref: &EventPolicyFromReference{
							APIVersion: "a",
							Kind:       "b",
						},
					}},
				},
			},
			want: func() *apis.FieldError {
				return apis.ErrMissingField("name").ViaField("ref").ViaFieldIndex("from", 0).ViaField("spec")
			}(),
		},
		{
			name: "invalid, from.ref missing kind",
			ep: &EventPolicy{
				Spec: EventPolicySpec{
					From: []EventPolicySpecFrom{{
						Ref: &EventPolicyFromReference{
							APIVersion: "a",
							Name:       "b",
						},
					}},
				},
			},
			want: func() *apis.FieldError {
				return apis.ErrMissingField("kind").ViaField("ref").ViaFieldIndex("from", 0).ViaField("spec")
			}(),
		},
		{
			name: "invalid, from.ref missing apiVersion",
			ep: &EventPolicy{
				Spec: EventPolicySpec{
					From: []EventPolicySpecFrom{{
						Ref: &EventPolicyFromReference{
							Kind: "a",
							Name: "b",
						},
					}},
				},
			},
			want: func() *apis.FieldError {
				return apis.ErrMissingField("apiVersion").ViaField("ref").ViaFieldIndex("from", 0).ViaField("spec")
			}(),
		},
		{
			name: "invalid, both from.ref and from.sub set for the same list element",
			ep: &EventPolicy{
				Spec: EventPolicySpec{
					From: []EventPolicySpecFrom{{
						Ref: &EventPolicyFromReference{
							APIVersion: "a",
							Kind:       "b",
							Name:       "c",
						},
						Sub: ptr.String("abc"),
					}},
				},
			},
			want: func() *apis.FieldError {
				return apis.ErrMultipleOneOf("ref", "sub").ViaFieldIndex("from", 0).ViaField("spec")
			}(),
		},
		{
			name: "invalid, missing to.ref and to.selector",
			ep: &EventPolicy{
				Spec: EventPolicySpec{
					To: []EventPolicySpecTo{{}},
				},
			},
			want: func() *apis.FieldError {
				return apis.ErrMissingOneOf("ref", "selector").ViaFieldIndex("to", 0).ViaField("spec")
			}(),
		},
		{
			name: "invalid, both to.ref and to.selector set for the same list element",
			ep: &EventPolicy{
				Spec: EventPolicySpec{
					To: []EventPolicySpecTo{
						{
							Ref: &EventPolicyToReference{
								APIVersion: "a",
								Kind:       "b",
								Name:       "c",
							},
							Selector: &EventPolicySelector{},
						},
					},
				},
			},
			want: func() *apis.FieldError {
				return apis.ErrMultipleOneOf("ref", "selector").ViaFieldIndex("to", 0).ViaField("spec")
			}(),
		},
		{
			name: "invalid, to.ref missing name",
			ep: &EventPolicy{
				Spec: EventPolicySpec{
					To: []EventPolicySpecTo{{
						Ref: &EventPolicyToReference{
							APIVersion: "a",
							Kind:       "b",
						},
					}},
				},
			},
			want: func() *apis.FieldError {
				return apis.ErrMissingField("name").ViaField("ref").ViaFieldIndex("to", 0).ViaField("spec")
			}(),
		},
		{
			name: "invalid, to.ref missing kind",
			ep: &EventPolicy{
				Spec: EventPolicySpec{
					To: []EventPolicySpecTo{{
						Ref: &EventPolicyToReference{
							APIVersion: "a",
							Name:       "b",
						},
					}},
				},
			},
			want: func() *apis.FieldError {
				return apis.ErrMissingField("kind").ViaField("ref").ViaFieldIndex("to", 0).ViaField("spec")
			}(),
		},
		{
			name: "invalid, to.ref missing apiVersion",
			ep: &EventPolicy{
				Spec: EventPolicySpec{
					To: []EventPolicySpecTo{{
						Ref: &EventPolicyToReference{
							Kind: "a",
							Name: "b",
						},
					}},
				},
			},
			want: func() *apis.FieldError {
				return apis.ErrMissingField("apiVersion").ViaField("ref").ViaFieldIndex("to", 0).ViaField("spec")
			}(),
		},
		{
			name: "invalid, from.sub '*' set as infix",
			ep: &EventPolicy{
				Spec: EventPolicySpec{
					From: []EventPolicySpecFrom{{
						Sub: ptr.String("a*c"),
					}},
				},
			},
			want: func() *apis.FieldError {
				return apis.ErrInvalidValue("a*c", "sub", "'*' is only allowed as suffix").ViaFieldIndex("from", 0).ViaField("spec")
			}(),
		},
		{
			name: "invalid, from.sub '*' set as prefix",
			ep: &EventPolicy{
				Spec: EventPolicySpec{
					From: []EventPolicySpecFrom{{
						Sub: ptr.String("*a"),
					}},
				},
			},
			want: func() *apis.FieldError {
				return apis.ErrInvalidValue("*a", "sub", "'*' is only allowed as suffix").ViaFieldIndex("from", 0).ViaField("spec")
			}(),
		},
		{
			name: "valid, from.sub '*' set as suffix",
			ep: &EventPolicy{
				Spec: EventPolicySpec{
					From: []EventPolicySpecFrom{{
						Sub: ptr.String("a*"),
					}},
				},
			},
			want: func() *apis.FieldError {
				return nil
			}(),
		},
		{
			name: "valid, from.sub exactly '*'",
			ep: &EventPolicy{
				Spec: EventPolicySpec{
					From: []EventPolicySpecFrom{{
						Sub: ptr.String("*"),
					}},
				},
			},
			want: func() *apis.FieldError {
				return nil
			}(),
		},
		{
			name: "valid, from.sub exactly '*', valid filters",
			ep: &EventPolicy{
				Spec: EventPolicySpec{
					From: []EventPolicySpecFrom{{
						Sub: ptr.String("*"),
					}},
					Filters: []eventingv1.SubscriptionsAPIFilter{
						{
							Prefix: map[string]string{"type": "example"},
						},
					},
				},
			},
			want: func() *apis.FieldError {
				return nil
			}(),
		},
		{
			name: "invalid, from.sub exactly '*', invalid cesql filter",
			ep: &EventPolicy{
				Spec: EventPolicySpec{
					From: []EventPolicySpecFrom{{
						Sub: ptr.String("*"),
					}},
					Filters: []eventingv1.SubscriptionsAPIFilter{
						{
							CESQL: "type LIKE id",
						},
					},
				},
			},
			want: func() *apis.FieldError {

				return apis.ErrInvalidValue("type LIKE id", "cesql", "parse error: syntax error: |failed to parse LIKE expression: the pattern was not a string literal").
					ViaFieldIndex("filters", 0).
					ViaField("spec")
			}(),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := feature.ToContext(context.TODO(), feature.Flags{
				feature.OIDCAuthentication: feature.Enabled,
			})
			got := test.ep.Validate(ctx)
			if diff := cmp.Diff(test.want.Error(), got.Error()); diff != "" {
				t.Errorf("%s: Validate EventPolicySpec (-want, +got) = %v", test.name, diff)
			}
		})
	}
}
