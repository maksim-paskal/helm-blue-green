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
	"time"

	"github.com/maksim-paskal/helm-blue-green/pkg/config"
	"github.com/maksim-paskal/helm-blue-green/pkg/metrics"
	"github.com/maksim-paskal/helm-blue-green/pkg/prometheus"
	"github.com/maksim-paskal/helm-blue-green/pkg/types"
	"github.com/maksim-paskal/helm-blue-green/pkg/utils"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

// additional time for metrics wait.
const analyseMetricsWait = 5 * time.Second

type AnalyseType string

const (
	Phase1Canary AnalyseType = "Phase1Canary"
	Phase1ABTest AnalyseType = "Phase1ABTest"
	Phase2Full   AnalyseType = "Phase2Full"
)

const (
	Phase1 int = 1
	Phase2 int = 2
)

func (a AnalyseType) GetPhaseNumber() int {
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

	if !values.Prometheus.Enabled() {
		log.Warn("prometheus not enabled, skip analyse traffic")

		return nil
	}

	prometheusWaitTime := config.Get().Prometheus.GetScrapeInterval() + analyseMetricsWait

	log.Infof("Waiting %s for prometheus gather metrics...", prometheusWaitTime.String())
	utils.SleepWithContext(ctx, prometheusWaitTime)

	totalSamples := 0
	totalErrors := 0

	var promQLType types.CanaryProvidePromQLType

	switch analyseType {
	case Phase1Canary:
		promQLType = types.CanaryProvideProQLTypeCanary
	case Phase1ABTest:
		promQLType = types.CanaryProvideProQLTypeABTest
	case Phase2Full:
		promQLType = types.CanaryProvideProQLTypeFull
	}

	getPromQL, err := values.Canary.GetServiceMesh().GetPromQL(promQLType, values.Canary.QualityGate.ErrorBudgetPeriodInSeconds) //nolint:lll
	if err != nil {
		return errors.Wrap(err, "failed to get promQL")
	}

	for _, promQL := range getPromQL.TotalSamplesQLs {
		result, err := prometheus.GetMetrics(ctx, promQL)
		if err != nil {
			return errors.Wrap(err, "failed to get metrics")
		}

		if len(result) > 0 {
			totalSamples += int(result[0].Value)
		}
	}

	for _, promQL := range getPromQL.BadSamplesQLs {
		result, err := prometheus.GetMetrics(ctx, promQL)
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

	if totalErrors > values.Canary.QualityGate.ErrorBudgetCount {
		log.Errorf("Traffic is bad, errors=%d,samples=%d,allowed=%d errors over %s",
			totalErrors,
			totalSamples,
			values.Canary.QualityGate.ErrorBudgetCount,
			values.Canary.QualityGate.GetErrorBudgetPeriod().String(),
		)

		return types.ErrNewReleaseBadQuality
	}

	log.Infof("errors=%d,samples=%d,allowed=%d, Traffic is ok", totalErrors, totalSamples, values.Canary.QualityGate.ErrorBudgetCount) //nolint:lll

	return nil
}
