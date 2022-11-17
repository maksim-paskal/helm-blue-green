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
	"encoding/json"
	"flag"
	"os"
	"strconv"

	"github.com/maksim-paskal/helm-blue-green/pkg/types"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/util/yaml"
)

var configFile = flag.String("config", os.Getenv("CONFIG_PATH"), "Path to config file")

const (
	defaultPodCheckIntervalSeconds = 3
	defaultPodCheckMaxSeconds      = 600
)

var config = newConfig()

func newConfig() Type {
	return Type{
		PodCheckIntervalSeconds: defaultPodCheckIntervalSeconds,
		PodCheckMaxSeconds:      defaultPodCheckMaxSeconds,
		DeleteOrigins:           true,
		Hpa: Hpa{
			Enabled: true,
		},
		Pdb: Pdb{
			Enabled: true,
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
	Enabled      bool
	MinAvailable int
}

type Deployment struct {
	Name        string
	MinReplicas int32
	Hpa         *Hpa
	Pdb         *Pdb
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

type Type struct {
	// version spec
	Version types.Version
	// name blue green deployment, defaults to first deployment
	Name string
	// namespace of deployment
	Namespace string
	// environment of deployment
	Environment string
	// HorizontalPodAutoscaler spec
	Hpa Hpa
	// PodDisruptionBudget spec
	Pdb Pdb
	// replicas to set for new deployments
	MinReplicas int32
	// flag to create new services with new version
	CreateService bool
	// flag to delete origin deployment,configmaps after successful deployment
	DeleteOrigins bool
	// interval between pod checks
	PodCheckIntervalSeconds int
	// max time to wait for pods to be ready
	PodCheckMaxSeconds int
	// deployments spec
	Deployments []*Deployment
	// services spec
	Services []*Service
	// configmaps spec
	ConfigMaps []*ConfigMap
	// WebHook spec
	WebHooks []*WebHook
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

func Validate() error {
	if len(config.Namespace) == 0 {
		return errors.New("namespace is not set")
	}

	if len(config.Version.Value) == 0 {
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

	return nil
}

const (
	int32Base    = 10
	int32BitSize = 32
)

// rewrite config values from env.
func loadFromEnv() error { //nolint:cyclop
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

	if averageUtilization := os.Getenv("AVARAGE_UTILIZATION"); len(averageUtilization) > 0 {
		averageUtilizationInt, err := strconv.ParseInt(averageUtilization, int32Base, int32BitSize)
		if err != nil {
			return errors.Wrapf(err, "error parsing average utilization %s", averageUtilization)
		}

		config.Hpa.AverageUtilization = int32(averageUtilizationInt)
	}

	return nil
}

func Load() error {
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

	err = Validate()
	if err != nil {
		return errors.Wrap(err, "error validating config file")
	}

	return nil
}

var gitVersion = "dev"

func GetVersion() string {
	return gitVersion
}
