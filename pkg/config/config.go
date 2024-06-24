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
package config

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/maksim-paskal/helm-blue-green/pkg/servicemesh"
	"github.com/maksim-paskal/helm-blue-green/pkg/template"
	"github.com/maksim-paskal/helm-blue-green/pkg/types"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/yaml"
)

var configFile = flag.String("config", os.Getenv("CONFIG_PATH"), "Path to config file")

const (
	defaultMinReplicas                           = 2
	defaultMaxReplicas                           = 4
	defaultAverageUtilization                    = 80
	defaultPodCheckIntervalSeconds               = 3
	defaultPodCheckAvailableTimes                = 5
	defaultMaxProcessingTimeSeconds              = 1800
	defaultPrometheusReadyWaitStepSeconds        = 2
	defaultPrometheusScrapeIntervalSeconds       = 3
	defaultPrometheusScrapeWaitSeconds           = 0
	defaultPrometheusCreateConfigIntervalSeconds = 5
	defaultPrometheusAllowedMetricsRegex         = "^(envoy_cluster_upstream_rq|envoy_cluster_canary_upstream_rq)$"

	defaultCanaryQualityGateMaxErrors = 10
	defaultCanaryQualityGatePeriod    = 60
	defaultBudgetInSeconds            = 60

	defaultPhase1CanaryPercentMin      = 10
	defaultPhase1CanaryPercentStep     = 5
	defaultPhase1CanaryIntervalSeconds = 3

	defaultPhase1MaxExecutionTimeSecondsSeconds = 300
	defaultPhase2MaxExecutionTimeSecondsSeconds = 300
)

var config = newConfig()

func newConfig() Type { //nolint:funlen
	defaultPhase1CanaryPercentMin := types.CanaryProviderPercent(defaultPhase1CanaryPercentMin)
	defaultPhase1CanaryPercentMax := types.CanaryProviderPercentMax
	defaultPhase1CanaryPercentStep := uint8(defaultPhase1CanaryPercentStep)
	defaultPhase1CanaryIntervalSeconds := uint16(defaultPhase1CanaryIntervalSeconds)
	defaultPhase1MaxExecutionTimeSecondsSeconds := uint16(defaultPhase1MaxExecutionTimeSecondsSeconds)
	defaultPhase2MaxExecutionTimeSecondsSeconds := uint16(defaultPhase2MaxExecutionTimeSecondsSeconds)
	defaultCanaryQualityGateMaxErrors := defaultCanaryQualityGateMaxErrors
	defaultCanaryQualityGatePeriod := defaultCanaryQualityGatePeriod

	return Type{
		Version:                  &types.Version{},
		CurrentVersion:           &types.Version{},
		PodCheckIntervalSeconds:  defaultPodCheckIntervalSeconds,
		PodCheckAvailableTimes:   defaultPodCheckAvailableTimes,
		MaxProcessingTimeSeconds: defaultMaxProcessingTimeSeconds,
		DeleteOrigins:            true,
		MinReplicas:              defaultMinReplicas,
		MaxReplicas:              defaultMaxReplicas,
		Hpa: &Hpa{
			Enabled:            true,
			MinReplicas:        defaultMinReplicas,
			MaxReplicas:        defaultMaxReplicas,
			AverageUtilization: defaultAverageUtilization,
		},
		Pdb: &Pdb{
			Enabled: true,
		},
		Canary: &Canary{
			Enabled:     false,
			ServiceMesh: servicemesh.DefaultServiceMesh,
			Strategy:    CanaryStrategyAllPhases,
			QualityGate: &CanaryQualityGate{
				ErrorBudgetCount:           &defaultCanaryQualityGateMaxErrors,
				ErrorBudgetPeriodInSeconds: &defaultCanaryQualityGatePeriod,
			},
			Phase1: &CanaryPhase1{
				Strategy:                CanaryPhase1ABTestStrategy,
				CanaryPercentMin:        &defaultPhase1CanaryPercentMin,
				CanaryPercentMax:        &defaultPhase1CanaryPercentMax,
				CanaryPercentStep:       &defaultPhase1CanaryPercentStep,
				CanaryIntervalSeconds:   &defaultPhase1CanaryIntervalSeconds,
				MaxExecutionTimeSeconds: &defaultPhase1MaxExecutionTimeSecondsSeconds,
				QualityGate: &CanaryQualityGate{
					ErrorBudgetCount:           &defaultCanaryQualityGateMaxErrors,
					ErrorBudgetPeriodInSeconds: &defaultCanaryQualityGatePeriod,
				},
			},
			Phase2: &CanaryPhase2{
				WaitErrorBudgetPeriod:   false,
				MaxExecutionTimeSeconds: &defaultPhase2MaxExecutionTimeSecondsSeconds,
				QualityGate: &CanaryQualityGate{
					ErrorBudgetCount:           &defaultCanaryQualityGateMaxErrors,
					ErrorBudgetPeriodInSeconds: &defaultCanaryQualityGatePeriod,
				},
			},
		},
		Prometheus: &Prometheus{
			AllowedMetricsRegex:         defaultPrometheusAllowedMetricsRegex,
			ReadyWaitStepSeconds:        defaultPrometheusReadyWaitStepSeconds,
			ScrapeIntervalSeconds:       defaultPrometheusScrapeIntervalSeconds,
			CreateConfigIntervalSeconds: defaultPrometheusCreateConfigIntervalSeconds,
			ScrapeWaitSeconds:           defaultPrometheusScrapeWaitSeconds,
		},
		Metadata: map[string]string{},
	}
}

