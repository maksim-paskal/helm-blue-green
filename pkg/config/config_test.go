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
	"flag"
	"testing"

	"github.com/maksim-paskal/helm-blue-green/pkg/config"
)

func TestConfig(t *testing.T) { //nolint:paralleltest
	t.Setenv("NAMESPACE", "default")
	t.Setenv("VERSION", "test-version-1")
	t.Setenv("MIN_REPLICAS", "1")
	t.Setenv("ENVIRONMENT", "testEnv")

	if err := flag.Set("config", "testdata/test_config.yaml"); err != nil {
		t.Fatal(err)
	}

	if err := config.Load(); err != nil {
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
}
