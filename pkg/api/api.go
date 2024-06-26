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
	"fmt"
	"strings"
	"time"

	"github.com/maksim-paskal/helm-blue-green/pkg/client"
	"github.com/maksim-paskal/helm-blue-green/pkg/config"
	"github.com/maksim-paskal/helm-blue-green/pkg/types"
	"github.com/maksim-paskal/helm-blue-green/pkg/utils"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	apierrorrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/retry"
)

func kube() *kubernetes.Clientset {
	return client.Client.KubeClient()
}

const (
	labelNamespace = "helm-blue-green"
	labelScope     = labelNamespace + "/scope"
	labelVersion   = labelNamespace + "/version"
)

func labels(version *types.Version, labels map[string]string) {
	// remove all old labels from previous versions
	for k := range labels {
		if strings.HasPrefix(k, labelNamespace+"/") {
			delete(labels, k)
		}
	}

	labels[version.Key()] = version.Value
	labels[labelScope] = version.Scope
	labels[labelVersion] = version.Value
}

func CopyDeployment(ctx context.Context, item *config.Deployment, values *config.Type) error {
	log.Debugf("Copying deployment %s/%s", values.Namespace, item.Name)

	deploy, err := kube().AppsV1().Deployments(values.Namespace).Get(ctx, item.Name, metav1.GetOptions{})
	if err != nil {
		return errors.Wrap(err, "error getting deployment")
	}

	minReplicas := item.MinReplicas

	newDeploy := deploy.DeepCopy()
	newDeploy.ResourceVersion = ""

	labels(values.Version, newDeploy.Labels)
	labels(values.Version, newDeploy.Spec.Template.Labels)
	labels(values.Version, newDeploy.Spec.Selector.MatchLabels)

	newDeploy.Name = fmt.Sprintf("%s-%s", item.Name, values.Version.Value)

	newDeploy.Spec.Replicas = &minReplicas

	// append version env to all containers in pod
	for i := range newDeploy.Spec.Template.Spec.Containers {
		newDeploy.Spec.Template.Spec.Containers[i].Env = append(newDeploy.Spec.Template.Spec.Containers[i].Env, corev1.EnvVar{
			Name:  "DEPLOYMENT_VERSION",
			Value: values.Version.Value,
		})
	}

	log.Debugf("New name %s/%s", values.Namespace, newDeploy.Name)

	_, err = kube().AppsV1().Deployments(values.Namespace).Create(ctx, newDeploy, metav1.CreateOptions{})
	if err != nil {
		return errors.Wrap(err, "error creating deployment")
	}

	if item.Hpa.Enabled {
		err := createHPA(ctx, newDeploy.Name, item.Hpa, values)
		if err != nil {
			return errors.Wrap(err, "error creating HPA")
		}
	}

	if item.Pdb.Enabled {
		err := createPdb(ctx, newDeploy, item.Pdb, values)
		if err != nil {
			return errors.Wrap(err, "error creating PDB")
		}
	}

	return nil
}

func CopyService(ctx context.Context, item *config.Service, values *config.Type) error {
	service, err := kube().CoreV1().Services(values.Namespace).Get(ctx, item.Name, metav1.GetOptions{})
	if err != nil {
		return errors.Wrap(err, "error getting service")
	}

	newService := service.DeepCopy()
	newService.ResourceVersion = ""
	newService.Spec.ClusterIP = ""
	newService.Spec.ClusterIPs = nil

	labels(values.Version, newService.Labels)
	labels(values.Version, newService.Spec.Selector)

	newService.Name = fmt.Sprintf("%s-%s", item.Name, values.Version.Value)

	log.Debugf("New name %s/%s", values.Namespace, newService.Name)

	_, err = kube().CoreV1().Services(values.Namespace).Create(ctx, newService, metav1.CreateOptions{})
	if err != nil {
		return errors.Wrap(err, "error creating service")
	}

	return nil
}

