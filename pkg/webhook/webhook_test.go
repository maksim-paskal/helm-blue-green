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
package webhook_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/maksim-paskal/helm-blue-green/pkg/config"
	"github.com/maksim-paskal/helm-blue-green/pkg/webhook"
)

var (
	ctx = context.Background()
	ts  = httptest.NewServer(getHandler())
)

func getHandler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/handler-001", handler001)
	mux.HandleFunc("/handler-002", handler002)

	return mux
}

func handler001(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet || r.Header.Get("test1") != "value1-v1" {
		w.WriteHeader(http.StatusBadRequest)

		return
	}

	if v := r.URL.Query().Get("version"); v != "v1" {
		w.WriteHeader(http.StatusBadRequest)

		return
	}

	defer r.Body.Close()

	body, err := io.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)

		return
	}

	if string(body) != "test-v1" {
		w.WriteHeader(http.StatusBadRequest)

		return
	}

	w.WriteHeader(http.StatusOK)
}

func handler002(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || r.Header.Get("test2") != "value2" {
		w.WriteHeader(http.StatusBadRequest)

		return
	}

	w.WriteHeader(http.StatusOK)
}

func TestHooks(t *testing.T) {
	t.Parallel()

	tests := make([]*config.WebHook, 0)

	tests = append(tests, &config.WebHook{
		URL:     ts.URL + "/handler-001?version={{ .Version }}",
		Headers: map[string]string{"test1": "value1-{{ .Version }}"},
		Body:    "test-{{ .Version }}",
	})

	tests = append(tests, &config.WebHook{
		URL:     ts.URL + "/handler-002",
		Headers: map[string]string{"test2": "value2"},
		Method:  "POST",
	})

	event := webhook.Event{
		Type:      webhook.EventTypeSuccess,
		Name:      "name",
		Namespace: "namespace",
		Version:   "v1",
	}

	for _, test := range tests {
		if err := webhook.Send(ctx, event, test); err != nil {
			t.Error(err)
		}
	}
}

func TestEventFormat(t *testing.T) {
	t.Parallel()

	event := webhook.Event{
		Type:        webhook.EventTypeSuccess,
		Name:        "1",
		Namespace:   "2",
		Environment: "3",
		Version:     "4",
		OldVersion:  "5",
		Duration:    "6",
	}

	tests := make(map[string]string, 0)
	tests["{{ .GetQueryString }}"] = "event.Type=success&event.Name=1&event.Namespace=2&event.Environment=3&event.Version=4&event.OldVersion=5&event.Duration=6" //nolint:lll
	tests["{{ .Type }}"] = "success"
	tests["{{ .Version }}"] = "4"
	tests["{{ .GetJSON }}"] = `{"Type":"success","Name":"1","Namespace":"2","Environment":"3","Version":"4","OldVersion":"5","Duration":"6"}` //nolint:lll
	tests["WW{{ .Version }}WW"] = "WW4WW"
	tests["test"] = "test"
	tests[""] = ""

	for key, value := range tests {
		if result, err := event.FormatValue(key); err != nil {
			t.Error(err)
		} else if result != value {
			t.Errorf("expected %s, got %s", value, result)
		}
	}
}
