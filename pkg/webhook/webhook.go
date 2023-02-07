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
package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"text/template"

	"github.com/maksim-paskal/helm-blue-green/pkg/config"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

type EventType string

const (
	// Processing was successful
	EventTypeSuccess EventType = "success"
	// Something went wrong during processing
	EventTypeFailed EventType = "failed"
	// Process was successful, but no changes were made
	EventTypeAlreadyDeployed EventType = "already_deployed"
)

type Event struct {
	Type        EventType
	Name        string
	Namespace   string
	Environment string
	Version     string
	OldVersion  string
	Duration    string
}

// return query string with not empty elements.
func (e *Event) GetQueryString() string {
	q := make([]string, 0)

	q = append(q, "event.Type="+string(e.Type))
	q = append(q, "event.Name="+url.QueryEscape(e.Name))
	q = append(q, "event.Namespace="+url.QueryEscape(e.Namespace))
	q = append(q, "event.Environment="+url.QueryEscape(e.Environment))
	q = append(q, "event.Version="+url.QueryEscape(e.Version))
	q = append(q, "event.OldVersion="+url.QueryEscape(e.OldVersion))
	q = append(q, "event.Duration="+url.QueryEscape(e.Duration))

	queryString := make([]string, 0)

	for _, v := range q {
		if strings.HasSuffix(v, "=") {
			continue
		}

		queryString = append(queryString, v)
	}

	return strings.Join(queryString, "&")
}

// return query string with not empty elements.
func (e *Event) GetJSON() string {
	b, err := json.Marshal(e)
	if err != nil {
		return err.Error()
	}

	return string(b)
}

func (e *Event) FormatValue(value string) (string, error) {
	tmpl, err := template.New("webhook").Parse(value)
	if err != nil {
		return "", errors.Wrap(err, "error parsing template")
	}

	var tpl bytes.Buffer

	err = tmpl.Execute(&tpl, e)
	if err != nil {
		return "", errors.Wrap(err, "error execute template")
	}

	return tpl.String(), err
}

const defaultMethod = http.MethodGet

var httpClient = http.Client{}

func Execute(ctx context.Context, event Event, values *config.Type) error {
	errs := make([]string, 0)

	for _, webhook := range values.WebHooks {
		if err := Send(ctx, event, webhook); err != nil {
			log.WithError(err).Warn("error sending webhook")
			errs = append(errs, err.Error())
		}
	}

	if len(errs) > 0 {
		return errors.New(strings.Join(errs, ","))
	}

	return nil
}

func Send(ctx context.Context, event Event, webhook *config.WebHook) error {
	var requestString []byte

	if len(webhook.Body) > 0 {
		webhookBody, err := event.FormatValue(webhook.Body)
		if err != nil {
			return errors.Wrap(err, "error formatting webhook body")
		}

		requestString = []byte(webhookBody)
	}

	requestMethod := defaultMethod

	if len(webhook.Method) > 0 {
		requestMethod = webhook.Method
	}

	webhookURL, err := event.FormatValue(webhook.URL)
	if err != nil {
		return errors.Wrap(err, "error formatting webhook URL")
	}

	log.Infof("Sending webhook: %s", webhookURL)

	request, err := http.NewRequestWithContext(ctx, requestMethod, webhookURL, bytes.NewBuffer(requestString))
	if err != nil {
		return errors.Wrap(err, "error creating request")
	}

	for key, value := range webhook.Headers {
		headerValue, err := event.FormatValue(value)
		if err != nil {
			return errors.Wrap(err, "error formatting header value")
		}

		request.Header.Set(key, headerValue)
	}

	response, err := httpClient.Do(request)
	if err != nil {
		return errors.Wrap(err, "error making request")
	}

	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return errors.Errorf("unexpected status code %d", response.StatusCode)
	}

	return nil
}
