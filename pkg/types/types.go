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
package types

import (
	"context"
	"fmt"
	"strings"

	"github.com/pkg/errors"
)

type Version struct {
	Scope string
	Value string
}

func NewVersion(scope, value string) Version {
	return Version{
		Scope: scope,
		Value: value,
	}
}

func (v Version) IsNotEmpty() bool {
	return len(v.Value) > 0
}

func (v Version) String() string {
	return fmt.Sprintf("%s=%s", v.Key(), v.Value)
}

func (v Version) Key() string {
	return v.Scope + "/version"
}

type CanaryProviderPercent uint8

const (
	CanaryProviderPercentMax CanaryProviderPercent = 100
	CanaryProviderPercentMin CanaryProviderPercent = 0
)

func (c CanaryProviderPercent) Validate() error {
	if c >= CanaryProviderPercentMin && c <= CanaryProviderPercentMax {
		return nil
	}

	return errors.Errorf("invalid value (%d) valid %d-%d", c, CanaryProviderPercentMin, CanaryProviderPercentMax) //nolint:lll
}

var ErrNewReleaseBadQuality = errors.New("new release failed by quality gate")

type CanaryProviderPromQLType string

const (
	CanaryProviderPromQLTypeCanary CanaryProviderPromQLType = "CanaryProviderPromQLTypeCanary"
	CanaryProviderPromQLTypeABTest CanaryProviderPromQLType = "CanaryProviderPromQLTypeABTest"
	CanaryProviderPromQLTypeFull   CanaryProviderPromQLType = "CanaryProviderPromQLTypeFull"
)

func (c CanaryProviderPromQLType) HasValue() bool {
	return len(c) > 0
}

func (c CanaryProviderPromQLType) Validate() error {
	if !c.HasValue() {
		return nil
	}

	validValues := []string{
		string(CanaryProviderPromQLTypeCanary),
		string(CanaryProviderPromQLTypeABTest),
		string(CanaryProviderPromQLTypeFull),
	}

	for _, v := range validValues {
		if c == CanaryProviderPromQLType(v) {
			return nil
		}
	}

	return errors.Errorf("invalid value (%s) valid %s", string(c), strings.Join(validValues, ","))
}

type CanaryProviderMetrics struct {
	TotalSamplesQLs []string
	BadSamplesQLs   []string
}

type ServiceMesh interface {
	// set canary percent
	SetCanaryPercent(ctx context.Context, percent CanaryProviderPercent) error
	// set service to canary mode
	SetServiceCanaryMode(ctx context.Context, service string, isStart bool) error
	// set service to ABTest mode
	SetServiceABTestMode(ctx context.Context, service string, isStart bool) error
	// Deprecated: use CanaryPhaseQualityGate
	// get prometheus expression for analyse bad status
	GetPromQL(promQLType CanaryProviderPromQLType, budgetSeconds int) (*CanaryProviderMetrics, error)
}

type ServiceMeshConfig struct {
	ServiceMesh string
	Config      string
	Namespace   string
}