type Hpa struct {
	Enabled            bool
	MinReplicas        int32
	MaxReplicas        int32
	AverageUtilization int32
}

type Pdb struct {
	Enabled        bool
	MinAvailable   int
	MaxUnavailable int
}

func (pdb *Pdb) GetMinAvailable() *intstr.IntOrString {
	value := intstr.FromInt(pdb.MinAvailable)

	return &value
}

func (pdb *Pdb) GetMaxUnavailable() *intstr.IntOrString {
	value := intstr.FromInt(pdb.MaxUnavailable)

	return &value
}

type Deployment struct {
	Hpa         *Hpa
	Pdb         *Pdb
	Name        string
	MinReplicas int32
	MaxReplicas int32
}

func (d *Deployment) SetHpa(values *Type) {
	if d.Hpa != nil {
		return
	}

	hpa := *values.Hpa

	d.Hpa = &hpa
}

func (d *Deployment) SetPdb(values *Type) {
	if d.Pdb != nil {
		return
	}

	pdb := *values.Pdb

	d.Pdb = &pdb
}

type Service struct {
	Name string
}

type ConfigMap struct { //nolint:golint,revive
	Name string
}

type WebHook struct {
	URL     string
	Headers map[string]string
	Body    string
	Method  string
}

type CanaryService struct {
	Name string
}

type CanaryStrategy string

const (
	CanaryStrategyAllPhases  CanaryStrategy = "CanaryStrategyAllPhases"
	CanaryStrategyOnlyPhase1 CanaryStrategy = "CanaryStrategyOnlyPhase1"
	CanaryStrategyOnlyPhase2 CanaryStrategy = "CanaryStrategyOnlyPhase2"
)

func (e CanaryStrategy) Validate() error {
	validStrategies := []string{
		string(CanaryStrategyAllPhases),
		string(CanaryStrategyOnlyPhase1),
		string(CanaryStrategyOnlyPhase2),
	}

	for _, v := range validStrategies {
		if e == CanaryStrategy(v) {
			return nil
		}
	}

	return errors.Errorf("invalid canary strategy %s valid %s", string(e), strings.Join(validStrategies, ","))
}

func (e CanaryStrategy) HasPhase1() bool {
	return e == CanaryStrategyAllPhases || e == CanaryStrategyOnlyPhase1
}