func CopyConfigMap(ctx context.Context, item *config.ConfigMap, values *config.Type) error {
	configMap, err := kube().CoreV1().ConfigMaps(values.Namespace).Get(ctx, item.Name, metav1.GetOptions{})
	if err != nil {
		return errors.Wrap(err, "error getting service")
	}

	newConfigMap := configMap.DeepCopy()
	newConfigMap.ResourceVersion = ""

	labels(values.Version, newConfigMap.Labels)

	newConfigMap.Name = fmt.Sprintf("%s-%s", item.Name, values.Version.Value)

	log.Debugf("New name %s/%s", values.Namespace, newConfigMap.Name)

	_, err = kube().CoreV1().ConfigMaps(values.Namespace).Create(ctx, newConfigMap, metav1.CreateOptions{})
	if err != nil {
		return errors.Wrap(err, "error creating configmap")
	}

	return nil
}

func WaitForPodsToBeReady(ctx context.Context, values *config.Type) error { //nolint:cyclop
	defer utils.TimeTrack("api.WaitForPodsToBeReady", time.Now())

	log.Debugf("Waiting for pods to be ready %s/%s", values.Namespace, values.Version.String())

	targetMinReplicas := 0
	counterPodsAvailable := 0

	for _, item := range values.Deployments {
		targetMinReplicas += int(item.MinReplicas)
	}

	for {
		if ctx.Err() != nil {
			return errors.Wrap(ctx.Err(), "context error")
		}

		pods, err := kube().CoreV1().Pods(values.Namespace).List(ctx, metav1.ListOptions{
			LabelSelector: values.Version.String(),
		})
		if err != nil {
			return errors.Wrap(err, "error getting pods")
		}

		ready := 0

		// is pod is ready
		for _, pod := range pods.Items {
			if pod.Status.Phase == corev1.PodRunning {
				for _, v := range pod.Status.Conditions {
					if v.Type == corev1.PodReady && v.Status == "True" {
						ready++
					}
				}
			}
		}

		if ready >= targetMinReplicas {
			log.Info("All pods are ready, waiting for additional time to be sure")

			counterPodsAvailable++
		}

		if counterPodsAvailable > int(values.PodCheckAvailableTimes) {
			break
		}

		log.Infof("Waiting %s for pods %s to be ready %d/%d",
			values.GetPodCheckInterval().String(),
			values.Version.String(),
			ready,
			targetMinReplicas,
		)
		utils.SleepWithContext(ctx, values.GetPodCheckInterval())
	}

	return nil
}

func UpdateServiceSelector(ctx context.Context, serviceName string, values *config.Type, version *types.Version) error {
	err := wait.ExponentialBackoff(retry.DefaultBackoff, func() (bool, error) {
		service, err := kube().CoreV1().Services(values.Namespace).Get(ctx, serviceName, metav1.GetOptions{})
		if err != nil {
			return false, errors.Wrap(err, "error getting service")
		}

		labels(version, service.Spec.Selector)

		_, err = kube().CoreV1().Services(values.Namespace).Update(ctx, service, metav1.UpdateOptions{})
		if err != nil {
			log.WithError(err).Warn(apierrorrs.ReasonForError(err))
		}

		switch {
		case err == nil:
			return true, nil
		case apierrorrs.IsConflict(err):
			return false, nil
		case err != nil:
			return false, errors.Wrap(err, service.Name)
		}

		return false, nil
	})
	if err != nil {
		return errors.Wrap(err, "error updating service")
	}

	return nil
}

type DeleteVersionType string

const (
	DeleteNewVersion  DeleteVersionType = "new"
	DeleteOldVersions DeleteVersionType = "old"
)

