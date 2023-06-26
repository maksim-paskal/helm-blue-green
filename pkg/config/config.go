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
	"github.com/maksim-paskal/helm-blue-green/pkg/types"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/yaml"
)

var configFile = flag.String("config", os.Getenv("CONFIG_PATH"), "Path to config file")

const (
	defaultPodCheckIntervalSeconds               = 3
	defaultPodCheckAvailableTimes                = 5
	defaultMaxProcessingTimeSeconds              = 1800
	defaultPrometheusReadyWaitStepSeconds        = 2
	defaultPrometheusScrapeIntervalSeconds       = 3
	defaultPrometheusCreateConfigIntervalSeconds = 5
	defaultPrometheusAllowedMetricsRegex         = "^(envoy_cluster_upstream_rq|envoy_cluster_canary_upstream_rq)$"

	defaultCanaryQualityGateMaxErrors = 10
	defaultCanaryQualityGatePeriod    = 60

	defaultPhase1CanaryPercentMin      = 10
	defaultPhase1CanaryPercentStep     = 10
	defaultPhase1CanaryIntervalSeconds = 30

	defaultPhase1MaxExecutionTimeSecondsSeconds = 300
	defaultPhase2MaxExecutionTimeSecondsSeconds = 300
)

var config = newConfig()

func newConfig() Type {
	defaultPhase1CanaryPercentMin := types.CanaryProviderPercent(defaultPhase1CanaryPercentMin)
	defaultPhase1CanaryPercentMax := types.CanaryProviderPercentMax
	defaultPhase1CanaryPercentStep := uint8(defaultPhase1CanaryPercentStep)
	defaultPhase1CanaryIntervalSeconds := uint16(defaultPhase1CanaryIntervalSeconds)
	defaultPhase1MaxExecutionTimeSecondsSeconds := uint16(defaultPhase1MaxExecutionTimeSecondsSeconds)
	defaultPhase2MaxExecutionTimeSecondsSeconds := uint16(defaultPhase2MaxExecutionTimeSecondsSeconds)

	return Type{
		PodCheckIntervalSeconds:  defaultPodCheckIntervalSeconds,
		PodCheckAvailableTimes:   defaultPodCheckAvailableTimes,
		MaxProcessingTimeSeconds: defaultMaxProcessingTimeSeconds,
		DeleteOrigins:            true,
		Hpa: Hpa{
			Enabled: true,
		},
		Pdb: Pdb{
			Enabled: true,
		},
		Canary: &Canary{
			Enabled:     false,
			ServiceMesh: servicemesh.DefaultServiceMesh,
			Strategy:    CanaryStrategyAllPhases,
			QualityGate: &CanaryQualityGate{
				ErrorBudgetCount:           defaultCanaryQualityGateMaxErrors,
				ErrorBudgetPeriodInSeconds: defaultCanaryQualityGatePeriod,
			},
			Phase1: &CanaryPhase1{
				Strategy:                CanaryPhase1ABTestStrategy,
				CanaryPercentMin:        &defaultPhase1CanaryPercentMin,
				CanaryPercentMax:        &defaultPhase1CanaryPercentMax,
				CanaryPercentStep:       &defaultPhase1CanaryPercentStep,
				CanaryIntervalSeconds:   &defaultPhase1CanaryIntervalSeconds,
				MaxExecutionTimeSeconds: &defaultPhase1MaxExecutionTimeSecondsSeconds,
			},
			Phase2: &CanaryPhase2{
				MaxExecutionTimeSeconds: &defaultPhase2MaxExecutionTimeSecondsSeconds,
			},
		},
		Prometheus: &Prometheus{
			AllowedMetricsRegex:         defaultPrometheusAllowedMetricsRegex,
			ReadyWaitStepSeconds:        defaultPrometheusReadyWaitStepSeconds,
			ScrapeIntervalSeconds:       defaultPrometheusScrapeIntervalSeconds,
			CreateConfigIntervalSeconds: defaultPrometheusCreateConfigIntervalSeconds,
		},
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
}

func (d *Deployment) GetMinReplicas(values *Type) int32 {
	if d.MinReplicas != 0 {
		return d.MinReplicas
	}

	return values.MinReplicas
}

func (d *Deployment) GetHpa(values *Type) Hpa {
	if d.Hpa != nil {
		return *d.Hpa
	}

	return values.Hpa
}

func (d *Deployment) GetPdb(values *Type) Pdb {
	if d.Pdb != nil {
		return *d.Pdb
	}

	return values.Pdb
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

	return fmt.Errorf("invalid canary strategy %s valid %s", string(e), strings.Join(validStrategies, ",")) //nolint:goerr113,lll
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

	return fmt.Errorf("invalid canary phase1 strategy %s valid %s", string(value), strings.Join(validCanaryPhase1Strategy, ",")) //nolint:goerr113,lll
}

const (
	CanaryPhase1CanaryStrategy CanaryPhase1Strategy = "CanaryStrategy"
	CanaryPhase1ABTestStrategy CanaryPhase1Strategy = "ABTestStrategy"
)

type CanaryPhase1 struct {
	Strategy                CanaryPhase1Strategy
	MaxExecutionTimeSeconds *uint16

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
	MaxExecutionTimeSeconds *uint16
}

func (cp2 *CanaryPhase2) GetMaxExecutionTime() time.Duration {
	return time.Duration(*cp2.MaxExecutionTimeSeconds) * time.Second
}

type CanaryQualityGate struct {
	ErrorBudgetCount           int
	ErrorBudgetPeriodInSeconds int
}

func (q *CanaryQualityGate) GetErrorBudgetPeriod() time.Duration {
	return time.Duration(q.ErrorBudgetPeriodInSeconds) * time.Second
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

	// to reduce the number of metrics in prometheus, select only pods with these labels
	// if empty, all pods will be selected in local prometheus
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

func (p *Prometheus) GetCreateConfigInterval() time.Duration {
	return time.Duration(p.CreateConfigIntervalSeconds) * time.Second
}

type Type struct {
	Canary                   *Canary
	Version                  types.Version
	CurrentVersion           types.Version
	Name                     string
	Namespace                string
	Environment              string
	Deployments              []*Deployment
	Services                 []*Service
	ConfigMaps               []*ConfigMap
	WebHooks                 []*WebHook
	Pdb                      Pdb
	PodCheckIntervalSeconds  int32
	PodCheckAvailableTimes   int32
	MaxProcessingTimeSeconds int32
	Hpa                      Hpa
	MinReplicas              int32
	CreateService            bool
	DeleteOrigins            bool
	canNotRollback           bool
	Prometheus               *Prometheus
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
		if len(deployment.Name) == 0 {
			return fmt.Errorf("deployment %d name is not set", i) //nolint:goerr113
		}
	}

	for i, service := range config.Services {
		if len(service.Name) == 0 {
			return fmt.Errorf("service %d name is not set", i) //nolint:goerr113
		}
	}

	if config.HasCanary() {
		if err := config.Canary.Strategy.Validate(); err != nil {
			return errors.Wrap(err, "invalid canary strategy")
		}

		if err := config.Canary.Phase1.Strategy.Validate(); err != nil {
			return errors.Wrap(err, "invalid phase1 strategy")
		}

		if err := config.Canary.InitServiceMesh(ctx); err != nil {
			return errors.Wrap(err, "error initializing servicemesh")
		}
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

	// load min replicas from env
	if err := loadMinReplicasFromEnv(); err != nil {
		return errors.Wrap(err, "error loading min replicas from env")
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

func loadMinReplicasFromEnv() error {
	regexpMinReplicas := regexp.MustCompile("MIN_REPLICAS_([0-9]+)=[0-9]+$")

	for _, envName := range os.Environ() {
		if regexpMinReplicas.MatchString(envName) {
			envKey := regexpMinReplicas.FindStringSubmatch(envName)[1]

			envKeyInt, err := strconv.ParseInt(envKey, int32Base, int32BitSize)
			if err != nil {
				return errors.Wrapf(err, "error parsing min replicas %s", envKey)
			}

			envValue := strings.Split(envName, "=")[1]

			envValueInt, err := strconv.ParseInt(envValue, int32Base, int32BitSize)
			if err != nil {
				return errors.Wrapf(err, "error parsing min replicas %s", envValue)
			}

			log.Debugf("%s (%d = %d)", envName, envKeyInt, envValueInt)

			if len(config.Deployments) > int(envKeyInt) {
				log.Infof("set MinReplicas for %s from %d to %d",
					config.Deployments[envKeyInt].Name,
					config.Deployments[envKeyInt].MinReplicas,
					envValueInt,
				)

				config.Deployments[envKeyInt].MinReplicas = int32(envValueInt)
			}
		}
	}

	return nil
}

func Load(ctx context.Context) error {
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

	err = Validate(ctx)
	if err != nil {
		return errors.Wrap(err, "error validating config file")
	}

	return nil
}

var gitVersion = "dev"

func GetVersion() string {
	return gitVersion
}