func (e CanaryStrategy) HasPhase2() bool {
	return e == CanaryStrategyAllPhases || e == CanaryStrategyOnlyPhase2
}

type CanaryPhase1Strategy string

func (value CanaryPhase1Strategy) Validate() error {
	validCanaryPhase1Strategy := []string{
		string(CanaryPhase1CanaryStrategy),
		string(CanaryPhase1ABTestStrategy),
	}

	for _, v := range validCanaryPhase1Strategy {
		if v == string(value) {
			return nil
		}
	}

	return errors.Errorf("invalid canary phase1 strategy %s valid %s", string(value), strings.Join(validCanaryPhase1Strategy, ",")) //nolint:lll
}

const (
	CanaryPhase1CanaryStrategy CanaryPhase1Strategy = "CanaryStrategy"
	CanaryPhase1ABTestStrategy CanaryPhase1Strategy = "ABTestStrategy"
)

type PromQLMetric struct {
	Metric          string
	BudgetInSeconds *int
	PromQL          string
}

func (pm *PromQLMetric) GetPromQL() string {
	if len(pm.PromQL) > 0 {
		return pm.PromQL
	}

	m := fmt.Sprintf("%s[%ds]", pm.Metric, *pm.BudgetInSeconds)

	return fmt.Sprintf("sum(max_over_time(%s)-min_over_time(%s))", m, m)
}

// Deprecated: use PromQLMetric instead []string.
func GetPromQLMetricFromSlices(slices []string) []*PromQLMetric {
	result := make([]*PromQLMetric, len(slices))

	for i, a := range slices {
		result[i] = &PromQLMetric{
			PromQL: a,
		}
	}

	return result
}

type CanaryPhase1 struct {
	Strategy                CanaryPhase1Strategy
	MaxExecutionTimeSeconds *uint16
	QualityGate             *CanaryQualityGate

	// CanaryPhase1CanaryStrategy
	CanaryPercentMin      *types.CanaryProviderPercent
	CanaryPercentMax      *types.CanaryProviderPercent
	CanaryPercentStep     *uint8
	CanaryIntervalSeconds *uint16
}

func (cp1 *CanaryPhase1) GetCanaryInterval() time.Duration {
	return time.Duration(*cp1.CanaryIntervalSeconds) * time.Second
}

func (cp1 *CanaryPhase1) GetMaxExecutionTime() time.Duration {
	return time.Duration(*cp1.MaxExecutionTimeSeconds) * time.Second
}

type CanaryPhase2 struct {
	QualityGate             *CanaryQualityGate
	MaxExecutionTimeSeconds *uint16
	// wait while prometheus collect new metrics
	WaitErrorBudgetPeriod bool
}

func (cp2 *CanaryPhase2) GetMaxExecutionTime() time.Duration {
	return time.Duration(*cp2.MaxExecutionTimeSeconds) * time.Second
}

type CanaryQualityGate struct {
	ErrorBudgetCount           *int
	ErrorBudgetPeriodInSeconds *int
	BadSamplesMetrics          []*PromQLMetric
	TotalSamplesMetrics        []*PromQLMetric
}

func (q *CanaryQualityGate) Clone() *CanaryQualityGate {
	result := CanaryQualityGate{}

	// copy object to avoid side effects
	m, _ := json.Marshal(q) //nolint:errchkjson
	_ = json.Unmarshal(m, &result)

	return &result
}

func (q *CanaryQualityGate) Merge(parent *CanaryQualityGate) *CanaryQualityGate {
	parentCopy := parent.Clone()

	if q == nil {
		return parentCopy
	}

	result := q.Clone()

	if result.BadSamplesMetrics == nil {
		result.BadSamplesMetrics = parentCopy.BadSamplesMetrics
	}

	if result.TotalSamplesMetrics == nil {
		result.TotalSamplesMetrics = parentCopy.TotalSamplesMetrics
	}

	if result.ErrorBudgetCount == nil {
		result.ErrorBudgetCount = parentCopy.ErrorBudgetCount
	}

	if result.ErrorBudgetPeriodInSeconds == nil {
		result.ErrorBudgetPeriodInSeconds = parentCopy.ErrorBudgetPeriodInSeconds
	}

	return result
}

