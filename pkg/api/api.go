package api

import (
	"context"
	"fmt"
	"time"

	"github.com/maksim-paskal/helm-blue-green/pkg/client"
	"github.com/maksim-paskal/helm-blue-green/pkg/config"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	apierrorrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/retry"
)

func kube() *kubernetes.Clientset {
	return client.Client.KubeClient()
}

func CopyDeployment(ctx context.Context, item config.Deployment, values *config.Type) error {
	log.Debugf("Copying deployment %s/%s", values.Namespace, item.Name)

	deploy, err := kube().AppsV1().Deployments(values.Namespace).Get(ctx, item.Name, metav1.GetOptions{})
	if err != nil {
		return errors.Wrap(err, "error getting deployment")
	}

	minReplicas := item.GetMinReplicas(values)

	newDeploy := deploy.DeepCopy()
	newDeploy.ResourceVersion = ""
	newDeploy.Labels[values.Version.Key()] = values.Version.Value
	newDeploy.Spec.Template.Labels[values.Version.Key()] = values.Version.Value
	newDeploy.Spec.Selector.MatchLabels[values.Version.Key()] = values.Version.Value

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

	hpa := item.GetHpa(values)

	if hpa.Enabled {
		err := createHPA(ctx, newDeploy.Name, hpa, values)
		if err != nil {
			return errors.Wrap(err, "error creating HPA")
		}
	}

	pdb := item.GetPdb(values)

	if pdb.Enabled {
		err := createPdb(ctx, newDeploy, pdb, values)
		if err != nil {
			return errors.Wrap(err, "error creating PDB")
		}
	}

	return nil
}

func createHPA(ctx context.Context, deploymentName string, hpaConfig config.Hpa, values *config.Type) error {
	hpa := &autoscalingv2.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Name: deploymentName,
			Labels: map[string]string{
				values.Version.Key(): values.Version.Value,
			},
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

	_, err := kube().AutoscalingV2().HorizontalPodAutoscalers(values.Namespace).Create(ctx, hpa, metav1.CreateOptions{})
	if err != nil {
		return errors.Wrap(err, "error creating hpa")
	}

	return nil
}

func createPdb(ctx context.Context, newDeploy *appsv1.Deployment, pdbConfig config.Pdb, values *config.Type) error {
	minAvailable := intstr.FromInt(pdbConfig.MinAvailable)

	pdb := &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{
			Name: newDeploy.Name,
			Labels: map[string]string{
				values.Version.Key(): values.Version.Value,
			},
		},

		Spec: policyv1.PodDisruptionBudgetSpec{
			MinAvailable: &minAvailable,
			Selector: &metav1.LabelSelector{
				MatchLabels: newDeploy.Spec.Selector.MatchLabels,
			},
		},
	}

	_, err := kube().PolicyV1().PodDisruptionBudgets(values.Namespace).Create(ctx, pdb, metav1.CreateOptions{})
	if err != nil {
		return errors.Wrap(err, "error creating PDB")
	}

	return nil
}

func CopyService(ctx context.Context, item config.Service, values *config.Type) error {
	service, err := kube().CoreV1().Services(values.Namespace).Get(ctx, item.Name, metav1.GetOptions{})
	if err != nil {
		return errors.Wrap(err, "error getting service")
	}

	newService := service.DeepCopy()
	newService.ResourceVersion = ""
	newService.Spec.ClusterIP = ""
	newService.Spec.ClusterIPs = nil
	newService.Labels[values.Version.Key()] = values.Version.Value
	newService.Spec.Selector[values.Version.Key()] = values.Version.Value
	newService.Name = fmt.Sprintf("%s-%s", item.Name, values.Version.Value)

	log.Debugf("New name %s/%s", values.Namespace, newService.Name)

	_, err = kube().CoreV1().Services(values.Namespace).Create(ctx, newService, metav1.CreateOptions{})
	if err != nil {
		return errors.Wrap(err, "error creating service")
	}

	return nil
}

