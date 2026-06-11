/*
Copyright 2026 Lighthouse Engineering.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

package ovh

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/ovh/go-ovh/ovh"
	v1 "k8s.io/api/core/v1"
	cloudprovider "k8s.io/cloud-provider"
	"k8s.io/klog/v2"
)

const (
	// providerIDPrefix is the prefix for the ProviderID on bare metal nodes.
	providerIDPrefix = "ovh-baremetal://"

	// cacheTTL is how long to cache OVH API responses for a given server.
	cacheTTL = 5 * time.Minute
)

// dedicatedServer represents the OVH API response for a dedicated server.
type dedicatedServer struct {
	IP               string `json:"ip"`
	Region           string `json:"region"`
	AvailabilityZone string `json:"availabilityZone"`
	Datacenter       string `json:"datacenter"`
	State            string `json:"state"`
	PowerState       string `json:"powerState"`
}

// cachedServer wraps a server response with a timestamp for cache expiry.
type cachedServer struct {
	server    *dedicatedServer
	fetchedAt time.Time
}

// instances implements cloudprovider.InstancesV2 for OVH bare metal servers.
type instances struct {
	ovhClient *ovh.Client

	mu    sync.RWMutex
	cache map[string]*cachedServer
}

func newInstances(client *ovh.Client) *instances {
	return &instances{
		ovhClient: client,
		cache:     make(map[string]*cachedServer),
	}
}

// getServiceName extracts the OVH service name from a node's annotation or ProviderID.
func getServiceName(node *v1.Node) (string, error) {
	// Try annotation first.
	if sn, ok := node.Annotations[ServiceNameAnnotation]; ok && sn != "" {
		return sn, nil
	}

	// Try ProviderID (format: ovh-baremetal://<service-name>).
	if node.Spec.ProviderID != "" && strings.HasPrefix(node.Spec.ProviderID, providerIDPrefix) {
		return strings.TrimPrefix(node.Spec.ProviderID, providerIDPrefix), nil
	}

	return "", fmt.Errorf("node %s has no %s annotation or %s ProviderID",
		node.Name, ServiceNameAnnotation, providerIDPrefix)
}

// getServer fetches dedicated server details from the OVH API with caching.
func (i *instances) getServer(serviceName string) (*dedicatedServer, error) {
	i.mu.RLock()
	if cached, ok := i.cache[serviceName]; ok && time.Since(cached.fetchedAt) < cacheTTL {
		i.mu.RUnlock()
		return cached.server, nil
	}
	i.mu.RUnlock()

	klog.V(4).Infof("Fetching OVH server details for %s", serviceName)

	var server dedicatedServer
	err := i.ovhClient.Get(
		fmt.Sprintf("/dedicated/server/%s", url.PathEscape(serviceName)),
		&server,
	)
	if err != nil {
		return nil, fmt.Errorf("OVH API error for %s: %w", serviceName, err)
	}

	i.mu.Lock()
	i.cache[serviceName] = &cachedServer{
		server:    &server,
		fetchedAt: time.Now(),
	}
	i.mu.Unlock()

	return &server, nil
}

// InstanceExists checks if the node's OVH server exists and is accessible.
func (i *instances) InstanceExists(ctx context.Context, node *v1.Node) (bool, error) {
	serviceName, err := getServiceName(node)
	if err != nil {
		// Node doesn't have our annotation — it's not one of our nodes.
		// Return true to avoid the lifecycle controller deleting it.
		klog.V(4).Infof("Node %s is not an OVH bare metal node, skipping: %v", node.Name, err)
		return true, nil
	}

	_, err = i.getServer(serviceName)
	if err != nil {
		klog.Warningf("Failed to check existence of OVH server %s for node %s: %v",
			serviceName, node.Name, err)
		// Return an error so the lifecycle controller retries instead of deleting.
		return false, err
	}

	return true, nil
}

// InstanceShutdown checks if the node's OVH server is powered off.
func (i *instances) InstanceShutdown(ctx context.Context, node *v1.Node) (bool, error) {
	serviceName, err := getServiceName(node)
	if err != nil {
		return false, nil
	}

	server, err := i.getServer(serviceName)
	if err != nil {
		return false, err
	}

	// OVH powerState: "poweron", "poweroff"
	return server.PowerState == "poweroff", nil
}

// InstanceMetadata returns the node's metadata from the OVH API:
// - ExternalIP from the server's public IP
// - ProviderID as ovh-baremetal://<service-name>
// - Zone from the server's availability zone
// - Region from the server's region (uppercased to match OpenStack CCM format)
func (i *instances) InstanceMetadata(ctx context.Context, node *v1.Node) (*cloudprovider.InstanceMetadata, error) {
	serviceName, err := getServiceName(node)
	if err != nil {
		return nil, cloudprovider.InstanceNotFound
	}

	server, err := i.getServer(serviceName)
	if err != nil {
		return nil, fmt.Errorf("failed to get OVH server %s: %w", serviceName, err)
	}

	// Build node addresses:
	// - Preserve existing InternalIP and Hostname from kubelet
	// - Add ExternalIP from OVH API
	addresses := []v1.NodeAddress{}
	for _, addr := range node.Status.Addresses {
		if addr.Type == v1.NodeInternalIP || addr.Type == v1.NodeHostName {
			addresses = append(addresses, addr)
		}
	}

	if server.IP != "" {
		addresses = append(addresses, v1.NodeAddress{
			Type:    v1.NodeExternalIP,
			Address: server.IP,
		})
	}

	klog.V(2).Infof("Node %s (OVH %s): ExternalIP=%s, Zone=%s, Region=%s",
		node.Name, serviceName, server.IP, server.AvailabilityZone, server.Region)

	return &cloudprovider.InstanceMetadata{
		ProviderID:    providerIDPrefix + serviceName,
		NodeAddresses: addresses,
		Zone:          server.AvailabilityZone,
		Region:        strings.ToUpper(server.Region),
	}, nil
}
