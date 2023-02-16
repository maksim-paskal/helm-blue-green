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
package e2e_test

import (
	"context"
	"flag"
	"testing"

	"github.com/maksim-paskal/helm-blue-green/internal"
	log "github.com/sirupsen/logrus"
)

var ctx = context.Background()

func TestApplication(t *testing.T) {
	t.Parallel()

	flag.Parse()
	log.SetLevel(log.DebugLevel)

	tests := []string{
		"testdata/test1.yaml",
		"testdata/test2.yaml",
		"testdata/test3.yaml",
	}

	for _, test := range tests {
		log.Infof("Starting test %s", test)

		if err := flag.Set("config", test); err != nil {
			t.Fatal(err)
		}

		if err := internal.Start(ctx); err != nil {
			t.Fatal(err)
		}
	}
}
