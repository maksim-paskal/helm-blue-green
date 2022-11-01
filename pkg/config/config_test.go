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

	if err := flag.Set("config", "testdata/test_config.yaml"); err != nil {
		t.Fatal(err)
	}

	if err := config.Load(); err != nil {
		t.Fatal(err)
	}
}
