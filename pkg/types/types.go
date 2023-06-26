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
	"errors"
	"fmt"
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
	return fmt.Sprintf("%s/version", v.Scope)
}

type CanaryProviderPercent uint8

const (
	CanaryProviderPercentMax CanaryProviderPercent = 100
	CanaryProviderPercentMin CanaryProviderPercent = 0
)

var ErrNewReleaseBadQuality = errors.New("new release failed by quality gate")

type CanaryProvidePromQLType string

const (
	CanaryProvideProQLTypeCanary CanaryProvidePromQLType = "CanaryProvideProQLTypeCanary"
	CanaryProvideProQLTypeABTest CanaryProvidePromQLType = "CanaryProvideProQLTypeABTest"
	CanaryProvideProQLTypeFull   CanaryProvidePromQLType = "CanaryProvideProQLTypeFull"
)

type CanaryProviderMetrics struct {
	TotalSamplesQLs []string
	BadSamplesQLs   []string
}

type ServiceMesh interface {
	// set canary percent
	SetCanaryPercent(context.Context, CanaryProviderPercent) error
	// set service to ABTest mode
	SetServiceABTestMode(context.Context, string, bool) error
	// get prometheus expression for analyse bad status
	GetPromQL(CanaryProvidePromQLType, int) (*CanaryProviderMetrics, error)
}

type ServiceMeshConfig struct {
	ServiceMesh string
	Config      string
	Namespace   string
}
