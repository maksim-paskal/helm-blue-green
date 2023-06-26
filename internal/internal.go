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
package internal

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/maksim-paskal/helm-blue-green/pkg/api"
	"github.com/maksim-paskal/helm-blue-green/pkg/canary"
	"github.com/maksim-paskal/helm-blue-green/pkg/client"
	"github.com/maksim-paskal/helm-blue-green/pkg/config"
	"github.com/maksim-paskal/helm-blue-green/pkg/metrics"
	"github.com/maksim-paskal/helm-blue-green/pkg/prometheus"
	"github.com/maksim-paskal/helm-blue-green/pkg/types"
	"github.com/maksim-paskal/helm-blue-green/pkg/utils"
	"github.com/maksim-paskal/helm-blue-green/pkg/webhook"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

const (
	rollbackTimeout = 10 * time.Second
	webhookTimeout  = 10 * time.Second
)

func Start(ctx context.Context) error { //nolint:funlen
	defer utils.TimeTrack("internal.Start", time.Now())

	err := client.Init()
	if err != nil {
		return errors.Wrap(err, "error initializing client")
	}

	if err := config.Load(ctx); err != nil {
		return errors.Wrap(err, "error loading config")
	}

	if err := prometheus.Init(); err != nil {
		return errors.Wrap(err, "error initializing prometheus")
	}

	log.Debugf("Using config:\n%s", config.Get().String())

	values := config.Get()

	event := webhook.Event{
		Name:        values.Name,
		Namespace:   values.Namespace,
		Environment: values.Environment,
		Version:     values.Version.Value,
	}

	startTime := time.Now()

	var processError error

	log.Infof("Creating processing context with timeout %s", config.Get().GetMaxProcessingTime())

	ctx, cancel := context.WithTimeout(ctx, config.Get().GetMaxProcessingTime())
	defer cancel()

	if result, err := process(ctx); err != nil {
		// if error happened during processing, send failed event
		event.Type = webhook.EventTypeFailed
		processError = err

		// try to rollback all changes
		// use background context because processing context can be canceled
		rollbackContext, rollbackCancel := context.WithTimeout(context.Background(), rollbackTimeout)
		defer rollbackCancel()

		if errors.Is(err, types.ErrNewReleaseBadQuality) {
			event.Type = webhook.EventTypeBadQuality
		}

		if err = rollback(rollbackContext); err != nil { //nolint:contextcheck
			log.WithError(err).Error("error rolling back")
		}
	} else {
		// if no error happened, send success event
		event.Type = result.EventType
		event.OldVersion = config.Get().CurrentVersion.Value
	}

	event.Duration = time.Since(startTime).Round(time.Second).String()

	log.Info("Executing webhooks")

	// use background context because context can be canceled
	webhookContext, webhookCancel := context.WithTimeout(context.Background(), webhookTimeout)
	defer webhookCancel()

	// get metrics to event
	event.Metrics = metrics.GetMetricsMap()

	if err := webhook.Execute(webhookContext, event, values); err != nil { //nolint:contextcheck
		return errors.Wrap(err, "error executing webhooks")
	}

	// send process error after executing webhooks
	if event.Type == webhook.EventTypeFailed {
		return processError
	}

	return nil
}

type processResult struct {
	EventType webhook.EventType
}

