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
	"github.com/maksim-paskal/helm-blue-green/pkg/types"
)

func TestConfig(t *testing.T) { //nolint:cyclop,funlen
	ctx := context.Background()

	t.Setenv("NAMESPACE", "default")
	t.Setenv("VERSION", "test-version-1")
	t.Setenv("MIN_REPLICAS", "1")
	t.Setenv("ENVIRONMENT", "testEnv")
	t.Setenv("MIN_REPLICAS_1", "22")
	t.Setenv("MAX_REPLICAS_1", "23")
	t.Setenv("MIN_REPLICAS_1111", "0")

	if err := flag.Set("config", "testdata/test_config.yaml"); err != nil {
		t.Fatal(err)
	}

	if err := config.Load(ctx); err != nil {
		t.Fatal(err)
	}

	if err := config.Validate(ctx); err != nil {
		t.Fatal(err)
	}

	if want := "testName/version"; want != config.Get().Version.Key() {
		t.Fatalf("Version is not %s, got %s", want, config.Get().Version.Key())
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

	if !(config.Get().Pdb.MinAvailable == 0 && config.Get().Pdb.MaxUnavailable == 2) {
		t.Fatal("Pdb is not 0/2")
	}

	d := config.Get().Deployments

	if len(d) != 2 {
		t.Fatal("Deployments length is not 2")
	}

	if !(d[0].Pdb.MinAvailable == 0 && d[0].Pdb.MaxUnavailable == 2) {
		t.Fatal("Pdb is not 0/2")
	}

	if !(d[1].Pdb.MinAvailable == 3 && d[1].Pdb.MaxUnavailable == 0) {
		t.Fatal("Pdb is not 3/0")
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

func TestGetPrometheusPodLabelSelector(t *testing.T) {
	t.Parallel()

	testConfig := config.Type{
		Version: &types.Version{
			Value: "test",
		},
		Prometheus: &config.Prometheus{
			PodLabelSelector: []string{
				"app.kubernetes.io/name=test1",
				"app.kubernetes.io/name=test2,test={{ .Version.Value }}",
				"app.kubernetes.io/name=test3,fakeTest={{ .FakeValue }}",
			},
		},
	}

	labels := testConfig.GetPrometheusPodLabelSelector()

	if len(labels) != len(testConfig.Prometheus.PodLabelSelector) {
		t.Fatal("result must have same length as testConfig.Prometheus.PodLabelSelector")
	}

	const fatalText = "want %s, got %s"

	if want := "app.kubernetes.io/name=test1"; want != labels[0] {
		t.Fatalf(fatalText, want, labels[0])
	}

	if want := "app.kubernetes.io/name=test2,test=test"; want != labels[1] {
		t.Fatalf(fatalText, want, labels[1])
	}

	if want := "app.kubernetes.io/name=test3,fakeTest={{ .FakeValue }}"; want != labels[2] {
		t.Fatalf(fatalText, want, labels[2])
	}
}

func pnt[T any](v T) *T {
	return &v
}

func TestPromMetric(t *testing.T) {
	t.Parallel()

	cases := make(map[config.PromQLMetric]string)

	cases[config.PromQLMetric{
		Metric:          "test1",
		BudgetInSeconds: pnt(2),
	}] = "sum(max_over_time(test1[2s])-min_over_time(test1[2s]))"

	cases[config.PromQLMetric{
		Metric:          "test2",
		BudgetInSeconds: pnt(1),
	}] = "sum(max_over_time(test2[1s])-min_over_time(test2[1s]))"

	cases[config.PromQLMetric{
		PromQL: "test2[1s]",
	}] = "test2[1s]"

	for k, v := range cases {
		if got := k.GetPromQL(); got != v {
			t.Fatalf("want %s, got %s", v, got)
		}
	}
}

func TestCanaryQualityGate(t *testing.T) {
	t.Parallel()

	defaultErrorBudgetCount := 1
	defaultErrorBudgetPeriodInSeconds := 2
	defaultPhase2ErrorBudgetCount := 3

	primary := config.CanaryQualityGate{
		ErrorBudgetCount:           &defaultErrorBudgetCount,
		ErrorBudgetPeriodInSeconds: &defaultErrorBudgetPeriodInSeconds,
	}
	phase1 := config.CanaryQualityGate{}
	phase2 := config.CanaryQualityGate{
		ErrorBudgetCount: &defaultPhase2ErrorBudgetCount,
	}

	phase1Merged := phase1.Merge(&primary)
	phase2Merged := phase2.Merge(&primary)

	if got := *phase1Merged.ErrorBudgetCount; got != defaultErrorBudgetCount {
		t.Fatalf("want %d, got %d", defaultErrorBudgetCount, got)
	}

	if got := *phase2Merged.ErrorBudgetCount; got != defaultPhase2ErrorBudgetCount {
		t.Fatalf("want %d, got %d", defaultPhase2ErrorBudgetCount, got)
	}

	if got := *phase2.ErrorBudgetCount; got != defaultPhase2ErrorBudgetCount {
		t.Fatalf("want %d, got %d", defaultPhase2ErrorBudgetCount, got)
	}

	if phase1.ErrorBudgetCount != nil {
		t.Fatal("phase1.ErrorBudgetCount must be nil")
	}

	if got := *primary.ErrorBudgetCount; got != defaultErrorBudgetCount {
		t.Fatalf("want %d, got %d", defaultErrorBudgetCount, got)
	}
}