func (q *CanaryQualityGate) GetErrorBudgetPeriod() time.Duration {
	return time.Duration(*q.ErrorBudgetPeriodInSeconds) * time.Second
}

func (q *CanaryQualityGate) HasPromQL() bool {
	if q == nil || q.BadSamplesMetrics == nil || q.TotalSamplesMetrics == nil {
		return false
	}

	return len(q.BadSamplesMetrics) > 0 && len(q.TotalSamplesMetrics) > 0
}

type Canary struct {
	Enabled           bool
	Strategy          CanaryStrategy
	QualityGate       *CanaryQualityGate
	Phase1            *CanaryPhase1
	Phase2            *CanaryPhase2
	ServiceMesh       string
	ServiceMeshConfig string
	serviceMesh       types.ServiceMesh
	Services          []*CanaryService
}

func (c *Canary) InitServiceMesh(_ context.Context) error {
	serviceMesh, err := servicemesh.NewServiceMesh(types.ServiceMeshConfig{
		ServiceMesh: c.ServiceMesh,
		Config:      c.ServiceMeshConfig,
		Namespace:   Get().Namespace,
	})
	if err != nil {
		return errors.Wrap(err, "error creating service mesh")
	}

	c.serviceMesh = serviceMesh

	return nil
}

func (c *Canary) GetServiceMesh() types.ServiceMesh { //nolint:ireturn
	return c.serviceMesh
}

type Prometheus struct {
	URL          string
	AuthUser     string
	AuthPassword string

	AllowedMetricsRegex string

	LocalConfigPath             string
	ReadyWaitStepSeconds        uint
	ScrapeIntervalSeconds       uint
	CreateConfigIntervalSeconds uint
	ScrapeWaitSeconds           uint

	// to reduce the number of metrics in prometheus, select only pods with these labels
	// if empty, all pods will be selected in local prometheus
	// labels can be templated with .Type struct
	PodLabelSelector []string
}

func (p *Prometheus) Enabled() bool {
	return len(p.URL) > 0
}

func (p *Prometheus) HasLocalPrometheus() bool {
	return len(p.LocalConfigPath) > 0
}

func (p *Prometheus) GetEndpoint(action string) string {
	return fmt.Sprintf("%s%s", p.URL, action)
}

func (p *Prometheus) GetReadyWaitStep() time.Duration {
	return time.Duration(p.ReadyWaitStepSeconds) * time.Second
}

func (p *Prometheus) GetScrapeInterval() time.Duration {
	return time.Duration(p.ScrapeIntervalSeconds) * time.Second
}

// Prometheus need some time to save scraped metrics, so we need to wait some time before we can get metrics.
func (p *Prometheus) GetTotalScrapeInterval() time.Duration {
	return time.Duration(p.ScrapeIntervalSeconds+p.ScrapeWaitSeconds) * time.Second
}

func (p *Prometheus) GetCreateConfigInterval() time.Duration {
	return time.Duration(p.CreateConfigIntervalSeconds) * time.Second
}

type Type struct {
	Deployments              []*Deployment
	Services                 []*Service
	ConfigMaps               []*ConfigMap
	WebHooks                 []*WebHook
	Name                     string
	Namespace                string
	Environment              string
	Canary                   *Canary
	Version                  *types.Version
	CurrentVersion           *types.Version
	Pdb                      *Pdb
	Hpa                      *Hpa
	Prometheus               *Prometheus
	PodCheckIntervalSeconds  int32
	PodCheckAvailableTimes   int32
	MaxProcessingTimeSeconds int32
	MinReplicas              int32
	MaxReplicas              int32
	CreateService            bool
	DeleteOrigins            bool
	canNotRollback           bool
	Metadata                 map[string]string
}

