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
	"encoding/json"
	"fmt"
	"strings"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
)

// ReconcileNode checks and corrects ProviderID and topology labels for a bare
// metal node. It compares the current node state against the OVH API and
// patches any drift. This is called periodically (alongside the upstream
// UpdateNodeStatus which only reconciles addresses) so that topology labels
// and ProviderID are self-healing.
//
// Returns true if any changes were made.
func (c *cloud) ReconcileNode(ctx context.Context, kubeClient clientset.Interface, node *v1.Node) (bool, error) {
	serviceName, err := getServiceName(node)
	if err != nil {
		return false, nil // not our node
	}

	server, err := c.instances.getServer(serviceName)
	if err != nil {
		return false, fmt.Errorf("failed to get OVH server %s: %w", serviceName, err)
	}

	expectedProviderID := providerIDPrefix + serviceName
	expectedZone := server.AvailabilityZone
	expectedRegion := strings.ToUpper(server.Region)

	// Check what needs updating.
	needsSpecPatch := node.Spec.ProviderID != expectedProviderID
	needsLabelPatch := node.Labels[v1.LabelTopologyZone] != expectedZone ||
		node.Labels[v1.LabelTopologyRegion] != expectedRegion

	if !needsSpecPatch && !needsLabelPatch {
		return false, nil
	}

	// Build a strategic merge patch.
	patch := map[string]interface{}{}

	if needsSpecPatch {
		klog.V(2).Infof("Node %s: ProviderID drift detected (have=%q, want=%q)",
			node.Name, node.Spec.ProviderID, expectedProviderID)
		patch["spec"] = map[string]interface{}{
			"providerID": expectedProviderID,
		}
	}

	if needsLabelPatch {
		labels := map[string]string{}
		if node.Labels[v1.LabelTopologyZone] != expectedZone {
			klog.V(2).Infof("Node %s: zone label drift (have=%q, want=%q)",
				node.Name, node.Labels[v1.LabelTopologyZone], expectedZone)
			labels[v1.LabelTopologyZone] = expectedZone
		}
		if node.Labels[v1.LabelTopologyRegion] != expectedRegion {
			klog.V(2).Infof("Node %s: region label drift (have=%q, want=%q)",
				node.Name, node.Labels[v1.LabelTopologyRegion], expectedRegion)
			labels[v1.LabelTopologyRegion] = expectedRegion
		}
		patch["metadata"] = map[string]interface{}{
			"labels": labels,
		}
	}

	patchBytes, err := json.Marshal(patch)
	if err != nil {
		return false, fmt.Errorf("failed to marshal patch: %w", err)
	}

	_, err = kubeClient.CoreV1().Nodes().Patch(ctx, node.Name, types.StrategicMergePatchType, patchBytes, metav1.PatchOptions{})
	if err != nil {
		return false, fmt.Errorf("failed to patch node %s: %w", node.Name, err)
	}

	klog.Infof("Reconciled node %s: patched providerID/topology from OVH API", node.Name)
	return true, nil
}
