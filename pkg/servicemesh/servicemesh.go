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
package servicemesh

import (
	"fmt"

	"github.com/maksim-paskal/helm-blue-green/pkg/servicemesh/providers/envoycontrolplane"
	"github.com/maksim-paskal/helm-blue-green/pkg/types"
	"github.com/pkg/errors"
)

const (
	DefaultServiceMesh           = serviceMeshEnvoyControlPlane
	serviceMeshEnvoyControlPlane = "envoy_control_plane"
)

func NewServiceMesh(config types.ServiceMeshConfig) (types.ServiceMesh, error) { //nolint:ireturn
	switch config.ServiceMesh {
	case serviceMeshEnvoyControlPlane:
		servicemesh, err := envoycontrolplane.NewServiceMesh(config)

		return servicemesh, errors.Wrapf(err, "error creating provider %s", config.ServiceMesh) //nolint:go
	default:
		return nil, fmt.Errorf("provider %s not found", config.ServiceMesh) //nolint:goerr113
	}
}
