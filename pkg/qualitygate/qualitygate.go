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
package qualitygate

import (
	"context"

	"github.com/maksim-paskal/helm-blue-green/pkg/config"
	"github.com/maksim-paskal/helm-blue-green/pkg/metrics"
	"github.com/maksim-paskal/helm-blue-green/pkg/prometheus"
	"github.com/maksim-paskal/helm-blue-green/pkg/types"
	"github.com/maksim-paskal/helm-blue-green/pkg/utils"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

type AnalyseType string

const (
	Phase1Canary AnalyseType = "Phase1Canary"
	Phase1ABTest AnalyseType = "Phase1ABTest"
	Phase2Full   AnalyseType = "Phase2Full"
)

const (
	Phase1 int8 = 1
	Phase2 int8 = 2
)

func (a AnalyseType) GetPhaseNumber() int8 {
	if a.IsPhase1() {
		return Phase1
	}

	return Phase2
}

func (a AnalyseType) IsPhase1() bool {
	return a == Phase1Canary || a == Phase1ABTest
}

func (a AnalyseType) IsPhase2() bool {
	return a == Phase2Full
}

// calculate errors while switching to canary.
func AnalyseTraffic(ctx context.Context, values *config.Type, analyseType AnalyseType) error { //nolint:cyclop,funlen
	log := log.WithFields(log.Fields{
		"phase": analyseType.GetPhaseNumber(),
	})

	prometheusWaitTime := config.Get().Prometheus.GetTotalScrapeInterval()

	log.Infof("Waiting %s for prometheus gather metrics...", prometheusWaitTime.String())
	utils.SleepWithContext(ctx, prometheusWaitTime)

	totalSamples := 0
	totalErrors := 0

	qualityGateConfig := values.Canary.QualityGate
	if analyseType.IsPhase1() {
		qualityGateConfig = values.Canary.Phase1.QualityGate.Merge(values.Canary.QualityGate)
	}

	if analyseType.IsPhase2() {
		qualityGateConfig = values.Canary.Phase2.QualityGate.Merge(values.Canary.QualityGate)
	}

	if !qualityGateConfig.HasPromQL() {
		canaryQualityGateServiceMesh, err := getPromQLFromServiceMeshProvider(values, qualityGateConfig, analyseType)
		if err != nil {
			return errors.Wrap(err, "failed to get promQL from service mesh provider")
		}

		qualityGateConfig = canaryQualityGateServiceMesh
	}

	for _, promMetric := range qualityGateConfig.TotalSamplesMetrics {
		result, err := prometheus.GetMetrics(ctx, promMetric.GetPromQL())
		if err != nil {
			return errors.Wrap(err, "failed to get metrics")
		}

		if len(result) > 0 {
			totalSamples += int(result[0].Value)
		}
	}

	for _, promMetric := range qualityGateConfig.BadSamplesMetrics {
		result, err := prometheus.GetMetrics(ctx, promMetric.GetPromQL())
		if err != nil {
			return errors.Wrap(err, "failed to get metrics")
		}

		if len(result) > 0 {
			totalErrors += int(result[0].Value)
		}
	}

	// add current metrics for budget
	metrics.SetTotal(analyseType.GetPhaseNumber(), totalSamples)
	metrics.SetBad(analyseType.GetPhaseNumber(), totalErrors)

	if totalErrors > *qualityGateConfig.ErrorBudgetCount {
		log.Errorf("Traffic is bad, errors=%d,samples=%d,allowed=%d errors over %s",
			totalErrors,
			totalSamples,
			*qualityGateConfig.ErrorBudgetCount,
			values.Canary.QualityGate.GetErrorBudgetPeriod().String(),
		)

		return types.ErrNewReleaseBadQuality
	}

	log.Infof("errors=%d,samples=%d,allowed=%d, Traffic is ok",
		totalErrors,
		totalSamples,
		*qualityGateConfig.ErrorBudgetCount,
	)

	return nil
}

// Deprecated: use phase CanaryPhaseQualityGate.
func getPromQLFromServiceMeshProvider(values *config.Type, qualityGateConfig *config.CanaryQualityGate, analyseType AnalyseType) (*config.CanaryQualityGate, error) { //nolint:lll
	log.Warn("getPromQLFromServiceMeshProvider is deprecated, use phase CanaryPhaseQualityGate")

	var promQLType types.CanaryProviderPromQLType

	switch analyseType {
	case Phase1Canary:
		promQLType = types.CanaryProviderPromQLTypeCanary
	case Phase1ABTest:
		promQLType = types.CanaryProviderPromQLTypeABTest
	case Phase2Full:
		promQLType = types.CanaryProviderPromQLTypeFull
	}

	getPromQL, err := values.Canary.GetServiceMesh().GetPromQL(
		promQLType,
		*qualityGateConfig.ErrorBudgetPeriodInSeconds,
	)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get promQL")
	}

	result := &config.CanaryQualityGate{
		ErrorBudgetCount:           qualityGateConfig.ErrorBudgetCount,
		ErrorBudgetPeriodInSeconds: qualityGateConfig.ErrorBudgetPeriodInSeconds,
		TotalSamplesMetrics:        config.GetPromQLMetricFromSlices(getPromQL.TotalSamplesQLs),
		BadSamplesMetrics:          config.GetPromQLMetricFromSlices(getPromQL.BadSamplesQLs),
	}

	return result, nil
}
