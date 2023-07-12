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
package envoycontrolplane_test

import (
	"testing"

	"github.com/maksim-paskal/helm-blue-green/pkg/client"
	"github.com/maksim-paskal/helm-blue-green/pkg/servicemesh/providers/envoycontrolplane"
	"github.com/maksim-paskal/helm-blue-green/pkg/types"
)

func TestGetPromQL(t *testing.T) {
	t.Parallel()

	client.FakeInit()

	serviceMesh, err := envoycontrolplane.NewServiceMesh(types.ServiceMeshConfig{
		Config: `
Clusters:
- ClusterName: test
`,
	})
	if err != nil {
		t.Fatal(err)
	}

	m, err := serviceMesh.GetPromQL(types.CanaryProviderPromQLTypeCanary, 123)
	if err != nil {
		t.Fatal(err)
	}

	if got := len(m.BadSamplesQLs); got != 1 {
		t.Fatalf("len(m.BadSamplesQLs) != 1, %d", got)
	}

	if got := m.BadSamplesQLs[0]; got != "sum(max_over_time(envoy_cluster_upstream_rq{envoy_cluster_name='test-canary',envoy_response_code!~'[1-4]..'}[123s])-min_over_time(envoy_cluster_upstream_rq{envoy_cluster_name='test-canary',envoy_response_code!~'[1-4]..'}[123s]))" { //nolint:lll
		t.Fatalf("%s", got)
	}
}
