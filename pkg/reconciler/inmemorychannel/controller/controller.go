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

package controller

import (
	"context"

	"knative.dev/eventing/pkg/auth"

	"github.com/kelseyhightower/envconfig"
	"k8s.io/client-go/tools/cache"
	messagingv1 "knative.dev/eventing/pkg/apis/messaging/v1"
	kubeclient "knative.dev/pkg/client/injection/kube/client"
	"knative.dev/pkg/configmap"
	"knative.dev/pkg/controller"
	"knative.dev/pkg/logging"
	"knative.dev/pkg/system"

	"knative.dev/pkg/resolver"

	"knative.dev/eventing/pkg/apis/feature"
	"knative.dev/eventing/pkg/client/injection/informers/eventing/v1alpha1/eventpolicy"
	"knative.dev/eventing/pkg/client/injection/informers/messaging/v1/inmemorychannel"
	inmemorychannelreconciler "knative.dev/eventing/pkg/client/injection/reconciler/messaging/v1/inmemorychannel"
	"knative.dev/eventing/pkg/eventingtls"
	"knative.dev/eventing/pkg/reconciler/inmemorychannel/controller/config"

	"knative.dev/pkg/client/injection/kube/informers/apps/v1/deployment"
	"knative.dev/pkg/client/injection/kube/informers/core/v1/endpoints"
	"knative.dev/pkg/client/injection/kube/informers/core/v1/service"
	"knative.dev/pkg/client/injection/kube/informers/core/v1/serviceaccount"
	"knative.dev/pkg/client/injection/kube/informers/rbac/v1/rolebinding"
	secretinformer "knative.dev/pkg/injection/clients/namespacedkube/informers/core/v1/secret"
)

// TODO: this should be passed in on the env.
const dispatcherName = "imc-dispatcher"

type envConfig struct {
	Image string `envconfig:"DISPATCHER_IMAGE" required:"true"`
}

// NewController initializes the controller and is called by the generated code.
// Registers event handlers to enqueue events.
func NewController(
	ctx context.Context,
	cmw configmap.Watcher,
) *controller.Impl {
	logger := logging.FromContext(ctx)
	inmemorychannelInformer := inmemorychannel.Get(ctx)
	deploymentInformer := deployment.Get(ctx)
	serviceInformer := service.Get(ctx)
	endpointsInformer := endpoints.Get(ctx)
	serviceAccountInformer := serviceaccount.Get(ctx)
	roleBindingInformer := rolebinding.Get(ctx)
	secretInformer := secretinformer.Get(ctx)
	eventPolicyInformer := eventpolicy.Get(ctx)

	r := &Reconciler{
		kubeClientSet:        kubeclient.Get(ctx),
		systemNamespace:      system.Namespace(),
		deploymentLister:     deploymentInformer.Lister(),
		serviceLister:        serviceInformer.Lister(),
		endpointsLister:      endpointsInformer.Lister(),
		serviceAccountLister: serviceAccountInformer.Lister(),
		roleBindingLister:    roleBindingInformer.Lister(),
		secretLister:         secretInformer.Lister(),
		eventPolicyLister:    eventPolicyInformer.Lister(),
	}

	env := &envConfig{}
	if err := envconfig.Process("", env); err != nil {
		logger.Panicf("unable to process in-memory channel's required environment variables: %v", err)
	}

	if env.Image == "" {
		logger.Panic("unable to process in-memory channel's required environment variables (missing DISPATCHER_IMAGE)")
	}

	var globalResync func(obj interface{})

	featureStore := feature.NewStore(logging.FromContext(ctx).Named("feature-config-store"), func(name string, value interface{}) {
		if globalResync != nil {
			globalResync(nil)
		}
	})
	featureStore.WatchConfigs(cmw)

	r.dispatcherImage = env.Image

	impl := inmemorychannelreconciler.NewImpl(ctx, r, func(impl *controller.Impl) controller.Options {
		return controller.Options{
			ConfigStore: featureStore,
		}
	})
	r.uriResolver = resolver.NewURIResolverFromTracker(ctx, impl.Tracker)

	inmemorychannelInformer.Informer().AddEventHandler(controller.HandleAll(impl.Enqueue))

	// Set up watches for dispatcher resources we care about, since any changes to these
	// resources will affect our Channels. So, set up a watch here, that will cause
	// a global Resync for all the channels to take stock of their health when these change.

	// Call GlobalResync on inmemorychannels.
	globalResync = func(interface{}) {
		impl.GlobalResync(inmemorychannelInformer.Informer())
	}

	deploymentInformer.Informer().AddEventHandler(cache.FilteringResourceEventHandler{
		FilterFunc: controller.FilterWithName(dispatcherName),
		Handler:    controller.HandleAll(globalResync),
	})
	serviceInformer.Informer().AddEventHandler(cache.FilteringResourceEventHandler{
		FilterFunc: controller.FilterWithName(dispatcherName),
		Handler:    controller.HandleAll(globalResync),
	})
	endpointsInformer.Informer().AddEventHandler(cache.FilteringResourceEventHandler{
		FilterFunc: controller.FilterWithName(dispatcherName),
		Handler:    controller.HandleAll(globalResync),
	})
	serviceAccountInformer.Informer().AddEventHandler(cache.FilteringResourceEventHandler{
		FilterFunc: controller.FilterWithName(dispatcherName),
		Handler:    controller.HandleAll(globalResync),
	})
	roleBindingInformer.Informer().AddEventHandler(cache.FilteringResourceEventHandler{
		FilterFunc: controller.FilterWithName(dispatcherName),
		Handler:    controller.HandleAll(globalResync),
	})
	secretInformer.Informer().AddEventHandler(cache.FilteringResourceEventHandler{
		FilterFunc: controller.FilterWithName(eventingtls.IMCDispatcherServerTLSSecretName),
		Handler:    controller.HandleAll(globalResync),
	})

	imcGK := messagingv1.SchemeGroupVersion.WithKind("InMemoryChannel").GroupKind()

	// Enqueue the InMemoryChannel, if we have an EventPolicy which was referencing
	// or got updated and now is referencing the InMemoryChannel
	eventPolicyInformer.Informer().AddEventHandler(auth.EventPolicyEventHandler(inmemorychannelInformer.Informer().GetIndexer(), imcGK, impl.EnqueueKey))

	// Setup the watch on the config map of dispatcher config
	configStore := config.NewEventDispatcherConfigStore(logging.FromContext(ctx))
	configStore.WatchConfigs(cmw)
	r.eventDispatcherConfigStore = configStore

	return impl
}
