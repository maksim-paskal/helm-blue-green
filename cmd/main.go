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
	"flag"
	"fmt"
	"os"

	"github.com/maksim-paskal/helm-blue-green/internal"
	"github.com/maksim-paskal/helm-blue-green/pkg/config"
	log "github.com/sirupsen/logrus"
)

var (
	logLevel = flag.String("log.level", "info", "Log level")
	logJSON  = flag.Bool("log.json", true, "Logs as JSON")
	version  = flag.Bool("version", false, "Print version and exit")
)

func main() {
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

	if *logJSON {
		log.SetFormatter(&log.JSONFormatter{})
	}

	log.Infof("Starting helm-blue-green %s...", config.GetVersion())

	if err := internal.Start(); err != nil {
		log.WithError(err).Fatal()
	}
}
