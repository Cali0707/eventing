/*
Copyright 2020 The Knative Authors.

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

package testing

import (
	"context"
	"encoding/json"
	"testing"

	"knative.dev/pkg/configmap"
	"knative.dev/pkg/logging"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	"k8s.io/client-go/tools/record"

	"go.uber.org/zap"
	ktesting "k8s.io/client-go/testing"
	"knative.dev/pkg/controller"

	fakeeventingclient "knative.dev/eventing/pkg/client/injection/client/fake"
	fakeapiextensionsclient "knative.dev/pkg/client/injection/apiextensions/client/fake"
	fakekubeclient "knative.dev/pkg/client/injection/kube/client/fake"
	fakedynamicclient "knative.dev/pkg/injection/clients/dynamicclient/fake"

	"knative.dev/pkg/reconciler"
	//nolint:staticcheck  // Not sure why this is dot imported...
	. "knative.dev/pkg/reconciler/testing"
)

const (
	// maxEventBufferSize is the estimated max number of event notifications that
	// can be buffered during reconciliation.
	maxEventBufferSize = 10
)

// Ctor functions create a k8s controller with given params.
type Ctor func(context.Context, *Listers, configmap.Watcher) controller.Reconciler

// MakeFactory creates a reconciler factory with fake clients and controller created by `ctor`.
func MakeFactory(ctor Ctor, unstructured bool, logger *zap.SugaredLogger) Factory {
	return func(t *testing.T, r *TableRow) (controller.Reconciler, ActionRecorderList, EventList) {
		ls := NewListers(r.Objects)

		ctx := context.Background()
		ctx = logging.WithLogger(ctx, logger)

		ctx, kubeClient := fakekubeclient.With(ctx, ls.GetKubeObjects()...)
		ctx, client := fakeeventingclient.With(ctx, ls.GetEventingObjects()...)
		ctx, extClient := fakeapiextensionsclient.With(ctx, ls.GetAPIExtensionObjects()...)
		ctx, dynamicClient := fakedynamicclient.With(ctx,
			NewScheme(), ToUnstructured(t, r.Objects)...)

		// The dynamic client's support for patching is BS.  Implement it
		// here via PrependReactor (this can be overridden below by the
		// provided reactors).
		dynamicClient.PrependReactor("patch", "*",
			func(action ktesting.Action) (bool, runtime.Object, error) {
				return true, nil, nil
			})

		eventRecorder := record.NewFakeRecorder(maxEventBufferSize)
		ctx = controller.WithEventRecorder(ctx, eventRecorder)

		// Set up our Controller from the fakes.
		c := ctor(ctx, &ls, configmap.NewStaticWatcher())

		// If the reconcilers is leader aware, then promote it.
		if la, ok := c.(reconciler.LeaderAware); ok {
			la.Promote(reconciler.UniversalBucket(), func(reconciler.Bucket, types.NamespacedName) {})
		}

		for _, reactor := range r.WithReactors {
			kubeClient.PrependReactor("*", "*", reactor)
			client.PrependReactor("*", "*", reactor)
			dynamicClient.PrependReactor("*", "*", reactor)
			extClient.PrependReactor("*", "*", reactor)
		}

		// Validate all Create operations through the eventing client.
		client.PrependReactor("create", "*", func(action ktesting.Action) (handled bool, ret runtime.Object, err error) {
			return ValidateCreates(ctx, action)
		})
		client.PrependReactor("update", "*", func(action ktesting.Action) (handled bool, ret runtime.Object, err error) {
			return ValidateUpdates(ctx, action)
		})

		actionRecorderList := ActionRecorderList{dynamicClient, client, kubeClient}
		eventList := EventList{Recorder: eventRecorder}

		return c, actionRecorderList, eventList
	}
}

// ToUnstructured takes a list of k8s resources and converts them to
// Unstructured objects.
// We must pass objects as Unstructured to the dynamic client fake, or it
// won't handle them properly.
func ToUnstructured(t *testing.T, objs []runtime.Object) (us []runtime.Object) {
	sch := NewScheme()
	for _, obj := range objs {
		obj = obj.DeepCopyObject() // Don't mess with the primary copy

		ta, err := meta.TypeAccessor(obj)
		if err != nil {
			t.Fatal("Unable to create type accessor:", err)
		}

		if ta.GetAPIVersion() == "" || ta.GetKind() == "" {
			// Determine and set the TypeMeta for this object based on our test scheme.
			gvks, _, err := sch.ObjectKinds(obj)
			if err != nil {
				t.Fatal("Unable to determine kind for type:", err)
			}
			apiv, k := gvks[0].ToAPIVersionAndKind()
			ta.SetAPIVersion(apiv)
			ta.SetKind(k)
		}

		b, err := json.Marshal(obj)
		if err != nil {
			t.Fatal("Unable to marshal:", err)
		}
		u := &unstructured.Unstructured{}
		if err := json.Unmarshal(b, u); err != nil {
			t.Fatal("Unable to unmarshal:", err)
		}
		us = append(us, u)
	}
	return
}
