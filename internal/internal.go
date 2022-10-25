package internal

import (
	"context"
	"sync"

	"github.com/maksim-paskal/helm-blue-green/pkg/api"
	"github.com/maksim-paskal/helm-blue-green/pkg/client"
	"github.com/maksim-paskal/helm-blue-green/pkg/config"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

func Start() error {
	ctx := context.Background()

	err := client.Init()
	if err != nil {
		return errors.Wrap(err, "error initializing client")
	}

	if err := config.Load(); err != nil {
		return errors.Wrap(err, "error loading config")
	}

	log.Debugf("Using config:\n%s", config.Get().String())

	if err := process(ctx); err != nil {
		return errors.Wrap(err, "error processing")
	}

	return nil
}

func process(ctx context.Context) error { //nolint:cyclop
	values := config.Get()

	// get current version of service selector
	currentVersion, err := api.GetCurrentVersion(ctx, values)
	if err != nil {
		return errors.Wrap(err, "error getting current version")
	}

	// stop processing if traffic was already switched to new version
	if currentVersion == values.Version.Value {
		log.Infof("Version %s is already deployed", currentVersion)

		return nil
	}

	// delete unsuccessful deployment
	if err := api.DeleteVersion(ctx, values, api.DeleteNewVersion); err != nil {
		return errors.Wrap(err, "error deleting old version")
	}

	// scale original deployments to 0
	if err := scaleOriginalDeploymens(ctx, values); err != nil {
		return errors.Wrap(err, "error updating services")
	}

	// scale original deployments to 0
	if err := scaleOriginalDeploymens(ctx, values); err != nil {
		return errors.Wrap(err, "error updating services")
	}

	// create new versions of deployments,services,configmaps
	if err := createNewVersions(ctx, values); err != nil {
		return errors.Wrap(err, "error creating new versions")
	}

	// wait for all deployments to be ready
	if err := api.WaitForPodsToBeReady(ctx, values); err != nil {
		return errors.Wrap(err, "error waiting for pods to be ready")
	}

	// update service to new version
	if err := updateServicesSelector(ctx, values); err != nil {
		return errors.Wrap(err, "error updating services")
	}

	// delete old versions
	if err := api.DeleteVersion(ctx, values, api.DeleteOldVersions); err != nil {
		return errors.Wrap(err, "error deleting old version")
	}

	// delete originals
	if values.DeleteOrigins {
		if err := api.DeleteOrigins(ctx, values); err != nil {
			return errors.Wrap(err, "error deleting origin")
		}
	}

	return nil
}

func createNewVersions(ctx context.Context, values *config.Type) error {
	var processErrors sync.Map

	var wg sync.WaitGroup

	for _, deployment := range values.Deployments {
		wg.Add(1)

		go func(deployment config.Deployment) {
			defer wg.Done()

			if err := api.CopyDeployment(ctx, deployment, values); err != nil {
				processErrors.Store("deployment/"+deployment.Name, err)
			}
		}(deployment)
	}

	if values.CreateService {
		for _, service := range values.Services {
			wg.Add(1)

			go func(service config.Service) {
				defer wg.Done()

				if err := api.CopyService(ctx, service, values); err != nil {
					processErrors.Store("service/"+service.Name, err)
				}
			}(service)
		}
	}

	for _, configMap := range values.ConfigMaps {
		wg.Add(1)

		go func(configMap config.ConfigMap) {
			defer wg.Done()

			if err := api.CopyConfigMap(ctx, configMap, values); err != nil {
				processErrors.Store("configMap/"+configMap.Name, err)
			}
		}(configMap)
	}

	wg.Wait()

	return getProcessedErrors(&processErrors)
}

func updateServicesSelector(ctx context.Context, values *config.Type) error {
	var processErrors sync.Map

	var wg sync.WaitGroup

	for _, service := range values.Services {
		wg.Add(1)

		go func(service config.Service) {
			defer wg.Done()

			if err := api.UpdateServicesSelector(ctx, service, values); err != nil {
				processErrors.Store("service/"+service.Name, err)
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

		go func(deployment config.Deployment) {
			defer wg.Done()

			if err := api.ScaleDeployment(ctx, deployment, 0, values); err != nil {
				processErrors.Store("deployment/"+deployment.Name, err)
			}
		}(deployment)
	}

	wg.Wait()

	return getProcessedErrors(&processErrors)
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
