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
	"os"
	"testing"

	"github.com/maksim-paskal/helm-blue-green/pkg/config"
	"github.com/maksim-paskal/helm-blue-green/pkg/metrics"
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
	if r.Method != http.MethodGet || r.Header.Get("Test1") != "value1-v1" {
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
	if r.Method != http.MethodPost || r.Header.Get("Test2") != "value2" {
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
		Method:  "GET",
		Headers: map[string]string{"Test1": "value1-{{ .Version }}"},
		Body:    "test-{{ .Version }}",
	})

	tests = append(tests, &config.WebHook{
		URL:     ts.URL + "/handler-002",
		Headers: map[string]string{"Test2": "value2"},
		Method:  "POST",
	})

	event := webhook.Event{
		Type:      webhook.EventTypeSuccess,
		Name:      "name",
		Namespace: "namespace",
		Version:   "v1",
	}

	for _, test := range tests {
		t.Run(test.URL, func(t *testing.T) {
			t.Parallel()

			if err := webhook.Send(ctx, event, test); err != nil {
				t.Error(err)
			}
		})
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
		Metadata: map[string]string{
			"testKey": "testValue",
		},
	}

	metrics.SetTotal(1, 1)
	metrics.SetBad(1, 1)

	event.Metrics = metrics.GetMetricsMap()

	tests := make(map[string]string, 0)
	tests["{{ .GetQueryString }}"] = "event.Duration=6&event.Environment=3&event.Metadata.testKey=testValue&event.Metrics.Phase1Bad=1&event.Metrics.Phase1Total=1&event.Name=1&event.Namespace=2&event.OldVersion=5&event.Type=success&event.Version=4"          //nolint:lll
	tests["{{ .Type }}"] = "success"                                                                                                                                                                                                                             //nolint:lll
	tests["{{ .Version }}"] = "4"                                                                                                                                                                                                                                //nolint:lll
	tests["{{ .GetJSON }}"] = `{"Type":"success","Name":"1","Namespace":"2","Environment":"3","Version":"4","OldVersion":"5","Duration":"6","Metrics":{"Phase1Bad":"1","Phase1Total":"1","Phase2Bad":"0","Phase2Total":"0"},"Metadata":{"testKey":"testValue"}}` //nolint:lll
	tests["WW{{ .Version }}WW"] = "WW4WW"                                                                                                                                                                                                                        //nolint:lll
	tests["test"] = "test"                                                                                                                                                                                                                                       //nolint:lll
	tests[""] = ""                                                                                                                                                                                                                                               //nolint:lll

	for key, value := range tests {
		if result, err := event.FormatValue(key); err != nil {
			t.Error(err)
		} else if result != value {
			t.Errorf("expected %s, got %s", value, result)
		}
	}
}

func TestGetQueryStringEmoji(t *testing.T) {
	t.Parallel()

	event := webhook.Event{
		Type: webhook.EventTypeSuccess,
	}

	if want := "event.Type=success\u2705"; event.GetQueryStringEmoji() != want {
		t.Errorf("expected %s, got %s", want, event.GetQueryStringEmoji())
	}

	if event.Type != webhook.EventTypeSuccess {
		t.Errorf("expected %s, got %s", webhook.EventTypeSuccess, event.Type)
	}

	event.Type = webhook.EventTypeFailed

	if want := "event.Type=failed\u274C"; event.GetQueryStringEmoji() != want {
		t.Errorf("expected %s, got %s", want, event.GetQueryStringEmoji())
	}

	if event.Type != webhook.EventTypeFailed {
		t.Errorf("expected %s, got %s", webhook.EventTypeSuccess, event.Type)
	}
}

func TestSlackPayload(t *testing.T) {
	t.Parallel()

	slackHook := os.Getenv("TEST_SLACK_HOOK")

	if slackHook == "" {
		t.Skip("no slack hook")
	}

	test := &config.WebHook{
		URL:  slackHook,
		Body: "{{ .GetSlackPayload }}",
	}

	events := make([]webhook.Event, 0)

	events = append(events, webhook.Event{
		Environment: "env1",
		Name:        "name1",
		Type:        webhook.EventTypeSuccess,
		OldVersion:  "old1",
		Version:     "new1",
	})

	events = append(events, webhook.Event{
		Environment: "env2",
		Name:        "name2",
		Type:        webhook.EventTypeFailed,
		OldVersion:  "old2",
		Version:     "new2",
	})

	for _, event := range events {
		if err := webhook.Send(ctx, event, test); err != nil {
			t.Fatal(err)
		}
	}
}
