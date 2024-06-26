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
	appsv1 "k8s.io/api/apps/v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func createPdb(ctx context.Context, newDeploy *appsv1.Deployment, pdbConfig *config.Pdb, values *config.Type) error {
	pdb := &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{
			Name:   newDeploy.Name,
			Labels: make(map[string]string),
		},

		Spec: policyv1.PodDisruptionBudgetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: newDeploy.Spec.Selector.MatchLabels,
			},
		},
	}
	labels(values.Version, pdb.ObjectMeta.Labels)

	if pdbConfig.MinAvailable > 0 {
		pdb.Spec.MinAvailable = pdbConfig.GetMinAvailable()
	}

	if pdbConfig.MaxUnavailable > 0 {
		pdb.Spec.MaxUnavailable = pdbConfig.GetMaxUnavailable()
	}

	_, err := kube().PolicyV1().PodDisruptionBudgets(values.Namespace).Create(ctx, pdb, metav1.CreateOptions{})
	if err != nil {
		return errors.Wrap(err, "error creating PDB")
	}

	return nil
}