func (t *Type) Normalize() {
	setBudgetInSeconds := func(a []*PromQLMetric) {
		localDefaultBudgetInSeconds := defaultBudgetInSeconds

		for _, q := range a {
			if q.BudgetInSeconds == nil {
				q.BudgetInSeconds = &localDefaultBudgetInSeconds
			}
		}
	}

	setBudgetInSeconds(t.Canary.QualityGate.BadSamplesMetrics)
	setBudgetInSeconds(t.Canary.QualityGate.TotalSamplesMetrics)

	t.Canary.Phase1.QualityGate = t.Canary.Phase1.QualityGate.Merge(t.Canary.QualityGate)
	t.Canary.Phase2.QualityGate = t.Canary.Phase2.QualityGate.Merge(t.Canary.QualityGate)

	setBudgetInSeconds(t.Canary.Phase1.QualityGate.BadSamplesMetrics)
	setBudgetInSeconds(t.Canary.Phase1.QualityGate.TotalSamplesMetrics)
	setBudgetInSeconds(t.Canary.Phase2.QualityGate.BadSamplesMetrics)
	setBudgetInSeconds(t.Canary.Phase2.QualityGate.TotalSamplesMetrics)

	for _, deployment := range t.Deployments {
		if deployment.MinReplicas == 0 {
			deployment.MinReplicas = t.MinReplicas
		}

		if deployment.MaxReplicas == 0 {
			deployment.MaxReplicas = t.MaxReplicas
		}

		if deployment.Hpa != nil {
			if deployment.Hpa.AverageUtilization == 0 {
				deployment.Hpa.AverageUtilization = defaultAverageUtilization
			}
		}
	}
}

func (t *Type) GetPodCheckInterval() time.Duration {
	return time.Duration(t.PodCheckIntervalSeconds) * time.Second
}

func (t *Type) GetMaxProcessingTime() time.Duration {
	return time.Duration(t.MaxProcessingTimeSeconds) * time.Second
}

func (t *Type) HasCanary() bool {
	return t.Canary != nil && t.Canary.Enabled
}

func (t *Type) CanRollBack() bool {
	return !t.canNotRollback
}

// after that time rollback will not available.
func (t *Type) SetCanNotRollback() {
	t.canNotRollback = true
}

func (t *Type) GetPrometheusPodLabelSelector() []string {
	result := make([]string, len(t.Prometheus.PodLabelSelector))

	for i, value := range t.Prometheus.PodLabelSelector {
		formatedValue, err := template.FormatValue(value, t)
		if err != nil {
			log.Errorf("error formatting prometheus pod label selector %s: %v", value, err)

			result[i] = value
		} else {
			result[i] = formatedValue
		}
	}

	return result
}

func (t *Type) String() string {
	b, err := json.Marshal(t)
	if err != nil {
		panic(err)
	}

	return string(b)
}

func Get() *Type {
	return &config
}

func Validate(ctx context.Context) error { //nolint:cyclop
	if len(config.Namespace) == 0 {
		return errors.New("namespace is not set")
	}

	if !config.Version.IsNotEmpty() {
		return errors.New("version is not set")
	}

	if len(config.Deployments) == 0 {
		return errors.New("no deployments specified")
	}

	if len(config.Services) == 0 {
		return errors.New("no services specified")
	}

	if config.MinReplicas == 0 {
		return errors.New("no deployments min replicas specified")
	}

	for i, deployment := range config.Deployments {
		if deployment.MinReplicas > deployment.MaxReplicas {
			return errors.Errorf("minReplicas (%d) bigger than maxReplicas (%d) in %d deployment",
				deployment.MinReplicas,
				deployment.MaxReplicas,
				i,
			)
		}

		if deployment.Hpa.MinReplicas > deployment.Hpa.MaxReplicas {
			return errors.Errorf("minReplicas (%d) bigger than maxReplicas (%d) in (HPA section) %d deployment",
				deployment.Hpa.MinReplicas,
				deployment.Hpa.MaxReplicas,
				i,
			)
		}

		if deployment.Pdb.MinAvailable > 0 && deployment.Pdb.MaxUnavailable > 0 {
			return errors.Errorf("only one need MinAvailable or MaxUnavailable in (PDB section) %d deployment", i)
		}

		if len(deployment.Name) == 0 {
			return errors.Errorf("deployment %d name is not set", i)
		}
	}

	for i, service := range config.Services {
		if len(service.Name) == 0 {
			return errors.Errorf("service %d name is not set", i)
		}
	}

	if err := validateCanaryConfigs(ctx); err != nil {
		return errors.Wrap(err, "invalid canary config")
	}

	return nil
}

