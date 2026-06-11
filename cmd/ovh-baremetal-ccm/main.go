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
// Nodes are identified by the atlas.io/ovh-service-name annotation.
package main

import (
	"io"
	"os"

	"k8s.io/apimachinery/pkg/util/wait"
	cloudprovider "k8s.io/cloud-provider"
	"k8s.io/cloud-provider/app"
	"k8s.io/cloud-provider/app/config"
	"k8s.io/cloud-provider/names"
	"k8s.io/cloud-provider/options"
	"k8s.io/component-base/cli"
	cliflag "k8s.io/component-base/cli/flag"
	_ "k8s.io/component-base/metrics/prometheus/clientgo"
	_ "k8s.io/component-base/metrics/prometheus/version"
	"k8s.io/klog/v2"

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
	controllerInitializers := app.DefaultInitFuncConstructors

	// Use unique client names to avoid conflicts with the OpenStack CCM.
	if constructor, ok := controllerInitializers[names.CloudNodeController]; ok {
		constructor.InitContext.ClientName = "ovh-baremetal-cloud-node-controller"
		controllerInitializers[names.CloudNodeController] = constructor
	}
	if constructor, ok := controllerInitializers[names.CloudNodeLifecycleController]; ok {
		constructor.InitContext.ClientName = "ovh-baremetal-cloud-node-lifecycle-controller"
		controllerInitializers[names.CloudNodeLifecycleController] = constructor
	}

	// Remove controllers we don't need (no LoadBalancer or Route support).
	delete(controllerInitializers, names.ServiceLBController)
	delete(controllerInitializers, names.NodeRouteController)

	return controllerInitializers
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
