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

package fv_test

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	configv1alpha1 "github.com/projectsveltos/cluster-api-feature-manager/api/v1alpha1"
)

const (
	kongServiceAccount = `apiVersion: v1
kind: ServiceAccount
metadata:
  name: kong-serviceaccount
  namespace: kong
  `

	kongRole = `apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: kong-leader-election
  namespace: kong2
rules:
- apiGroups:
  - ""
  - coordination.k8s.io
  resources:
  - configmaps
  - leases
  verbs:
  - get
  - list
  - watch
  - create
  - update
  - patch
  - delete
- apiGroups:
  - ""
  resources:
  - events
  verbs:
  - create
  - patch
  `
)

var _ = Describe("DryRun", func() {
	const (
		namePrefix = "drynrun"
	)

	It("Correctly reports helm chart that would be installed, uninstalled or have conflicts", Label("FV"), func() {
		configMapNs := randomString()
		Byf("Create configMap's namespace %s", configMapNs)
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: configMapNs,
			},
		}
		Expect(k8sClient.Create(context.TODO(), ns)).To(Succeed())

		Byf("Create a configMap with kong ServiceAccount")
		kongSAConfigMap := createConfigMapWithPolicy(configMapNs, namePrefix+randomString(), kongServiceAccount)
		Expect(k8sClient.Create(context.TODO(), kongSAConfigMap)).To(Succeed())

		Byf("Create a ClusterFeature in Continuous syncMode matching Cluster %s/%s", kindWorkloadCluster.Namespace, kindWorkloadCluster.Name)
		clusterFeature := getClusterfeature(namePrefix, map[string]string{key: value})
		clusterFeature.Spec.SyncMode = configv1alpha1.SyncModeContinuous
		Expect(k8sClient.Create(context.TODO(), clusterFeature)).To(Succeed())

		verifyClusterFeatureMatches(clusterFeature)

		verifyClusterSummary(clusterFeature, kindWorkloadCluster.Namespace, kindWorkloadCluster.Name)

		Byf("Update ClusterFeature %s to reference ConfigMap with Kong ServiceAccount %s/%s",
			clusterFeature.Name, kongSAConfigMap.Namespace, kongSAConfigMap.Name)
		currentClusterFeature := &configv1alpha1.ClusterFeature{}
		Expect(k8sClient.Get(context.TODO(), types.NamespacedName{Name: clusterFeature.Name}, currentClusterFeature)).To(Succeed())
		currentClusterFeature.Spec.PolicyRefs = []configv1alpha1.PolicyRef{
			{
				Kind:      string(configv1alpha1.ConfigMapReferencedResourceKind),
				Namespace: kongSAConfigMap.Namespace,
				Name:      kongSAConfigMap.Name,
			},
		}
		Expect(k8sClient.Update(context.TODO(), currentClusterFeature)).To(Succeed())

		Byf("Update ClusterFeature %s to deploy mysql helm chart", clusterFeature.Name)
		Expect(k8sClient.Get(context.TODO(), types.NamespacedName{Name: clusterFeature.Name}, currentClusterFeature)).To(Succeed())
		currentClusterFeature.Spec.HelmCharts = []configv1alpha1.HelmChart{
			{
				RepositoryURL:    "https://charts.bitnami.com/bitnami",
				RepositoryName:   "bitnami",
				ChartName:        "bitnami/mysql",
				ChartVersion:     "9.3.3",
				ReleaseName:      "mysql",
				ReleaseNamespace: "mysql",
				HelmChartAction:  configv1alpha1.HelmChartActionInstall,
			},
		}
		Expect(k8sClient.Update(context.TODO(), currentClusterFeature)).To(Succeed())

		clusterSummary := verifyClusterSummary(currentClusterFeature, kindWorkloadCluster.Namespace, kindWorkloadCluster.Name)

		Byf("Verifying ClusterSummary %s status is set to Deployed for Helm feature", clusterSummary.Name)
		verifyFeatureStatusIsProvisioned(kindWorkloadCluster.Namespace, clusterSummary.Name, configv1alpha1.FeatureHelm)

		Byf("Verifying ClusterSummary %s status is set to Deployed for Resource feature", clusterSummary.Name)
		verifyFeatureStatusIsProvisioned(kindWorkloadCluster.Namespace, clusterSummary.Name, configv1alpha1.FeatureResources)

		charts := []configv1alpha1.Chart{
			{ChartName: "mysql", ChartVersion: "9.3.3", Namespace: "mysql"},
		}

		verifyClusterConfiguration(clusterFeature.Name, clusterSummary.Spec.ClusterNamespace,
			clusterSummary.Spec.ClusterName, configv1alpha1.FeatureHelm, nil, charts)

		policies := []policy{
			{kind: "ServiceAccount", name: "kong-serviceaccount", namespace: "kong", group: ""},
		}
		verifyClusterConfiguration(clusterFeature.Name, clusterSummary.Spec.ClusterNamespace,
			clusterSummary.Spec.ClusterName, configv1alpha1.FeatureResources, policies, nil)

		Byf("Create a configMap with kong Role")
		kongRoleConfigMap := createConfigMapWithPolicy(configMapNs, namePrefix+randomString(), kongRole)
		Expect(k8sClient.Create(context.TODO(), kongRoleConfigMap)).To(Succeed())

		Byf("Create a ClusterFeature in DryRun syncMode matching Cluster %s/%s", kindWorkloadCluster.Namespace, kindWorkloadCluster.Name)
		dryRunClusterFeature := getClusterfeature(namePrefix, map[string]string{key: value})
		dryRunClusterFeature.Spec.SyncMode = configv1alpha1.SyncModeDryRun
		Expect(k8sClient.Create(context.TODO(), dryRunClusterFeature)).To(Succeed())

		verifyClusterFeatureMatches(dryRunClusterFeature)

		verifyClusterSummary(dryRunClusterFeature, kindWorkloadCluster.Namespace, kindWorkloadCluster.Name)

		Byf("Update ClusterFeature %s to reference configMaps with Kong's configuration", dryRunClusterFeature.Name)
		Expect(k8sClient.Get(context.TODO(), types.NamespacedName{Name: dryRunClusterFeature.Name}, currentClusterFeature)).To(Succeed())
		currentClusterFeature.Spec.PolicyRefs = []configv1alpha1.PolicyRef{
			{Kind: string(configv1alpha1.ConfigMapReferencedResourceKind), Namespace: configMapNs, Name: kongRoleConfigMap.Name},
			{Kind: string(configv1alpha1.ConfigMapReferencedResourceKind), Namespace: configMapNs, Name: kongSAConfigMap.Name},
		}
		Expect(k8sClient.Update(context.TODO(), currentClusterFeature)).To(Succeed())

		Byf("Update ClusterFeature %s to reference some helm charts", dryRunClusterFeature.Name)
		Expect(k8sClient.Get(context.TODO(), types.NamespacedName{Name: dryRunClusterFeature.Name}, currentClusterFeature)).To(Succeed())
		currentClusterFeature.Spec.HelmCharts = []configv1alpha1.HelmChart{
			{
				RepositoryURL:    "https://charts.bitnami.com/bitnami",
				RepositoryName:   "bitnami",
				ChartName:        "bitnami/mysql",
				ChartVersion:     "9.3.3",
				ReleaseName:      "mysql",
				ReleaseNamespace: "mysql",
				HelmChartAction:  configv1alpha1.HelmChartActionInstall,
			},
			{
				RepositoryURL:    "https://charts.bitnami.com/bitnami",
				RepositoryName:   "bitnami",
				ChartName:        "bitnami/redis",
				ChartVersion:     "17.1.6",
				ReleaseName:      "redis",
				ReleaseNamespace: "redis",
				HelmChartAction:  configv1alpha1.HelmChartActionInstall,
			},
			{
				RepositoryURL:    "https://charts.bitnami.com/bitnami",
				RepositoryName:   "bitnami",
				ChartName:        "bitnami/postgresql",
				ChartVersion:     "11.9.1",
				ReleaseName:      "postgresql",
				ReleaseNamespace: "postgresql",
				HelmChartAction:  configv1alpha1.HelmChartActionUninstall,
			},
		}
		Expect(k8sClient.Update(context.TODO(), currentClusterFeature)).To(Succeed())

		dryRunClusterSummary := verifyClusterSummary(currentClusterFeature, kindWorkloadCluster.Namespace, kindWorkloadCluster.Name)

		By("Verifying ClusterReport for helm reports")
		clusterReportName := fmt.Sprintf("%s--%s", dryRunClusterFeature.Name, dryRunClusterSummary.Spec.ClusterName)
		Eventually(func() error {
			currentClusterReport := &configv1alpha1.ClusterReport{}
			err := k8sClient.Get(context.TODO(),
				types.NamespacedName{Namespace: dryRunClusterSummary.Spec.ClusterNamespace, Name: clusterReportName}, currentClusterReport)
			if err != nil {
				return err
			}
			// Another ClusterFeature is managing mysql release
			err = verifyReleaseReport(currentClusterReport, currentClusterFeature.Spec.HelmCharts[0].ReleaseNamespace,
				currentClusterFeature.Spec.HelmCharts[0].ReleaseName, string(configv1alpha1.NoHelmAction))
			if err != nil {
				return err
			}
			// If not in DryRun, it would install redis release
			err = verifyReleaseReport(currentClusterReport, currentClusterFeature.Spec.HelmCharts[1].ReleaseNamespace,
				currentClusterFeature.Spec.HelmCharts[1].ReleaseName, string(configv1alpha1.InstallHelmAction))
			if err != nil {
				return err
			}
			// postgres is Uninstall and not installed yet so no action
			err = verifyReleaseReport(currentClusterReport, currentClusterFeature.Spec.HelmCharts[2].ReleaseNamespace,
				currentClusterFeature.Spec.HelmCharts[2].ReleaseName, string(configv1alpha1.NoHelmAction))
			if err != nil {
				return err
			}
			return nil
		}, timeout, pollingInterval).Should(BeNil())

		By("Verifying ClusterReport for policy reports")
		Eventually(func() error {
			currentClusterReport := &configv1alpha1.ClusterReport{}
			err := k8sClient.Get(context.TODO(),
				types.NamespacedName{Namespace: dryRunClusterSummary.Spec.ClusterNamespace, Name: clusterReportName}, currentClusterReport)
			if err != nil {
				return err
			}
			// If not in DryRun, it would create Kong Role
			err = verifyResourceReport(currentClusterReport, "kong2", "kong-leader-election",
				"Role", "rbac.authorization.k8s.io", string(configv1alpha1.CreateResourceAction))
			if err != nil {
				return err
			}
			// Another ClusterFeature is managing this. This ClusterFeature is also referencing same ConfigMap. So Action is Update
			// as changing SyncMode will cause reconciliation and so a patch of this resource.
			err = verifyResourceReport(currentClusterReport, "kong", "kong-serviceaccount",
				"ServiceAccount", "", string(configv1alpha1.UpdateResourceAction))
			if err != nil {
				return err
			}
			return nil
		}, timeout, pollingInterval).Should(BeNil())

		Byf("Delete ClusterFeature %s", clusterFeature.Name)
		deleteClusterFeature(clusterFeature)

		Byf("Changing syncMode to Continuous and HelmCharts (all install) for ClusterFeature %s", dryRunClusterFeature.Name)
		Expect(k8sClient.Get(context.TODO(), types.NamespacedName{Name: dryRunClusterFeature.Name}, currentClusterFeature)).To(Succeed())
		currentClusterFeature.Spec.SyncMode = configv1alpha1.SyncModeContinuous
		currentClusterFeature.Spec.HelmCharts = []configv1alpha1.HelmChart{
			{
				RepositoryURL:    "https://charts.bitnami.com/bitnami",
				RepositoryName:   "bitnami",
				ChartName:        "bitnami/mysql",
				ChartVersion:     "9.3.3",
				ReleaseName:      "mysql",
				ReleaseNamespace: "mysql",
				HelmChartAction:  configv1alpha1.HelmChartActionInstall,
			},
			{
				RepositoryURL:    "https://charts.bitnami.com/bitnami",
				RepositoryName:   "bitnami",
				ChartName:        "bitnami/redis",
				ChartVersion:     "17.1.6",
				ReleaseName:      "redis",
				ReleaseNamespace: "redis",
				HelmChartAction:  configv1alpha1.HelmChartActionInstall,
			},
			{
				RepositoryURL:    "https://charts.bitnami.com/bitnami",
				RepositoryName:   "bitnami",
				ChartName:        "bitnami/postgresql",
				ChartVersion:     "11.9.1",
				ReleaseName:      "postgresql",
				ReleaseNamespace: "postgresql",
				HelmChartAction:  configv1alpha1.HelmChartActionInstall,
			},
		}
		Expect(k8sClient.Update(context.TODO(), currentClusterFeature)).To(Succeed())

		verifyClusterSummary(currentClusterFeature, kindWorkloadCluster.Namespace, kindWorkloadCluster.Name)

		Byf("Verifying ClusterSummary %s status is set to Deployed for Helm feature", clusterSummary.Name)
		verifyFeatureStatusIsProvisioned(kindWorkloadCluster.Namespace, dryRunClusterSummary.Name, configv1alpha1.FeatureHelm)

		Byf("Verifying ClusterSummary %s status is set to Deployed for Resource feature", clusterSummary.Name)
		verifyFeatureStatusIsProvisioned(kindWorkloadCluster.Namespace, dryRunClusterSummary.Name, configv1alpha1.FeatureResources)

		Byf("Changing syncMode to DryRun and HelmCharts (some install, one uninstall) for ClusterFeature %s", dryRunClusterFeature.Name)
		Expect(k8sClient.Get(context.TODO(), types.NamespacedName{Name: dryRunClusterFeature.Name}, currentClusterFeature)).To(Succeed())
		currentClusterFeature.Spec.SyncMode = configv1alpha1.SyncModeDryRun
		currentClusterFeature.Spec.PolicyRefs = []configv1alpha1.PolicyRef{
			{Kind: string(configv1alpha1.ConfigMapReferencedResourceKind), Namespace: configMapNs, Name: kongRoleConfigMap.Name},
		}
		currentClusterFeature.Spec.HelmCharts = []configv1alpha1.HelmChart{
			{
				RepositoryURL:    "https://charts.bitnami.com/bitnami",
				RepositoryName:   "bitnami",
				ChartName:        "bitnami/mysql",
				ChartVersion:     "9.3.3",
				ReleaseName:      "mysql",
				ReleaseNamespace: "mysql",
				HelmChartAction:  configv1alpha1.HelmChartActionInstall,
			},
			{
				RepositoryURL:    "https://charts.bitnami.com/bitnami",
				RepositoryName:   "bitnami",
				ChartName:        "bitnami/redis",
				ChartVersion:     "17.1.6",
				ReleaseName:      "redis",
				ReleaseNamespace: "redis",
				HelmChartAction:  configv1alpha1.HelmChartActionInstall,
			},
			{
				RepositoryURL:    "https://charts.bitnami.com/bitnami",
				RepositoryName:   "bitnami",
				ChartName:        "bitnami/postgresql",
				ChartVersion:     "11.9.1",
				ReleaseName:      "postgresql",
				ReleaseNamespace: "postgresql",
				HelmChartAction:  configv1alpha1.HelmChartActionUninstall,
			},
		}
		Expect(k8sClient.Update(context.TODO(), currentClusterFeature)).To(Succeed())

		verifyClusterSummary(currentClusterFeature, kindWorkloadCluster.Namespace, kindWorkloadCluster.Name)

		By("Verifying ClusterReport")
		Eventually(func() error {
			currentClusterReport := &configv1alpha1.ClusterReport{}
			err := k8sClient.Get(context.TODO(),
				types.NamespacedName{Namespace: dryRunClusterSummary.Spec.ClusterNamespace, Name: clusterReportName}, currentClusterReport)
			if err != nil {
				return err
			}
			// ClusterFeature is managing mysql release
			err = verifyReleaseReport(currentClusterReport, currentClusterFeature.Spec.HelmCharts[0].ReleaseNamespace,
				currentClusterFeature.Spec.HelmCharts[0].ReleaseName, string(configv1alpha1.NoHelmAction))
			if err != nil {
				return err
			}
			// ClusterFeature is managing mysql release
			err = verifyReleaseReport(currentClusterReport, currentClusterFeature.Spec.HelmCharts[1].ReleaseNamespace,
				currentClusterFeature.Spec.HelmCharts[1].ReleaseName, string(configv1alpha1.NoHelmAction))
			if err != nil {
				return err
			}
			// postgres is installed and action is Uninstall
			err = verifyReleaseReport(currentClusterReport, currentClusterFeature.Spec.HelmCharts[2].ReleaseNamespace,
				currentClusterFeature.Spec.HelmCharts[2].ReleaseName, string(configv1alpha1.UninstallHelmAction))
			if err != nil {
				return err
			}
			return nil
		}, timeout, pollingInterval).Should(BeNil())

		By("Verifying ClusterReport for policy reports")
		Eventually(func() error {
			currentClusterReport := &configv1alpha1.ClusterReport{}
			err := k8sClient.Get(context.TODO(),
				types.NamespacedName{Namespace: dryRunClusterSummary.Spec.ClusterNamespace, Name: clusterReportName}, currentClusterReport)
			if err != nil {
				return err
			}
			// If not in DryRun, it would create Kong Role
			err = verifyResourceReport(currentClusterReport, "kong2", "kong-leader-election",
				"Role", "rbac.authorization.k8s.io", string(configv1alpha1.UpdateResourceAction))
			if err != nil {
				return err
			}
			// Previously installed this resource. Now not referencing the ConfigMap with this resource anymore.
			// So action would be delete
			err = verifyResourceReport(currentClusterReport, "kong", "kong-serviceaccount",
				"ServiceAccount", "", string(configv1alpha1.DeleteResourceAction))
			if err != nil {
				return err
			}
			return nil
		}, timeout, pollingInterval).Should(BeNil())

		Byf("Changing clusterSelector for ClusterFeature %s so to not match any CAPI cluster", dryRunClusterFeature.Name)
		Expect(k8sClient.Get(context.TODO(), types.NamespacedName{Name: dryRunClusterFeature.Name}, currentClusterFeature)).To(Succeed())
		selector := "bar=foo"
		currentClusterFeature.Spec.ClusterSelector = configv1alpha1.Selector(selector)
		Expect(k8sClient.Update(context.TODO(), currentClusterFeature)).To(Succeed())

		// Since ClusterFeature is in DryRun mode, ClusterSummary should be marked as deleted but not removed
		// In DryRun mode ClusterReport still needs to be updated.

		// First wait for clusterSummary to be marked for deletion
		Eventually(func() bool {
			currentClusterSummary := &configv1alpha1.ClusterSummary{}
			err := k8sClient.Get(context.TODO(),
				types.NamespacedName{Namespace: dryRunClusterSummary.Namespace, Name: dryRunClusterSummary.Name}, currentClusterSummary)
			if err != nil {
				return false
			}
			return !currentClusterSummary.DeletionTimestamp.IsZero()
		}, timeout, pollingInterval).Should(BeTrue())

		// Then verify ClusterSummary is not removed.
		Consistently(func() bool {
			currentClusterSummary := &configv1alpha1.ClusterSummary{}
			err := k8sClient.Get(context.TODO(),
				types.NamespacedName{Namespace: dryRunClusterSummary.Namespace, Name: dryRunClusterSummary.Name}, currentClusterSummary)
			if err != nil {
				return false
			}
			return !currentClusterSummary.DeletionTimestamp.IsZero()
		}, timeout/2, pollingInterval).Should(BeTrue())

		By("Verifying ClusterReport")
		Eventually(func() error {
			currentClusterReport := &configv1alpha1.ClusterReport{}
			err := k8sClient.Get(context.TODO(),
				types.NamespacedName{Namespace: dryRunClusterSummary.Spec.ClusterNamespace, Name: clusterReportName}, currentClusterReport)
			if err != nil {
				return err
			}
			// ClusterFeature is managing mysql release
			err = verifyReleaseReport(currentClusterReport, currentClusterFeature.Spec.HelmCharts[0].ReleaseNamespace,
				currentClusterFeature.Spec.HelmCharts[0].ReleaseName, string(configv1alpha1.UninstallHelmAction))
			if err != nil {
				return err
			}
			// ClusterFeature is managing mysql release
			err = verifyReleaseReport(currentClusterReport, currentClusterFeature.Spec.HelmCharts[1].ReleaseNamespace,
				currentClusterFeature.Spec.HelmCharts[1].ReleaseName, string(configv1alpha1.UninstallHelmAction))
			if err != nil {
				return err
			}
			// postgres is installed and action is Uninstall
			err = verifyReleaseReport(currentClusterReport, currentClusterFeature.Spec.HelmCharts[2].ReleaseNamespace,
				currentClusterFeature.Spec.HelmCharts[2].ReleaseName, string(configv1alpha1.UninstallHelmAction))
			if err != nil {
				return err
			}
			return nil
		}, timeout, pollingInterval).Should(BeNil())

		By("Verifying ClusterReport for policy reports")
		Eventually(func() error {
			currentClusterReport := &configv1alpha1.ClusterReport{}
			err := k8sClient.Get(context.TODO(),
				types.NamespacedName{Namespace: dryRunClusterSummary.Spec.ClusterNamespace, Name: clusterReportName}, currentClusterReport)
			if err != nil {
				return err
			}
			// If not in DryRun, it would create Kong Role
			err = verifyResourceReport(currentClusterReport, "kong2", "kong-leader-election",
				"Role", "rbac.authorization.k8s.io", string(configv1alpha1.DeleteResourceAction))
			if err != nil {
				return err
			}
			// Previously installed this resource. Now not referencing the ConfigMap with this resource anymore.
			// So action would be delete
			err = verifyResourceReport(currentClusterReport, "kong", "kong-serviceaccount",
				"ServiceAccount", "", string(configv1alpha1.DeleteResourceAction))
			if err != nil {
				return err
			}
			return nil
		}, timeout, pollingInterval).Should(BeNil())

		Byf("Changing syncMode to Continuous for ClusterFeature %s", dryRunClusterFeature.Name)
		Expect(k8sClient.Get(context.TODO(), types.NamespacedName{Name: dryRunClusterFeature.Name}, currentClusterFeature)).To(Succeed())
		currentClusterFeature.Spec.SyncMode = configv1alpha1.SyncModeContinuous
		Expect(k8sClient.Update(context.TODO(), currentClusterFeature))

		verifyClusterSummary(currentClusterFeature, kindWorkloadCluster.Namespace, kindWorkloadCluster.Name)

		Byf("Delete ClusterFeature %s", dryRunClusterFeature.Name)
		deleteClusterFeature(dryRunClusterFeature)
	})
})

