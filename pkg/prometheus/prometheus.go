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
package prometheus

import (
	"context"
	"net/http"
	"os"
	"time"

	"github.com/maksim-paskal/helm-blue-green/pkg/client"
	"github.com/maksim-paskal/helm-blue-green/pkg/config"
	"github.com/maksim-paskal/helm-blue-green/pkg/utils"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/api"
	prometheusv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	promConfig "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	log "github.com/sirupsen/logrus"
	yamlv3 "gopkg.in/yaml.v3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

var promClient api.Client

const (
	prometheusConfigFileMod = 0o644
)

type prometheusStaticConfig struct {
	Targets []string `yaml:"targets"`
}

//nolint:tagliatelle
type prometheusMetricRelabelConfigs struct {
	SourceLabels []string `yaml:"source_labels"`
	Regex        string   `yaml:"regex"`
	Action       string   `yaml:"action"`
}

//nolint:tagliatelle
type prometheusScrapeConfig struct {
	JobName              string                           `yaml:"job_name"`
	MetricsPath          string                           `yaml:"metrics_path"`
	ScrapeInterval       string                           `yaml:"scrape_interval"`
	ScrapeTimeout        string                           `yaml:"scrape_timeout"`
	StaticConfigs        []prometheusStaticConfig         `yaml:"static_configs"`
	MetricRelabelConfigs []prometheusMetricRelabelConfigs `yaml:"metric_relabel_configs"`
}

//nolint:tagliatelle
type prometheusConfigType struct {
	ScrapeConfigs []prometheusScrapeConfig `yaml:"scrape_configs"`
}

func Init() error {
	prometheusConfig := api.Config{
		Address: config.Get().Prometheus.URL,
	}

	if len(config.Get().Prometheus.AuthUser) > 0 {
		prometheusConfig.RoundTripper = promConfig.NewBasicAuthRoundTripper(
			config.Get().Prometheus.AuthUser,
			promConfig.Secret(config.Get().Prometheus.AuthPassword),
			"",
			"",
			api.DefaultRoundTripper,
		)
	}

	client, err := api.NewClient(prometheusConfig)
	if err != nil {
		return errors.Wrap(err, "error creating client")
	}

	promClient = client

	return nil
}

type prometheusManagmentAction string

const (
	prometheusManagmentActionReload prometheusManagmentAction = "/-/reload"
	prometheusManagmentActionPing   prometheusManagmentAction = "/-/ping"
)

// wait for prometheus to start.
func WaitForPrometheus(ctx context.Context) {
	for {
		if ctx.Err() != nil {
			break
		}

		if err := makePrometheusAction(ctx, prometheusManagmentActionPing); err != nil {
			log.WithError(err).Warn()
		} else {
			break
		}

		log.Infof("Waiting %s for prometheus to start...", config.Get().Prometheus.GetReadyWaitStep().String())
		utils.SleepWithContext(ctx, config.Get().Prometheus.GetReadyWaitStep())
	}

	log.Info("Prometheus is ready")
}

// make management prometheus action.
func makePrometheusAction(ctx context.Context, action prometheusManagmentAction) error {
	rq, err := http.NewRequestWithContext(ctx, http.MethodPost, config.Get().Prometheus.GetEndpoint(string(action)), nil)
	if err != nil {
		return errors.Wrap(err, "error creating request")
	}

	rsp, _, err := promClient.Do(ctx, rq)
	if err != nil {
		return errors.Wrap(err, "error executing request")
	}
	defer rsp.Body.Close()

	return nil
}

func GetMetrics(ctx context.Context, query string) (model.Vector, error) {
	log.Debugf("query: %s", query)

	if !config.Get().Prometheus.Enabled() {
		log.Warn("prometheus not enabled, skip getting metric")

		return nil, nil
	}

	v1api := prometheusv1.NewAPI(promClient)

	result, warnings, err := v1api.Query(ctx, query, time.Now())
	if err != nil {
		return nil, errors.Wrap(err, "error executing query")
	}

	if len(warnings) > 0 {
		log.Warn(warnings)
	}

	v, ok := result.(model.Vector)
	if !ok {
		return nil, errors.New("assertion error")
	}

	log.Debugf("result: %v", v)

	return v, nil
}

func ScheduleCreationPrometheusConfig(ctx context.Context) {
	for {
		if ctx.Err() != nil {
			break
		}

		if err := createPrometheusConfig(ctx); err != nil {
			log.WithError(err).Error()
		}

		utils.SleepWithContext(ctx, config.Get().Prometheus.GetCreateConfigInterval())
	}
}

func getPodTargets(ctx context.Context, labelsSelector string) (map[string][]string, error) {
	targets := make(map[string][]string, 0)

	pods, err := client.Client.KubeClient().CoreV1().Pods(config.Get().Namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labelsSelector,
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to list pods")
	}

	for _, pod := range pods.Items {
		if _, ok := pod.Annotations["prometheus.io/scrape"]; !ok {
			continue
		}

		if len(pod.Status.PodIP) == 0 {
			continue
		}

		prometheusPath := "/metrics"
		prometheusPort := "80"

		if podPrometheusPath, ok := pod.Annotations["prometheus.io/path"]; ok {
			prometheusPath = podPrometheusPath
		}

		if podPrometheusPort, ok := pod.Annotations["prometheus.io/port"]; ok {
			prometheusPort = podPrometheusPort
		}

		targets[prometheusPath] = append(targets[prometheusPath], pod.Status.PodIP+":"+prometheusPort)
	}

	return targets, nil
}

func createPrometheusConfig(ctx context.Context) error { //nolint:funlen,cyclop
	targets := make(map[string][]string, 0)

	// get all pod targets
	if len(config.Get().Prometheus.PodLabelSelector) == 0 {
		podTargets, err := getPodTargets(ctx, labels.Everything().String())
		if err != nil {
			return errors.Wrap(err, "failed to get pod targets")
		}

		targets = podTargets
	}

	// get user defined pod targets
	for _, podLabelSelector := range config.Get().GetPrometheusPodLabelSelector() {
		podTargets, err := getPodTargets(ctx, podLabelSelector)
		if err != nil {
			return errors.Wrap(err, "failed to get pod targets")
		}

		for k, v := range podTargets {
			targets[k] = append(targets[k], v...)
		}
	}

	prometheusConfig := prometheusConfigType{}

	metricRelabelConfigs := []prometheusMetricRelabelConfigs{
		{
			SourceLabels: []string{"__name__"},
			Regex:        config.Get().Prometheus.AllowedMetricsRegex,
			Action:       "keep",
		},
	}

	for metricsPath, metricsTargets := range targets {
		scrapeConfig := prometheusScrapeConfig{
			JobName:        "blue_green_canary",
			MetricsPath:    metricsPath,
			ScrapeInterval: config.Get().Prometheus.GetScrapeInterval().String(),
			ScrapeTimeout:  config.Get().Prometheus.GetScrapeInterval().String(),
			StaticConfigs: []prometheusStaticConfig{
				{
					Targets: metricsTargets,
				},
			},
		}
		if len(metricRelabelConfigs[0].Regex) > 0 {
			scrapeConfig.MetricRelabelConfigs = metricRelabelConfigs
		}

		prometheusConfig.ScrapeConfigs = append(prometheusConfig.ScrapeConfigs, scrapeConfig)
	}

	prometheusConfigYaml, err := yamlv3.Marshal(prometheusConfig)
	if err != nil {
		return errors.Wrap(err, "failed to marshal prometheus config")
	}

	if err = os.WriteFile(config.Get().Prometheus.LocalConfigPath, prometheusConfigYaml, prometheusConfigFileMod); err != nil { //nolint:lll
		return errors.Wrap(err, "failed to write prometheus config")
	}

	// reload local prometheus config
	if err = makePrometheusAction(ctx, prometheusManagmentActionReload); err != nil {
		log.WithError(err).Warn("failed to reload prometheus config")
	}

	return nil
}
