/*
Copyright 2026 Lighthouse Engineering.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

package ovh

import (
	"fmt"
	"os"

	"github.com/ovh/go-ovh/ovh"
	cloudprovider "k8s.io/cloud-provider"
	"k8s.io/klog/v2"
)

const (
	// ProviderName is the name of this cloud provider, used in --cloud-provider flag.
	ProviderName = "ovh-baremetal"

	// ServiceNameAnnotation is the node annotation that maps a K8s node
	// to an OVH dedicated server service name.
	ServiceNameAnnotation = "node.ovh.com/service-name"
)

// cloud implements cloudprovider.Interface for OVH bare metal servers.
type cloud struct {
	ovhClient *ovh.Client
	instances *instances
}

// NewCloud creates a new OVH bare metal cloud provider.
// OVH API credentials are read from environment variables:
//   - OVH_ENDPOINT (default: ovh-eu)
//   - OVH_APPLICATION_KEY
//   - OVH_APPLICATION_SECRET
//   - OVH_CONSUMER_KEY
func NewCloud() (cloudprovider.Interface, error) {
	endpoint := os.Getenv("OVH_ENDPOINT")
	if endpoint == "" {
		endpoint = "ovh-eu"
	}

	client, err := ovh.NewClient(
		endpoint,
		os.Getenv("OVH_APPLICATION_KEY"),
		os.Getenv("OVH_APPLICATION_SECRET"),
		os.Getenv("OVH_CONSUMER_KEY"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create OVH API client: %w", err)
	}

	klog.Infof("OVH bare metal cloud provider initialized (endpoint: %s)", endpoint)

	c := &cloud{
		ovhClient: client,
	}
	c.instances = newInstances(client)

	return c, nil
}

// Initialize is called after the cloud provider is created and provides
// additional information from the cloud controller manager.
func (c *cloud) Initialize(clientBuilder cloudprovider.ControllerClientBuilder, stop <-chan struct{}) {
	klog.Info("OVH bare metal cloud provider initialized with controller client")
}

// LoadBalancer returns the load balancer interface. Not supported for bare metal.
func (c *cloud) LoadBalancer() (cloudprovider.LoadBalancer, bool) {
	return nil, false
}

// Instances returns the deprecated Instances interface. Not implemented.
func (c *cloud) Instances() (cloudprovider.Instances, bool) {
	return nil, false
}

// InstancesV2 returns the InstancesV2 interface for managing node addresses.
func (c *cloud) InstancesV2() (cloudprovider.InstancesV2, bool) {
	return c.instances, true
}

// Zones returns the deprecated Zones interface. Not implemented.
func (c *cloud) Zones() (cloudprovider.Zones, bool) {
	return nil, false
}

// Routes returns the Routes interface. Not supported for bare metal.
func (c *cloud) Routes() (cloudprovider.Routes, bool) {
	return nil, false
}

// Clusters returns the Clusters interface. Not supported.
func (c *cloud) Clusters() (cloudprovider.Clusters, bool) {
	return nil, false
}

// ProviderName returns the cloud provider name.
func (c *cloud) ProviderName() string {
	return ProviderName
}

// HasClusterID returns true. We don't require a cluster ID.
func (c *cloud) HasClusterID() bool {
	return true
}
