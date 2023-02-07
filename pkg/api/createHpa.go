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
package api

import (
	"context"

	"github.com/maksim-paskal/helm-blue-green/pkg/config"
	"github.com/pkg/errors"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func createHPA(ctx context.Context, deploymentName string, hpaConfig config.Hpa, values *config.Type) error {
	hpa := &autoscalingv2.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Name:   deploymentName,
			Labels: make(map[string]string),
		},
		Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
			ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{
				APIVersion: "apps/v1",
				Kind:       "Deployment",
				Name:       deploymentName,
			},
			MinReplicas: &hpaConfig.MinReplicas,
			MaxReplicas: hpaConfig.MaxReplicas,
			Metrics: []autoscalingv2.MetricSpec{
				{
					Type: autoscalingv2.ResourceMetricSourceType,
					Resource: &autoscalingv2.ResourceMetricSource{
						Name: "cpu",
						Target: autoscalingv2.MetricTarget{
							Type:               autoscalingv2.UtilizationMetricType,
							AverageUtilization: &hpaConfig.AverageUtilization,
						},
					},
				},
			},
		},
	}

	labels(values, hpa.ObjectMeta.Labels)

	_, err := kube().AutoscalingV2().HorizontalPodAutoscalers(values.Namespace).Create(ctx, hpa, metav1.CreateOptions{})
	if err != nil {
		return errors.Wrap(err, "error creating hpa")
	}

	return nil
}