func DeleteVersion(ctx context.Context, config *config.Type, deleteType DeleteVersionType) error {
	labelSelector := config.Version.Key()

	switch deleteType {
	case DeleteNewVersion:
		labelSelector += fmt.Sprintf(",%s=%s", config.Version.Key(), config.Version.Value)
	case DeleteOldVersions:
		labelSelector += fmt.Sprintf(",%s!=%s", config.Version.Key(), config.Version.Value)
	default:
		return errors.Errorf("unknown delete type %s", deleteType)
	}

	log.Debugf("Deleting objects with labels %s", labelSelector)

	if err := deleteDeployments(ctx, config, labelSelector); err != nil {
		return errors.Wrap(err, "error deleting deployments")
	}

	if err := deleteConfigMaps(ctx, config, labelSelector); err != nil {
		return errors.Wrap(err, "error deleting configmaps")
	}

	if err := deleteHpa(ctx, config, labelSelector); err != nil {
		return errors.Wrap(err, "error deleting hpa")
	}

	if err := deletePdb(ctx, config, labelSelector); err != nil {
		return errors.Wrap(err, "error deleting pdb")
	}

	if err := deleteServices(ctx, config, labelSelector); err != nil {
		return errors.Wrap(err, "error deleting services")
	}

	return nil
}

func ScaleDeployment(ctx context.Context, item *config.Deployment, replicas int32, values *config.Type) error {
	err := wait.ExponentialBackoff(retry.DefaultBackoff, func() (bool, error) {
		deployment, err := kube().AppsV1().Deployments(values.Namespace).Get(ctx, item.Name, metav1.GetOptions{})
		if err != nil {
			return false, errors.Wrap(err, "error getting deployment")
		}

		// if current replicas is equal to target replicas - do nothing
		if deployment.Spec.Replicas == &replicas {
			return false, nil
		}

		deployment.Spec.Replicas = &replicas

		_, err = kube().AppsV1().Deployments(values.Namespace).Update(ctx, deployment, metav1.UpdateOptions{})
		if err != nil {
			log.WithError(err).Warn(apierrorrs.ReasonForError(err))
		}

		switch {
		case err == nil:
			return true, nil
		case apierrorrs.IsConflict(err):
			return false, nil
		case err != nil:
			return false, errors.Wrap(err, deployment.Name)
		}

		return false, nil
	})
	if err != nil {
		return errors.Wrap(err, "error updating deployment")
	}

	return nil
}

func GetCurrentVersion(ctx context.Context, values *config.Type) (*types.Version, error) {
	version := types.Version{
		Scope: values.Version.Scope,
		Value: "",
	}

	// use first service to get current version of traffic
	service, err := kube().CoreV1().Services(values.Namespace).Get(ctx, values.Services[0].Name, metav1.GetOptions{})
	if err != nil {
		return &version, errors.Wrap(err, "error getting service")
	}

	serviceVersion, ok := service.Spec.Selector[values.Version.Key()]
	if !ok {
		return &version, nil
	}

	version.Value = serviceVersion

	return &version, nil
}

func DeleteOrigins(ctx context.Context, values *config.Type) error { //nolint:cyclop
	for _, item := range values.Deployments {
		itemCopy := item

		err := wait.ExponentialBackoff(retry.DefaultBackoff, func() (bool, error) {
			err := kube().AppsV1().Deployments(values.Namespace).Delete(ctx, itemCopy.Name, metav1.DeleteOptions{})
			if err != nil {
				log.WithError(err).Warn(apierrorrs.ReasonForError(err))
			}

			switch {
			case err == nil:
				return true, nil
			case apierrorrs.IsConflict(err):
				return false, nil
			case err != nil:
				return false, errors.Wrap(err, itemCopy.Name)
			}

			return false, nil
		})
		if err != nil {
			return errors.Wrapf(err, "error deleting deployment %s", item.Name)
		}
	}

	for _, item := range values.ConfigMaps {
		itemCopy := item

		err := wait.ExponentialBackoff(retry.DefaultBackoff, func() (bool, error) {
			err := kube().CoreV1().ConfigMaps(values.Namespace).Delete(ctx, itemCopy.Name, metav1.DeleteOptions{})
			if err != nil {
				log.WithError(err).Warn(apierrorrs.ReasonForError(err))
			}

			switch {
			case err == nil:
				return true, nil
			case apierrorrs.IsConflict(err):
				return false, nil
			case err != nil:
				return false, errors.Wrap(err, itemCopy.Name)
			}

			return false, nil
		})
		if err != nil {
			return errors.Wrapf(err, "error deleting configmap %s", item.Name)
		}
	}

	return nil
}
