/*
Copyright paskal.maksim@gmail.com
Licensed under the Apache License, Version 2.0 (the "License")
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package envoycontrolplane

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/maksim-paskal/helm-blue-green/pkg/client"
	"github.com/maksim-paskal/helm-blue-green/pkg/types"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	apierrorrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/util/retry"
)

const (
	AppName                      = "envoy-control-plane"
	annotationRouteClusterWeight = AppName + "/routes.cluster.weight."
	annotationCanaryEnabled      = AppName + "/canary.enabled"
	badSamplesPromQL             = ",envoy_response_code!~'[1-4]..'"
)

type Config struct {
	ConfigMaps         []string
	ConfigMapsSelector []string
	Clusters           []*Cluster
}

func (c Config) GetAnnotation(clusterName string) string {
	return fmt.Sprintf("%s%s", annotationRouteClusterWeight, clusterName)
}

func (c Config) String() string {
	data, err := json.Marshal(c)
	if err != nil {
		log.Fatal(err)
	}

	return string(data)
}

func NewConfig() Config {
	return Config{}
}

type Cluster struct {
	ClusterName       string
	ClusterNameCanary string
}

func (c *Cluster) GetClusterNameCanary() string {
	if len(c.ClusterNameCanary) == 0 {
		return c.ClusterName + "-canary"
	}

	return c.ClusterNameCanary
}

func (e ServiceMesh) GetPromQL(promQLType types.CanaryProviderPromQLType, budgetSeconds int) (*types.CanaryProviderMetrics, error) { //nolint:lll,funlen
	result := types.CanaryProviderMetrics{
		TotalSamplesQLs: make([]string, 0),
		BadSamplesQLs:   make([]string, 0),
	}

	for _, cluster := range e.configProvider.Clusters {
		switch promQLType {
		case types.CanaryProviderPromQLTypeCanary:
			labels := fmt.Sprintf("envoy_cluster_upstream_rq{envoy_cluster_name='%s'%s}[%ds]",
				cluster.GetClusterNameCanary(),
				badSamplesPromQL,
				budgetSeconds,
			)

			result.BadSamplesQLs = append(result.BadSamplesQLs, fmt.Sprintf("sum(max_over_time(%s)-min_over_time(%s))",
				labels,
				labels,
			))

			labels = fmt.Sprintf("envoy_cluster_upstream_rq{envoy_cluster_name='%s'}[%ds]",
				cluster.GetClusterNameCanary(),
				budgetSeconds,
			)

			result.TotalSamplesQLs = append(result.TotalSamplesQLs, fmt.Sprintf("sum(max_over_time(%s)-min_over_time(%s))",
				labels,
				labels,
			))
		case types.CanaryProviderPromQLTypeABTest:
			labels := fmt.Sprintf("envoy_cluster_canary_upstream_rq{envoy_cluster_name='%s'%s}[%ds]",
				cluster.ClusterName,
				badSamplesPromQL,
				budgetSeconds,
			)

			result.BadSamplesQLs = append(result.BadSamplesQLs, fmt.Sprintf("sum(max_over_time(%s)-min_over_time(%s))",
				labels,
				labels,
			))

			labels = fmt.Sprintf("envoy_cluster_canary_upstream_rq{envoy_cluster_name='%s'}[%ds]",
				cluster.ClusterName,
				budgetSeconds,
			)

			result.TotalSamplesQLs = append(result.TotalSamplesQLs, fmt.Sprintf("sum(max_over_time(%s)-min_over_time(%s))",
				labels,
				labels,
			))
		case types.CanaryProviderPromQLTypeFull:
			labels := fmt.Sprintf("envoy_cluster_upstream_rq{envoy_cluster_name='%s'%s}[%ds]",
				cluster.ClusterName,
				badSamplesPromQL,
				budgetSeconds,
			)

			result.BadSamplesQLs = append(result.BadSamplesQLs, fmt.Sprintf("sum(max_over_time(%s)-min_over_time(%s))",
				labels,
				labels,
			))

			labels = fmt.Sprintf("envoy_cluster_upstream_rq{envoy_cluster_name='%s'}[%ds]",
				cluster.ClusterName,
				budgetSeconds,
			)

			result.TotalSamplesQLs = append(result.TotalSamplesQLs, fmt.Sprintf("sum(max_over_time(%s)-min_over_time(%s))",
				labels,
				labels,
			))
		}
	}

	return &result, nil
}

type ServiceMesh struct {
	config         types.ServiceMeshConfig
	configProvider Config
}

func (e ServiceMesh) SetCanaryPercent(ctx context.Context, percent types.CanaryProviderPercent) error {
	newAnnotation := make(map[string]string)

	for _, cluster := range e.configProvider.Clusters {
		newAnnotation[e.configProvider.GetAnnotation(cluster.GetClusterNameCanary())] = fmt.Sprintf("%d", percent)                     //nolint:lll
		newAnnotation[e.configProvider.GetAnnotation(cluster.ClusterName)] = fmt.Sprintf("%d", types.CanaryProviderPercentMax-percent) //nolint:lll
	}

	// update configmap by name
	for _, configMap := range e.configProvider.ConfigMaps {
		if err := e.updateConfigMapAnnotation(ctx, configMap, newAnnotation); err != nil {
			return errors.Wrapf(err, "failed to update config map %s", configMap)
		}
	}

	// update configmap by selector
	for _, configMapSelector := range e.configProvider.ConfigMapsSelector {
		configMapList, err := client.Client.KubeClient().CoreV1().ConfigMaps(e.config.Namespace).List(ctx, metav1.ListOptions{
			LabelSelector: configMapSelector,
		})
		if err != nil {
			return errors.Wrapf(err, "failed to list config maps by selector %s", configMapSelector)
		}

		for _, configMap := range configMapList.Items {
			if err := e.updateConfigMapAnnotation(ctx, configMap.Name, newAnnotation); err != nil {
				return errors.Wrapf(err, "failed to update config map %s", configMap.Name)
			}
		}
	}

	return nil
}

func NewServiceMesh(config types.ServiceMeshConfig) (*ServiceMesh, error) {
	envoyControlPlaneProviderConfig := NewConfig()
	if err := yaml.Unmarshal([]byte(config.Config), &envoyControlPlaneProviderConfig); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal config")
	}

	log.Debugf("loaded servicemesh config %s", envoyControlPlaneProviderConfig.String())

	servicemesh := ServiceMesh{
		config:         config,
		configProvider: envoyControlPlaneProviderConfig,
	}

	return &servicemesh, nil
}

func (e *ServiceMesh) updateConfigMapAnnotation(ctx context.Context, name string, annotations map[string]string) error { //nolint:lll,dupl
	cm, err := client.Client.KubeClient().CoreV1().ConfigMaps(e.config.Namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return errors.Wrapf(err, "error get configname %s", name)
	}

	// create annotations map if it does not exist
	// this annotation can be nil if context is canceled
	if cm.Annotations == nil {
		cm.Annotations = make(map[string]string)
	}

	for k, v := range annotations {
		cm.Annotations[k] = v
	}

	err = wait.ExponentialBackoff(retry.DefaultBackoff, func() (bool, error) {
		_, err = client.Client.KubeClient().CoreV1().ConfigMaps(e.config.Namespace).Update(ctx, cm, metav1.UpdateOptions{})
		if err != nil {
			log.WithError(err).Warn(apierrorrs.ReasonForError(err))
		}

		switch {
		case err == nil:
			return true, nil
		case apierrorrs.IsConflict(err):
			return false, nil
		case err != nil:
			return false, errors.Wrap(err, name)
		}

		return false, nil
	})
	if err != nil {
		return errors.Wrapf(err, "error updating config map %s", name)
	}

	return nil
}

func (e *ServiceMesh) updateEndpointAnnotation(ctx context.Context, name string, annotations map[string]string) error { //nolint:lll,dupl
	cm, err := client.Client.KubeClient().CoreV1().Endpoints(e.config.Namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return errors.Wrapf(err, "error get endpoint %s", name)
	}

	// create annotations map if it does not exist
	// this annotation can be nil if context is canceled
	if cm.Annotations == nil {
		cm.Annotations = make(map[string]string)
	}

	for k, v := range annotations {
		cm.Annotations[k] = v
	}

	err = wait.ExponentialBackoff(retry.DefaultBackoff, func() (bool, error) {
		_, err = client.Client.KubeClient().CoreV1().Endpoints(e.config.Namespace).Update(ctx, cm, metav1.UpdateOptions{})
		if err != nil {
			log.WithError(err).Warn(apierrorrs.ReasonForError(err))
		}

		switch {
		case err == nil:
			return true, nil
		case apierrorrs.IsConflict(err):
			return false, nil
		case err != nil:
			return false, errors.Wrap(err, name)
		}

		return false, nil
	})
	if err != nil {
		return errors.Wrapf(err, "error updating endpoint %s", name)
	}

	return nil
}

func (e *ServiceMesh) SetServiceCanaryMode(ctx context.Context, service string, isStart bool) error {
	newAnnotation := map[string]string{
		annotationCanaryEnabled: strconv.FormatBool(isStart),
	}

	err := e.updateEndpointAnnotation(ctx, service, newAnnotation)
	if err != nil {
		return errors.Wrapf(err, "failed to update endpoint %s", service)
	}

	return nil
}

func (e *ServiceMesh) SetServiceABTestMode(ctx context.Context, service string, isStart bool) error {
	err := e.SetServiceCanaryMode(ctx, service, isStart)

	return errors.Wrap(err, "failed to set service abtest mode")
}
