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
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/maksim-paskal/helm-blue-green/internal"
	"github.com/maksim-paskal/helm-blue-green/pkg/config"
	"github.com/maksim-paskal/helm-blue-green/pkg/types"
	log "github.com/sirupsen/logrus"
)

var (
	logLevel   = flag.String("log.level", "info", "Log level")
	logJSON    = flag.Bool("log.json", false, "Logs as JSON")
	version    = flag.Bool("version", false, "Print version and exit")
	showConfig = flag.Bool("showConfig", false, "Print config and exit")
)

func main() { //nolint:cyclop
	flag.Parse()

	if *version {
		fmt.Println(config.GetVersion()) //nolint:forbidigo
		os.Exit(0)
	}

	level, err := log.ParseLevel(*logLevel)
	if err != nil {
		log.Fatalf("Error parsing log level: %s", err)
	}

	log.SetLevel(level)
	log.SetReportCaller(true)

	if _, ok := os.LookupEnv("KUBERNETES_SERVICE_HOST"); ok || *logJSON {
		log.SetFormatter(&log.JSONFormatter{})
	}

	// get background context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	signalChanInterrupt := make(chan os.Signal, 1)
	signal.Notify(signalChanInterrupt, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		select {
		case <-ctx.Done():
		case <-signalChanInterrupt:
			log.Error("Received an interrupt, stopping services...")
			cancel()
		}
	}()

	if err := config.Load(ctx); err != nil {
		log.WithError(err).Fatal()
	}

	if *showConfig {
		fmt.Println(config.Get().String()) //nolint:forbidigo
		os.Exit(0)                         //nolint:gocritic
	}

	log.Infof("Starting helm-blue-green %s...", config.GetVersion())

	log.Infof("Using config:\n%s", config.Get().String())

	if err := config.Validate(ctx); err != nil {
		log.WithError(err).Fatal()
	}

	if err := internal.Start(ctx); err != nil {
		// do not retry process if new release was failed by quality check
		if errors.Is(err, types.ErrNewReleaseBadQuality) {
			log.WithError(err).Error()
		} else {
			log.WithError(err).Fatal()
		}
	}
}