func CopyConfigMap(ctx context.Context, item config.ConfigMap, values *config.Type) error {
	configMap, err := kube().CoreV1().ConfigMaps(values.Namespace).Get(ctx, item.Name, metav1.GetOptions{})
	if err != nil {
		return errors.Wrap(err, "error getting service")
	}

	newConfigMap := configMap.DeepCopy()
	newConfigMap.ResourceVersion = ""
	newConfigMap.Labels[values.Version.Key()] = values.Version.Value
	newConfigMap.Name = fmt.Sprintf("%s-%s", item.Name, values.Version.Value)

	log.Debugf("New name %s/%s", values.Namespace, newConfigMap.Name)

	_, err = kube().CoreV1().ConfigMaps(values.Namespace).Create(ctx, newConfigMap, metav1.CreateOptions{})
	if err != nil {
		return errors.Wrap(err, "error creating configmap")
	}

	return nil
}

func WaitForPodsToBeReady(ctx context.Context, values *config.Type) error { //nolint:cyclop
	log.Debugf("Waiting for pods to be ready %s/%s", values.Namespace, values.Version.String())

	targetMinReplicas := 0

	for _, item := range values.Deployments {
		targetMinReplicas += int(item.GetMinReplicas(values))
	}

	startTime := time.Now()

	for {
		if ctx.Err() != nil {
			return errors.Wrap(ctx.Err(), "context error")
		}

		if time.Since(startTime) > time.Duration(values.PodCheckMaxSeconds)*time.Second {
			return errors.New("timeout waiting for pods to be ready")
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

		log.Infof("Waiting for pods %s to be ready %d/%d", values.Version.String(), ready, targetMinReplicas)

		if ready >= targetMinReplicas {
			break
		}

		time.Sleep(time.Duration(values.PodCheckIntervalSeconds) * time.Second)
	}

	return nil
}

func UpdateServicesSelector(ctx context.Context, item config.Service, values *config.Type) error {
	service, err := kube().CoreV1().Services(values.Namespace).Get(ctx, item.Name, metav1.GetOptions{})
	if err != nil {
		return errors.Wrap(err, "error getting service")
	}

	service.Spec.Selector[values.Version.Key()] = values.Version.Value

	err = wait.ExponentialBackoff(retry.DefaultBackoff, func() (bool, error) {
		_, err = kube().CoreV1().Services(values.Namespace).Update(ctx, service, metav1.UpdateOptions{})
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

func DeleteVersion(ctx context.Context, config *config.Type, deleteType DeleteVersionType) error { //nolint:funlen,cyclop,lll,gocognit,gocyclo
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

	deployments, err := client.Client.KubeClient().AppsV1().Deployments(config.Namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
	})
	if err != nil {
		return errors.Wrap(err, "error listing deployment")
	}

	for _, deploy := range deployments.Items {
		deployCopy := deploy

		err := wait.ExponentialBackoff(retry.DefaultBackoff, func() (bool, error) {
			log.Debugf("Deleting deployment %s/%s", config.Namespace, deployCopy.Name)

			err = kube().AppsV1().Deployments(config.Namespace).Delete(ctx, deployCopy.Name, metav1.DeleteOptions{})
			switch {
			case err == nil:
				return true, nil
			case apierrorrs.IsConflict(err):
				return false, nil
			case err != nil:
				return false, errors.Wrap(err, deployCopy.Name)
			}

			return false, nil
		})
		if err != nil {
			return errors.Wrap(err, "error deleting deployment")
		}
	}

	configMaps, err := client.Client.KubeClient().CoreV1().ConfigMaps(config.Namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
	})
	if err != nil {
		return errors.Wrap(err, "error listing configMaps")
	}

	for _, configMap := range configMaps.Items {
		configMapCopy := configMap

		err := wait.ExponentialBackoff(retry.DefaultBackoff, func() (bool, error) {
			log.Debugf("Deleting configmap %s/%s", config.Namespace, configMapCopy.Name)

			err = kube().CoreV1().ConfigMaps(config.Namespace).Delete(ctx, configMapCopy.Name, metav1.DeleteOptions{})
			switch {
			case err == nil:
				return true, nil
			case apierrorrs.IsConflict(err):
				return false, nil
			case err != nil:
				return false, errors.Wrap(err, configMapCopy.Name)
			}

			return false, nil
		})
		if err != nil {
			return errors.Wrap(err, "error deleting configmap")
		}
	}

	hpas, err := kube().AutoscalingV2().HorizontalPodAutoscalers(config.Namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
	})
	if err != nil {
		return errors.Wrap(err, "error listing hpa")
	}

	for _, hpa := range hpas.Items {
		hpaCopy := hpa

		err := wait.ExponentialBackoff(retry.DefaultBackoff, func() (bool, error) {
			log.Debugf("Deleting hpa %s/%s", config.Namespace, hpaCopy.Name)

			err = kube().AutoscalingV2().HorizontalPodAutoscalers(config.Namespace).Delete(ctx, hpaCopy.Name, metav1.DeleteOptions{}) //nolint:lll
			switch {
			case err == nil:
				return true, nil
			case apierrorrs.IsConflict(err):
				return false, nil
			case err != nil:
				return false, errors.Wrap(err, hpaCopy.Name)
			}

			return false, nil
		})
		if err != nil {
			return errors.Wrap(err, "error deleting hpa")
		}
	}

	pdbs, err := kube().PolicyV1().PodDisruptionBudgets(config.Namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
	})
	if err != nil {
		return errors.Wrap(err, "error listing pdb")
	}

	for _, pdb := range pdbs.Items {
		pdbCopy := pdb

		err := wait.ExponentialBackoff(retry.DefaultBackoff, func() (bool, error) {
			log.Debugf("Deleting pdb %s/%s", config.Namespace, pdbCopy.Name)

			err = kube().PolicyV1().PodDisruptionBudgets(config.Namespace).Delete(ctx, pdbCopy.Name, metav1.DeleteOptions{})
			switch {
			case err == nil:
				return true, nil
			case apierrorrs.IsConflict(err):
				return false, nil
			case err != nil:
				return false, errors.Wrap(err, pdbCopy.Name)
			}

			return false, nil
		})
		if err != nil {
			return errors.Wrap(err, "error deleting pdb")
		}
	}

	services, err := client.Client.KubeClient().CoreV1().Services(config.Namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
	})
	if err != nil {
		return errors.Wrap(err, "error listing service")
	}

	for _, service := range services.Items {
		serviceCopy := service

		err := wait.ExponentialBackoff(retry.DefaultBackoff, func() (bool, error) {
			log.Debugf("Deleting service %s/%s", config.Namespace, serviceCopy.Name)

			err = kube().CoreV1().Services(config.Namespace).Delete(ctx, serviceCopy.Name, metav1.DeleteOptions{})
			switch {
			case err == nil:
				return true, nil
			case apierrorrs.IsConflict(err):
				return false, nil
			case err != nil:
				return false, errors.Wrap(err, serviceCopy.Name)
			}

			return false, nil
		})
		if err != nil {
			return errors.Wrap(err, "error deleting service")
		}
	}

	return nil
}

