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
package api //nolint:dupl

import (
	"context"

	"github.com/maksim-paskal/helm-blue-green/pkg/config"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	apierrorrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/retry"
)

func deleteServices(ctx context.Context, config *config.Type, labelSelector string) error {
	const deleteTypeName = "Services"

	deleteType := kube().CoreV1().Services(config.Namespace)

	err := wait.ExponentialBackoff(retry.DefaultBackoff, func() (bool, error) {
		items, err := deleteType.List(ctx, metav1.ListOptions{
			LabelSelector: labelSelector,
		})
		if err != nil {
			return false, errors.Wrapf(err, "error listing %s", deleteTypeName)
		}

		for _, item := range items.Items {
			log.Debugf("Deleting %s %s/%s", deleteTypeName, item.Namespace, item.Name)

			err = deleteType.Delete(ctx, item.Name, metav1.DeleteOptions{})
			if err != nil {
				log.WithError(err).Warn(apierrorrs.ReasonForError(err))
			}

			switch {
			case apierrorrs.IsConflict(err):
				return false, nil
			case err != nil:
				return false, errors.Wrap(err, item.Name)
			}
		}

		return true, nil
	})
	if err != nil {
		return errors.Wrapf(err, "error deleting %s", deleteTypeName)
	}

	return nil
}