func process(ctx context.Context) (*processResult, error) { //nolint:cyclop,funlen,gocognit
	values := config.Get()
	result := processResult{}

	if config.Get().Prometheus.HasLocalPrometheus() {
		// wait for prometheus sidecar, need for making management action to prometheus
		prometheus.WaitForPrometheus(ctx)

		// create autodiscovery config for local prometheus
		go prometheus.ScheduleCreationPrometheusConfig(ctx)
	}

	log.Info("Getting current version of service selector")

	currentVersion, err := api.GetCurrentVersion(ctx, values)
	if err != nil {
		return nil, errors.Wrap(err, "error getting current version")
	}

	values.CurrentVersion = currentVersion

	// stop processing if traffic was already switched to new version
	if values.CurrentVersion.Value == values.Version.Value {
		log.Infof("Version %s is already deployed", values.CurrentVersion)

		result.EventType = webhook.EventTypeAlreadyDeployed

		return &result, nil
	}

	log.Info("Delete unsuccessful deployment")

	if err := api.DeleteVersion(ctx, values, api.DeleteNewVersion); err != nil {
		return nil, errors.Wrap(err, "error deleting old version")
	}

	log.Info("Scale original deployments to 0")

	if err := scaleOriginalDeploymens(ctx, values); err != nil {
		return nil, errors.Wrap(err, "error scaling original deployments")
	}

	log.Info("Creating new versions")

	if err := createNewVersions(ctx, values); err != nil {
		return nil, errors.Wrap(err, "error creating new versions")
	}

	if values.HasCanary() {
		log.Info("Reset canary traffic")

		if err := turnOffCanary(ctx, values); err != nil {
			return nil, errors.Wrap(err, "error setting canary percent")
		}

		log.Info("Update canary service to new version")

		if err := updateCanaryServices(ctx, values); err != nil {
			return nil, errors.Wrap(err, "error updating canary service")
		}
	}

	log.Info("Waiting for all deployments to be ready")

	if err := api.WaitForPodsToBeReady(ctx, values); err != nil {
		return nil, errors.Wrap(err, "error waiting for pods to be ready")
	}

	if values.HasCanary() && values.Canary.Strategy.HasPhase1() {
		log.Infof("Start canary traffic shifting, strategy=%s", values.Canary.Phase1.Strategy)

		if err := canary.StartCanaryPhase1(ctx, values); err != nil {
			// if traffic switcher failed, set canary percent to min value
			if err := turnOffCanary(ctx, values); err != nil {
				log.WithError(err).Error("error setting canary percent")
			}

			return nil, errors.Wrap(err, "errors in phase 1")
		}

		// wait for all pods to be ready after canary phase 1
		log.Info("Waiting for all deployments to be ready")

		if err := api.WaitForPodsToBeReady(ctx, values); err != nil {
			return nil, errors.Wrap(err, "error waiting for pods to be ready")
		}
	}

	log.Info("Update service to new version")

	if err := updateServicesSelector(ctx, values, values.Version); err != nil {
		return nil, errors.Wrap(err, "error updating service selector")
	}

	if values.HasCanary() && values.Canary.Strategy.HasPhase1() {
		log.Info("Move traffic to main service")

		if err := turnOffCanary(ctx, values); err != nil {
			return nil, errors.Wrap(err, "error setting canary percent")
		}
	}

	if values.HasCanary() && values.Canary.Strategy.HasPhase2() {
		log.Info("Analyse full traffic")

		if err := canary.StartCanaryPhase2(ctx, values); err != nil {
			return nil, errors.Wrap(err, "errors in phase 2")
		}
	}

	values.SetCanNotRollback()

	log.Info("Delete old versions")

	if err := api.DeleteVersion(ctx, values, api.DeleteOldVersions); err != nil {
		return nil, errors.Wrap(err, "error deleting old version")
	}

	if values.DeleteOrigins {
		log.Info("Deleting origin deployments and configmaps")

		if err := api.DeleteOrigins(ctx, values); err != nil {
			return nil, errors.Wrap(err, "error deleting origin")
		}
	}

	result.EventType = webhook.EventTypeSuccess

	return &result, nil
}

func createNewVersions(ctx context.Context, values *config.Type) error {
	var processErrors sync.Map

	var wg sync.WaitGroup

	for _, deployment := range values.Deployments {
		wg.Add(1)

		go func(deployment *config.Deployment) {
			defer wg.Done()

			if err := api.CopyDeployment(ctx, deployment, values); err != nil {
				setProcessedError(&processErrors, "deployment", deployment.Name, err)
			}
		}(deployment)
	}

	if values.CreateService {
		for _, service := range values.Services {
			wg.Add(1)

			go func(service *config.Service) {
				defer wg.Done()

				if err := api.CopyService(ctx, service, values); err != nil {
					setProcessedError(&processErrors, "service", service.Name, err)
				}
			}(service)
		}
	}

	for _, configMap := range values.ConfigMaps {
		wg.Add(1)

		go func(configMap *config.ConfigMap) {
			defer wg.Done()

			if err := api.CopyConfigMap(ctx, configMap, values); err != nil {
				setProcessedError(&processErrors, "configMap", configMap.Name, err)
			}
		}(configMap)
	}

	wg.Wait()

	return getProcessedErrors(&processErrors)
}