func verifyReleaseReport(clusterReport *configv1alpha1.ClusterReport,
	releaseNamespace, releaseName, action string) error {

	for i := range clusterReport.Status.ReleaseReports {
		rr := clusterReport.Status.ReleaseReports[i]
		if rr.ReleaseName == releaseName && rr.ReleaseNamespace == releaseNamespace {
			if rr.Action == action {
				return nil
			}
			return fmt.Errorf("release %s/%s action %s does not match",
				releaseNamespace, releaseName, action)
		}
	}

	return fmt.Errorf("did not find entry for release %s/%s",
		releaseNamespace, releaseName)
}

func verifyResourceReport(clusterReport *configv1alpha1.ClusterReport,
	resourceNamespace, resourceName, resourceKind, resourceGroup, action string) error {

	for i := range clusterReport.Status.ResourceReports {
		rr := clusterReport.Status.ResourceReports[i]
		if rr.Resource.Name == resourceName &&
			rr.Resource.Namespace == resourceNamespace &&
			rr.Resource.Kind == resourceKind &&
			rr.Resource.Group == resourceGroup {

			if rr.Action == action {
				return nil
			}
			return fmt.Errorf("resource %s (gropup %s) %s/%s action %s does not match",
				resourceKind, resourceGroup, resourceNamespace, resourceName, action)
		}
	}

	return fmt.Errorf("did not find entry for resource %s (gropup %s) %s/%s",
		resourceKind, resourceGroup, resourceNamespace, resourceName)
}