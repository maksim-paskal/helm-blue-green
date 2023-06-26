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
package canary

import (
	"context"
	"time"

	"github.com/maksim-paskal/helm-blue-green/pkg/config"
	"github.com/maksim-paskal/helm-blue-green/pkg/qualitygate"
	"github.com/maksim-paskal/helm-blue-green/pkg/types"
	"github.com/maksim-paskal/helm-blue-green/pkg/utils"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

const defaultSleepTime = time.Second

func StartCanaryPhase1(ctx context.Context, values *config.Type) error {
	defer utils.TimeTrack("canary.StartCanaryPhase1", time.Now())

	log.Infof("Stage max execution time %s ...", values.Canary.Phase1.GetMaxExecutionTime().String())

	switch values.Canary.Phase1.Strategy {
	case config.CanaryPhase1CanaryStrategy:
		return startCanaryPhase1CanaryStrategy(ctx, values)
	case config.CanaryPhase1ABTestStrategy:
		return startCanaryPhase1ABTestStrategy(ctx, values)
	}

	return nil
}

// increament canary percent by step.
func startCanaryPhase1CanaryStrategy(ctx context.Context, values *config.Type) error {
	executionTime := time.Now()

	canaryPercentMax := *values.Canary.Phase1.CanaryPercentMax

	percent := *values.Canary.Phase1.CanaryPercentMin

	for {
		if ctx.Err() != nil {
			return errors.Wrap(ctx.Err(), "context error")
		}

		log.Infof("Set canary traffic to %d/%d...", percent, canaryPercentMax)

		err := values.Canary.GetServiceMesh().SetCanaryPercent(ctx, percent)
		if err != nil {
			return errors.Wrap(err, "failed to set canary percent")
		}

		if err = qualitygate.AnalyseTraffic(ctx, values, qualitygate.Phase1Canary); err != nil {
			return errors.Wrap(err, "failed to analyse traffic")
		}

		if percent == canaryPercentMax {
			break
		}

		if time.Since(executionTime) >= values.Canary.Phase1.GetMaxExecutionTime() {
			break
		}

		percent += types.CanaryProviderPercent(*values.Canary.Phase1.CanaryPercentStep)

		if percent > canaryPercentMax {
			percent = canaryPercentMax
		}

		log.Infof("Waiting %s for next step...", values.Canary.Phase1.GetCanaryInterval().String())
		utils.SleepWithContext(ctx, values.Canary.Phase1.GetCanaryInterval())
	}

	return nil
}

func SetServiceABTestMode(ctx context.Context, values *config.Type, isStarted bool) error {
	if !(values.Canary.Strategy.HasPhase1() && values.Canary.Phase1.Strategy == config.CanaryPhase1ABTestStrategy) {
		return nil
	}

	for _, service := range values.Canary.Services {
		if err := values.Canary.GetServiceMesh().SetServiceABTestMode(ctx, service.Name, isStarted); err != nil {
			return errors.Wrapf(err, "failed to set AB test mode for service %s", service.Name)
		}
	}

	return nil
}

// use AB test strategy.
func startCanaryPhase1ABTestStrategy(ctx context.Context, values *config.Type) error {
	executionTime := time.Now()

	if err := SetServiceABTestMode(ctx, values, true); err != nil {
		return errors.Wrap(err, "failed to set AB test mode")
	}

	for {
		if ctx.Err() != nil {
			return errors.Wrap(ctx.Err(), "context error")
		}

		if err := qualitygate.AnalyseTraffic(ctx, values, qualitygate.Phase1ABTest); err != nil {
			return errors.Wrap(err, "failed to analyse traffic")
		}

		if time.Since(executionTime) >= values.Canary.Phase1.GetMaxExecutionTime() {
			break
		}

		utils.SleepWithContext(ctx, defaultSleepTime)
	}

	if err := SetServiceABTestMode(ctx, values, false); err != nil {
		return errors.Wrap(err, "failed to set AB test mode")
	}

	return nil
}

func StartCanaryPhase2(ctx context.Context, values *config.Type) error {
	defer utils.TimeTrack("canary.StartCanaryPhase2", time.Now())

	// wait some time to gather metrics from new version
	log.Infof("Waiting %s to gather metrics from new version...", values.Canary.QualityGate.GetErrorBudgetPeriod().String()) //nolint:lll
	utils.SleepWithContext(ctx, values.Canary.QualityGate.GetErrorBudgetPeriod())

	log.Infof("Stage max execution time %s ...", values.Canary.Phase2.GetMaxExecutionTime().String())

	executionTime := time.Now()

	for {
		if ctx.Err() != nil {
			return errors.Wrap(ctx.Err(), "context error")
		}

		if err := qualitygate.AnalyseTraffic(ctx, values, qualitygate.Phase2Full); err != nil {
			return errors.Wrap(err, "failed to analyse traffic")
		}

		if time.Since(executionTime) >= values.Canary.Phase2.GetMaxExecutionTime() {
			break
		}

		utils.SleepWithContext(ctx, defaultSleepTime)
	}

	return nil
}
