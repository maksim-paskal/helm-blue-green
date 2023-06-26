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
package config_test

import (
	"context"
	"flag"
	"testing"

	"github.com/maksim-paskal/helm-blue-green/pkg/config"
)

func TestConfig(t *testing.T) { //nolint:paralleltest
	ctx := context.Background()

	t.Setenv("NAMESPACE", "default")
	t.Setenv("VERSION", "test-version-1")
	t.Setenv("MIN_REPLICAS", "1")
	t.Setenv("ENVIRONMENT", "testEnv")
	t.Setenv("MIN_REPLICAS_1", "22")
	t.Setenv("MIN_REPLICAS_1111", "0")

	if err := flag.Set("config", "testdata/test_config.yaml"); err != nil {
		t.Fatal(err)
	}

	if err := config.Load(ctx); err != nil {
		t.Fatal(err)
	}

	if want := "default"; want != config.Get().Namespace {
		t.Fatalf("Namespace is not %s", want)
	}

	if want := "testName"; want != config.Get().Name {
		t.Fatalf("Name is not %s", want)
	}

	if want := "test-version-1"; want != config.Get().Version.Value {
		t.Fatalf("Version is not %s", want)
	}

	if want := "testEnv"; want != config.Get().Environment {
		t.Fatalf("Environment is not %s", want)
	}

	if want := int32(22); want != config.Get().Deployments[1].MinReplicas {
		t.Fatalf("MinReplicas for 0 is not %d", want)
	}
}

func TestHasPhases(t *testing.T) {
	t.Parallel()

	testConfig := config.Type{
		Canary: &config.Canary{
			Strategy: config.CanaryStrategyAllPhases,
		},
	}

	if !(testConfig.Canary.Strategy.HasPhase1() && testConfig.Canary.Strategy.HasPhase2()) {
		t.Fatal("CanaryStrategyAllPhases problems")
	}

	testConfig.Canary.Strategy = config.CanaryStrategyOnlyPhase1

	if !(testConfig.Canary.Strategy.HasPhase1() && !testConfig.Canary.Strategy.HasPhase2()) {
		t.Fatal("CanaryStrategyOnlyPhase1 problems")
	}

	testConfig.Canary.Strategy = config.CanaryStrategyOnlyPhase2

	if !(!testConfig.Canary.Strategy.HasPhase1() && testConfig.Canary.Strategy.HasPhase2()) {
		t.Fatal("CanaryStrategyOnlyPhase2 problems")
	}
}
