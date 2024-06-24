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
package template_test

import (
	"testing"

	"github.com/maksim-paskal/helm-blue-green/pkg/template"
)

func TestFormatValue(t *testing.T) {
	t.Parallel()

	type testStruct struct {
		Test1 string
		Test2 string
	}

	testValue := testStruct{
		Test1: "value1",
		Test2: "value2",
	}

	result, err := template.FormatValue("{{ .Test1 }}-{{ .Test2 }}", testValue)
	if err != nil {
		t.Error(err)
	}

	if want := "value1-value2"; result != want {
		t.Errorf("want=%s, result=%s", want, result)
	}

	_, err = template.FormatValue("{{ .FakeTest1 }}-{{ .FakeTest2 }}", testValue)
	if err == nil {
		t.Error("error expected in parsing")
	}
}