func validateCanaryConfigs(ctx context.Context) error {
	if !config.HasCanary() {
		return nil
	}

	if err := config.Canary.Strategy.Validate(); err != nil {
		return errors.Wrap(err, "invalid canary strategy")
	}

	if err := config.Canary.Phase1.Strategy.Validate(); err != nil {
		return errors.Wrap(err, "invalid phase1 strategy")
	}

	if err := config.Canary.InitServiceMesh(ctx); err != nil {
		return errors.Wrap(err, "error initializing servicemesh")
	}

	if err := config.Canary.Phase1.CanaryPercentMin.Validate(); err != nil {
		return errors.Wrap(err, "invalid phase1 CanaryPercentMin")
	}

	if err := config.Canary.Phase1.CanaryPercentMax.Validate(); err != nil {
		return errors.Wrap(err, "invalid phase1 CanaryPercentMax")
	}

	return nil
}

const (
	int32Base    = 10
	int32BitSize = 32
)

// rewrite config values from env.
func loadFromEnv() error { //nolint:cyclop,funlen
	if namespace := os.Getenv("NAMESPACE"); len(namespace) > 0 {
		config.Namespace = namespace
	}

	if environment := os.Getenv("ENVIRONMENT"); len(environment) > 0 {
		config.Environment = environment
	}

	if version := os.Getenv("VERSION"); len(version) > 0 {
		config.Version.Value = version
	}

	if minReplicas := os.Getenv("MIN_REPLICAS"); len(minReplicas) > 0 {
		minReplicasInt, err := strconv.ParseInt(minReplicas, int32Base, int32BitSize)
		if err != nil {
			return errors.Wrapf(err, "error parsing min replicas %s", minReplicas)
		}

		config.MinReplicas = int32(minReplicasInt)
		config.Hpa.MinReplicas = int32(minReplicasInt)
	}

	if maxReplicas := os.Getenv("MAX_REPLICAS"); len(maxReplicas) > 0 {
		maxReplicasInt, err := strconv.ParseInt(maxReplicas, int32Base, int32BitSize)
		if err != nil {
			return errors.Wrapf(err, "error parsing max replicas %s", maxReplicas)
		}

		config.Hpa.MaxReplicas = int32(maxReplicasInt)
		config.MaxReplicas = int32(maxReplicasInt)
	}

	if minAvailable := os.Getenv("MIN_AVAILABLE"); len(minAvailable) > 0 {
		minAvailableInt, err := strconv.Atoi(minAvailable)
		if err != nil {
			return errors.Wrapf(err, "error parsing min available %s", minAvailable)
		}

		config.Pdb.MinAvailable = minAvailableInt
	}

	if maxUnavailable := os.Getenv("MAX_UNAVAILABLE"); len(maxUnavailable) > 0 {
		maxUnavailableInt, err := strconv.Atoi(maxUnavailable)
		if err != nil {
			return errors.Wrapf(err, "error parsing max unavailable %s", maxUnavailable)
		}

		config.Pdb.MaxUnavailable = maxUnavailableInt
	}

	if averageUtilization := os.Getenv("AVARAGE_UTILIZATION"); len(averageUtilization) > 0 {
		averageUtilizationInt, err := strconv.ParseInt(averageUtilization, int32Base, int32BitSize)
		if err != nil {
			return errors.Wrapf(err, "error parsing average utilization %s", averageUtilization)
		}

		config.Hpa.AverageUtilization = int32(averageUtilizationInt)
	}

	if canaryEnabled := os.Getenv("CANARY_ENABLED"); canaryEnabled == "true" {
		config.Canary.Enabled = true
	}

	if localConfigPath := os.Getenv("PROMETHEUS_CONFIG_PATH"); len(localConfigPath) > 0 {
		config.Prometheus.LocalConfigPath = localConfigPath
	}

	if prometheusURL := os.Getenv("PROMETHEUS_URL"); len(prometheusURL) > 0 {
		config.Prometheus.URL = prometheusURL
	}

	return nil
}

