/*
Copyright 2022. projectsveltos.io. All rights reserved.

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

package controllers

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2/klogr"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/util"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	configv1alpha1 "github.com/projectsveltos/cluster-api-feature-manager/api/v1alpha1"
	"github.com/projectsveltos/cluster-api-feature-manager/api/v1alpha1/index"
	"github.com/projectsveltos/cluster-api-feature-manager/pkg/logs"
)

func (r *ClusterSummaryReconciler) requeueClusterSummaryForReference(
	o client.Object,
) []reconcile.Request {

	logger := klogr.New().WithValues(
		"objectMapper",
		"requeueClusterSummaryForConfigMap",
		"reference",
		o.GetName(),
	)

	logger.V(logs.LogDebug).Info("reacting to configMap/secret change")

	r.PolicyMux.Lock()
	defer r.PolicyMux.Unlock()

	// Following is needed as o.GetObjectKind().GroupVersionKind().Kind is not set
	var key configv1alpha1.PolicyRef
	switch o.(type) {
	case *corev1.ConfigMap:
		key = configv1alpha1.PolicyRef{
			Kind:      string(configv1alpha1.ConfigMapReferencedResourceKind),
			Namespace: o.GetNamespace(),
			Name:      o.GetName(),
		}
	case *corev1.Secret:
		key = configv1alpha1.PolicyRef{
			Kind:      string(configv1alpha1.SecretReferencedResourceKind),
			Namespace: o.GetNamespace(),
			Name:      o.GetName(),
		}
	default:
		key = configv1alpha1.PolicyRef{
			Kind:      o.GetObjectKind().GroupVersionKind().Kind,
			Namespace: o.GetNamespace(),
			Name:      o.GetName(),
		}
	}

	logger.V(logs.LogDebug).Info(fmt.Sprintf("referenced key: %s", key))

	requests := make([]ctrl.Request, r.getReferenceMapForEntry(&key).len())

	consumers := r.getReferenceMapForEntry(&key).items()
	for i := range consumers {
		logger.V(logs.LogDebug).Info(fmt.Sprintf("requeue consumer: %s", consumers[i]))
		requests[i] = ctrl.Request{
			NamespacedName: client.ObjectKey{
				Name:      consumers[i].Name,
				Namespace: consumers[i].Namespace,
			},
		}
	}

	return requests
}

// requeueClusterSummaryForCluster is a handler.ToRequestsFunc to be used to enqueue requests for reconciliation
// for ClusterSummary to update when its own CAPI Cluster gets updated.
func (r *ClusterSummaryReconciler) requeueClusterSummaryForCluster(
	o client.Object,
) []reconcile.Request {

	cluster, ok := o.(*clusterv1.Cluster)
	if !ok {
		panic(fmt.Sprintf("Expected a Cluster but got a %T", o))
	}

	clusterSummaryList := &configv1alpha1.ClusterSummaryList{}
	if err := r.Client.List(
		context.TODO(),
		clusterSummaryList,
		client.MatchingFields{index.ClusterNamespaceField: cluster.Namespace},
		client.MatchingFields{index.ClusterNameField: cluster.Name},
	); err != nil {
		return nil
	}

	// There can be more than one cluster using the same cluster class.
	// create a request for each of the clusters.
	requests := []ctrl.Request{}
	for i := range clusterSummaryList.Items {
		requests = append(requests, ctrl.Request{NamespacedName: util.ObjectKey(&clusterSummaryList.Items[i])})
	}
	return requests
}
