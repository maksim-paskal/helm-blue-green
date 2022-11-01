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
package types_test

import (
	"testing"

	"github.com/maksim-paskal/helm-blue-green/pkg/types"
)

func TestVersion(t *testing.T) {
	t.Parallel()

	v := types.NewVersion("scope", "value")

	if v.Scope != "scope" {
		t.Errorf("expected scope to be %s, got %s", "scope", v.Scope)
	}

	if v.Value != "value" {
		t.Errorf("expected value to be %s, got %s", "value", v.Value)
	}

	if v.String() != "scope/version=value" {
		t.Errorf("expected string to be %s, got %s", "scope/version=value", v.String())
	}
}
