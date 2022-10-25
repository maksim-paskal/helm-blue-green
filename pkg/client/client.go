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
package client

import (
	"flag"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

var kubeconfig = flag.String("kubeconfig", "", "absolute path to the kubeconfig file")

type client struct {
	stopCh     chan struct{}
	clientset  *kubernetes.Clientset
	restconfig *rest.Config
}

var Client *client

func Init() error {
	var err error

	Client, err = newClient()
	if err != nil {
		return errors.Wrap(err, "error creating newClient")
	}

	return nil
}

func newClient() (*client, error) {
	client := client{
		stopCh: make(chan struct{}),
	}

	var err error

	if len(*kubeconfig) > 0 {
		client.restconfig, err = clientcmd.BuildConfigFromFlags("", *kubeconfig)
		if err != nil {
			return nil, errors.Wrap(err, "error building kubeconfig")
		}
	} else {
		log.Info("No kubeconfig file use incluster")
		client.restconfig, err = rest.InClusterConfig()
		if err != nil {
			return nil, errors.Wrap(err, "error building incluster config")
		}
	}

	client.clientset, err = kubernetes.NewForConfig(client.restconfig)
	if err != nil {
		log.WithError(err).Fatal()
	}

	return &client, nil
}

func (c *client) KubeClient() *kubernetes.Clientset {
	return c.clientset
}