func updateServicesSelector(ctx context.Context, values *config.Type, version types.Version) error {
	var processErrors sync.Map

	var wg sync.WaitGroup

	for _, service := range values.Services {
		wg.Add(1)

		go func(service *config.Service) {
			defer wg.Done()

			if err := api.UpdateServiceSelector(ctx, service.Name, values, version); err != nil {
				setProcessedError(&processErrors, "service", service.Name, err)
			}
		}(service)
	}

	wg.Wait()

	return getProcessedErrors(&processErrors)
}

func scaleOriginalDeploymens(ctx context.Context, values *config.Type) error {
	var processErrors sync.Map

	var wg sync.WaitGroup

	for _, deployment := range values.Deployments {
		wg.Add(1)

		go func(deployment *config.Deployment) {
			defer wg.Done()

			if err := api.ScaleDeployment(ctx, deployment, 0, values); err != nil {
				setProcessedError(&processErrors, "deployment", deployment.Name, err)
			}
		}(deployment)
	}

	wg.Wait()

	return getProcessedErrors(&processErrors)
}

func setProcessedError(processErrors *sync.Map, name, key string, err error) {
	processErrors.Store(fmt.Sprintf("%s/%s", name, key), err)
}

func getProcessedErrors(processErrors *sync.Map) error {
	errorsMessage := ""

	processErrors.Range(func(key, value interface{}) bool {
		err, ok := value.(error)

		if !ok {
			log.Fatal("assertion error")
		}

		name, ok := key.(string)

		if !ok {
			log.Fatal("assertion error")
		}

		errorsMessage += name + ": " + err.Error() + "\n"

		return true
	})

	if len(errorsMessage) > 0 {
		return errors.New(errorsMessage)
	}

	return nil
}

func updateCanaryServices(ctx context.Context, values *config.Type) error {
	var processErrors sync.Map

	var wg sync.WaitGroup

	for _, service := range values.Canary.Services {
		wg.Add(1)

		go func(service *config.CanaryService) {
			defer wg.Done()

			if err := api.UpdateServiceSelector(ctx, service.Name, values, values.Version); err != nil {
				setProcessedError(&processErrors, "service", service.Name, err)
			}
		}(service)
	}

	wg.Wait()

	return getProcessedErrors(&processErrors)
}

func turnOffCanary(ctx context.Context, values *config.Type) error {
	if !values.Canary.Strategy.HasPhase1() {
		return nil
	}

	switch values.Canary.Phase1.Strategy {
	case config.CanaryPhase1CanaryStrategy:
		err := values.Canary.GetServiceMesh().SetCanaryPercent(ctx, types.CanaryProviderPercentMin)
		if err != nil {
			return errors.Wrap(err, "error setting canary percent")
		}
	case config.CanaryPhase1ABTestStrategy:
		err := canary.SetServiceABTestMode(ctx, values, false)
		if err != nil {
			return errors.Wrap(err, "error stopping ab test")
		}
	}

	return nil
}

func rollback(ctx context.Context) error {
	var processErrors sync.Map

	values := config.Get()

	var wg sync.WaitGroup

	if values.CurrentVersion.IsNotEmpty() && values.CanRollBack() {
		log.Info("rollback to previous version")

		wg.Add(1)

		go func() {
			defer wg.Done()

			err := updateServicesSelector(ctx, values, values.CurrentVersion)
			if err != nil {
				setProcessedError(&processErrors, "rollback", "failed to update services", err)
			}
		}()
	}

	if values.CurrentVersion.IsNotEmpty() && values.CanRollBack() {
		log.Info("try to delete new version")

		wg.Add(1)

		go func() {
			defer wg.Done()

			if err := api.DeleteVersion(ctx, values, api.DeleteNewVersion); err != nil {
				setProcessedError(&processErrors, "rollback", "failed to delete new versions", err)
			}
		}()
	}

	if values.HasCanary() {
		log.Info("set canary percent to min")

		wg.Add(1)

		go func() {
			defer wg.Done()

			err := turnOffCanary(ctx, values)
			if err != nil {
				setProcessedError(&processErrors, "rollback", "failed to canary", err)
			}
		}()
	}

	wg.Wait()

	return getProcessedErrors(&processErrors)
}
