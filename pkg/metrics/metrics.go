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
package metrics

import (
	"fmt"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	dto "github.com/prometheus/client_model/go"
)

const namespace = "helm_blue_green"

var QualityGatePhase1Samples = promauto.NewGauge(prometheus.GaugeOpts{
	Namespace: namespace,
	Name:      "quality_gate_phase1_samples",
	Help:      "Total samples used by quality gate on phase1",
})

var QualityGatePhase1BadSamples = promauto.NewGauge(prometheus.GaugeOpts{
	Namespace: namespace,
	Name:      "quality_gate_phase1_bad_samples",
	Help:      "Total bad samples used by quality gate on phase1",
})

var QualityGatePhase2Samples = promauto.NewGauge(prometheus.GaugeOpts{
	Namespace: namespace,
	Name:      "quality_gate_phase2_samples",
	Help:      "Total samples used by quality gate on phase2",
})

var QualityGatePhase2BadSamples = promauto.NewGauge(prometheus.GaugeOpts{
	Namespace: namespace,
	Name:      "quality_gate_phase2_bad_samples",
	Help:      "Total bad samples used by quality gate on phase2",
})

func GetMetricsMap() map[string]string {
	result := make(map[string]string)

	result["Phase1Total"] = getGaugeValue(QualityGatePhase1Samples)
	result["Phase1Bad"] = getGaugeValue(QualityGatePhase1BadSamples)

	result["Phase2Total"] = getGaugeValue(QualityGatePhase2Samples)
	result["Phase2Bad"] = getGaugeValue(QualityGatePhase2BadSamples)

	return result
}

func getGaugeValue(metric prometheus.Gauge) string {
	m := &dto.Metric{}
	if err := metric.Write(m); err != nil {
		return "0"
	}

	return fmt.Sprintf("%.0f", m.Gauge.GetValue())
}

const (
	Phase1 = 1
	Phase2 = 2
)

func SetTotal(phase int, value int) {
	switch phase {
	case Phase1:
		QualityGatePhase1Samples.Set(float64(value))
	case Phase2:
		QualityGatePhase2Samples.Set(float64(value))
	}
}

func SetBad(phase int, value int) {
	switch phase {
	case Phase1:
		QualityGatePhase1BadSamples.Set(float64(value))
	case Phase2:
		QualityGatePhase2BadSamples.Set(float64(value))
	}
}