func ScaleDeployment(ctx context.Context, item config.Deployment, replicas int32, values *config.Type) error {
	deployment, err := kube().AppsV1().Deployments(values.Namespace).Get(ctx, item.Name, metav1.GetOptions{})
	if err != nil {
		return errors.Wrap(err, "error getting deployment")
	}

	// if current replicas is equal to target replicas - do nothing
	if deployment.Spec.Replicas == &replicas {
		return nil
	}

	deployment.Spec.Replicas = &replicas

	err = wait.ExponentialBackoff(retry.DefaultBackoff, func() (bool, error) {
		_, err = kube().AppsV1().Deployments(values.Namespace).Update(ctx, deployment, metav1.UpdateOptions{})
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

func GetCurrentVersion(ctx context.Context, values *config.Type) (string, error) {
	// use first service to get current version of traffic
	service, err := kube().CoreV1().Services(values.Namespace).Get(ctx, values.Services[0].Name, metav1.GetOptions{})
	if err != nil {
		return "", errors.Wrap(err, "error getting service")
	}

	version, ok := service.Spec.Selector[values.Version.Key()]
	if !ok {
		return "", nil
	}

	return version, nil
}

func DeleteOrigins(ctx context.Context, values *config.Type) error { //nolint:cyclop
	for _, item := range values.Deployments {
		itemCopy := item

		err := wait.ExponentialBackoff(retry.DefaultBackoff, func() (bool, error) {
			err := kube().AppsV1().Deployments(values.Namespace).Delete(ctx, itemCopy.Name, metav1.DeleteOptions{})
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
