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

// Code generated by informer-gen. DO NOT EDIT.

package v1alpha1

import (
	internalinterfaces "knative.dev/eventing/pkg/client/informers/externalversions/internalinterfaces"
)

// Interface provides access to all the informers in this group version.
type Interface interface {
	// EventPolicies returns a EventPolicyInformer.
	EventPolicies() EventPolicyInformer
	// EventTransforms returns a EventTransformInformer.
	EventTransforms() EventTransformInformer
	// RequestReplies returns a RequestReplyInformer.
	RequestReplies() RequestReplyInformer
}

type version struct {
	factory          internalinterfaces.SharedInformerFactory
	namespace        string
	tweakListOptions internalinterfaces.TweakListOptionsFunc
}

// New returns a new Interface.
func New(f internalinterfaces.SharedInformerFactory, namespace string, tweakListOptions internalinterfaces.TweakListOptionsFunc) Interface {
	return &version{factory: f, namespace: namespace, tweakListOptions: tweakListOptions}
}

// EventPolicies returns a EventPolicyInformer.
func (v *version) EventPolicies() EventPolicyInformer {
	return &eventPolicyInformer{factory: v.factory, namespace: v.namespace, tweakListOptions: v.tweakListOptions}
}

// EventTransforms returns a EventTransformInformer.
func (v *version) EventTransforms() EventTransformInformer {
	return &eventTransformInformer{factory: v.factory, namespace: v.namespace, tweakListOptions: v.tweakListOptions}
}

// RequestReplies returns a RequestReplyInformer.
func (v *version) RequestReplies() RequestReplyInformer {
	return &requestReplyInformer{factory: v.factory, namespace: v.namespace, tweakListOptions: v.tweakListOptions}
}