func loadDeploymentParamsFromEnv() error {
	regexpMinReplicas := regexp.MustCompile("(MIN|MAX)_REPLICAS_([0-9]+)=([0-9]+)$")

	for _, envName := range os.Environ() {
		if !regexpMinReplicas.MatchString(envName) {
			continue
		}

		envAction := regexpMinReplicas.FindStringSubmatch(envName)[1]
		envKey := regexpMinReplicas.FindStringSubmatch(envName)[2]
		envValue := regexpMinReplicas.FindStringSubmatch(envName)[3]

		envKeyInt, err := strconv.ParseInt(envKey, int32Base, int32BitSize)
		if err != nil {
			return errors.Wrapf(err, "error parsing min replicas %s", envKey)
		}

		envValueInt, err := strconv.ParseInt(envValue, int32Base, int32BitSize)
		if err != nil {
			return errors.Wrapf(err, "error parsing min replicas %s", envValue)
		}

		log.Debugf("%s (%d = %d)", envName, envKeyInt, envValueInt)

		if len(config.Deployments) > int(envKeyInt) {
			log.Debugf("set MinReplicas for %s from %d to %d",
				config.Deployments[envKeyInt].Name,
				config.Deployments[envKeyInt].MinReplicas,
				envValueInt,
			)

			if envAction == "MIN" {
				config.Deployments[envKeyInt].Hpa.MinReplicas = int32(envValueInt)
				config.Deployments[envKeyInt].MinReplicas = int32(envValueInt)
			} else if envAction == "MAX" {
				config.Deployments[envKeyInt].Hpa.MaxReplicas = int32(envValueInt)
				config.Deployments[envKeyInt].MaxReplicas = int32(envValueInt)
			}
		}
	}

	return nil
}

func Load(_ context.Context) error { //nolint:cyclop
	configBytes, err := os.ReadFile(*configFile)
	if err != nil {
		return errors.Wrap(err, "error reading config file")
	}

	// clear old values if config can load multiple times
	config = newConfig()

	err = yaml.Unmarshal(configBytes, &config)
	if err != nil {
		log.Debugf("config: %s", string(configBytes))

		return errors.Wrap(err, "error unmarshaling config file")
	}

	if len(config.Version.Scope) == 0 && len(config.Deployments) > 0 {
		config.Version.Scope = config.Deployments[0].Name
	}

	if len(config.Name) == 0 && len(config.Deployments) > 0 {
		config.Name = config.Deployments[0].Name
	}

	if err := loadFromEnv(); err != nil {
		return errors.Wrap(err, "error loading from env")
	}

	for _, deployment := range config.Deployments {
		if deployment.MinReplicas == 0 {
			deployment.MinReplicas = config.MinReplicas
		}

		if deployment.MaxReplicas == 0 {
			deployment.MaxReplicas = config.MaxReplicas
		}

		deployment.SetHpa(&config)
		deployment.SetPdb(&config)
	}

	// set default values
	config.Normalize()

	// load min/max replicas from env
	if err := loadDeploymentParamsFromEnv(); err != nil {
		return errors.Wrap(err, "error loading min replicas from env")
	}

	return nil
}

var gitVersion = "dev"

func GetVersion() string {
	return gitVersion
}
