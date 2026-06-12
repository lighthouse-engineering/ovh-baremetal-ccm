/*
Copyright 2026 Lighthouse Engineering.

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

// ovh-baremetal-ccm is a minimal Kubernetes Cloud Controller Manager for
// OVH dedicated (bare metal) servers. It queries the OVH API to set
// ExternalIP, ProviderID, and topology labels on bare metal nodes.
//
// Only nodes with the node.ovh.com/service-name label are watched.
package main

import (
	"context"
	"io"
	"os"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	cloudprovider "k8s.io/cloud-provider"
	"k8s.io/cloud-provider/app"
	"k8s.io/cloud-provider/app/config"
	"k8s.io/cloud-provider/names"
	"k8s.io/cloud-provider/options"
	"k8s.io/component-base/cli"
	cliflag "k8s.io/component-base/cli/flag"
	_ "k8s.io/component-base/metrics/prometheus/clientgo"
	_ "k8s.io/component-base/metrics/prometheus/version"
	genericcontrollermanager "k8s.io/controller-manager/app"
	"k8s.io/controller-manager/controller"
	"k8s.io/klog/v2"

	cloudnodecontroller "k8s.io/cloud-provider/controllers/node"
	cloudnodelifecyclecontroller "k8s.io/cloud-provider/controllers/nodelifecycle"

	ovhprovider "github.com/lighthouse-engineering/ovh-baremetal-ccm/pkg/ovh"
)

func main() {
	// Register the OVH bare metal cloud provider.
	cloudprovider.RegisterCloudProvider(ovhprovider.ProviderName, func(config io.Reader) (cloudprovider.Interface, error) {
		return ovhprovider.NewCloud()
	})

	ccmOptions, err := options.NewCloudControllerManagerOptions()
	if err != nil {
		klog.Fatalf("unable to initialize command options: %v", err)
	}

	fss := cliflag.NamedFlagSets{}
	command := app.NewCloudControllerManagerCommand(
		ccmOptions,
		cloudInitializer,
		controllerInitializers(),
		names.CCMControllerAliases(),
		fss,
		wait.NeverStop,
	)

	code := cli.Run(command)
	os.Exit(code)
}

func controllerInitializers() map[string]app.ControllerInitFuncConstructor {
	// Start from scratch — only register the two controllers we need,
	// with custom constructors that use a label-filtered node informer.
	return map[string]app.ControllerInitFuncConstructor{
		names.CloudNodeController: {
			InitContext: app.ControllerInitContext{
				ClientName: "ovh-baremetal-cloud-node-controller",
			},
			Constructor: startFilteredCloudNodeController,
		},
		names.CloudNodeLifecycleController: {
			InitContext: app.ControllerInitContext{
				ClientName: "ovh-baremetal-cloud-node-lifecycle-controller",
			},
			Constructor: startFilteredCloudNodeLifecycleController,
		},
	}
}

// nodeInformerFactory creates a SharedInformerFactory that only watches nodes
// with the node.ovh.com/service-name label. This means the controller never
// sees VPS nodes (managed by the OpenStack CCM), eliminating spurious
// "instance not found" errors.
func nodeInformerFactory(completedConfig *config.CompletedConfig, clientName string) informers.SharedInformerFactory {
	client := completedConfig.ClientBuilder.ClientOrDie(clientName + "-informers")
	return informers.NewSharedInformerFactoryWithOptions(client, 0,
		informers.WithTweakListOptions(func(opts *metav1.ListOptions) {
			opts.LabelSelector = ovhprovider.ServiceNameLabel
		}),
	)
}

// startFilteredCloudNodeController constructs the cloud-node controller with a
// node informer filtered by the service-name label, and starts a periodic
// reconciliation loop for ProviderID and topology labels.
func startFilteredCloudNodeController(initContext app.ControllerInitContext, completedConfig *config.CompletedConfig, cloud cloudprovider.Interface) app.InitFunc {
	return func(ctx context.Context, controllerContext genericcontrollermanager.ControllerContext) (controller.Interface, bool, error) {
		filteredFactory := nodeInformerFactory(completedConfig, initContext.ClientName)
		kubeClient := completedConfig.ClientBuilder.ClientOrDie(initContext.ClientName)

		nodeController, err := cloudnodecontroller.NewCloudNodeController(
			filteredFactory.Core().V1().Nodes(),
			kubeClient,
			cloud,
			completedConfig.ComponentConfig.NodeStatusUpdateFrequency.Duration,
			completedConfig.ComponentConfig.NodeController.ConcurrentNodeSyncs,
			completedConfig.ComponentConfig.NodeController.ConcurrentNodeStatusUpdates,
		)
		if err != nil {
			klog.Warningf("failed to start cloud node controller: %s", err)
			return nil, false, nil
		}

		filteredFactory.Start(ctx.Done())
		filteredFactory.WaitForCacheSync(ctx.Done())

		go nodeController.Run(ctx.Done(), controllerContext.ControllerManagerMetrics)

		// Start periodic reconciliation of ProviderID and topology labels.
		// The upstream UpdateNodeStatus only reconciles addresses; this loop
		// ensures ProviderID and topology.kubernetes.io/* labels stay correct.
		if reconciler, ok := cloud.(ovhprovider.NodeReconciler); ok {
			nodeLister := filteredFactory.Core().V1().Nodes().Lister()
			go wait.UntilWithContext(ctx, func(ctx context.Context) {
				nodes, err := nodeLister.List(labels.Everything())
				if err != nil {
					klog.Errorf("Failed to list nodes for reconciliation: %v", err)
					return
				}
				for _, node := range nodes {
					if _, err := reconciler.ReconcileNode(ctx, kubeClient, node); err != nil {
						klog.Errorf("Failed to reconcile node %s: %v", node.Name, err)
					}
				}
			}, completedConfig.ComponentConfig.NodeStatusUpdateFrequency.Duration)
		}

		return nil, true, nil
	}
}

// startFilteredCloudNodeLifecycleController constructs the cloud-node-lifecycle
// controller with a node informer filtered by the service-name label.
func startFilteredCloudNodeLifecycleController(initContext app.ControllerInitContext, completedConfig *config.CompletedConfig, cloud cloudprovider.Interface) app.InitFunc {
	return func(ctx context.Context, controllerContext genericcontrollermanager.ControllerContext) (controller.Interface, bool, error) {
		filteredFactory := nodeInformerFactory(completedConfig, initContext.ClientName)

		lifecycleController, err := cloudnodelifecyclecontroller.NewCloudNodeLifecycleController(
			filteredFactory.Core().V1().Nodes(),
			completedConfig.ClientBuilder.ClientOrDie(initContext.ClientName),
			cloud,
			completedConfig.ComponentConfig.KubeCloudShared.NodeMonitorPeriod.Duration,
		)
		if err != nil {
			klog.Warningf("failed to start cloud node lifecycle controller: %s", err)
			return nil, false, nil
		}

		filteredFactory.Start(ctx.Done())
		filteredFactory.WaitForCacheSync(ctx.Done())

		go lifecycleController.Run(ctx, controllerContext.ControllerManagerMetrics)

		return nil, true, nil
	}
}

func cloudInitializer(config *config.CompletedConfig) cloudprovider.Interface {
	cloudConfig := config.ComponentConfig.KubeCloudShared.CloudProvider

	cloud, err := cloudprovider.InitCloudProvider(cloudConfig.Name, cloudConfig.CloudConfigFile)
	if err != nil {
		klog.Fatalf("Cloud provider could not be initialized: %v", err)
	}
	if cloud == nil {
		klog.Fatalf("Cloud provider is nil")
	}

	return cloud
}
