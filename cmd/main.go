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
	log.SetFormatter(&log.JSONFormatter{})

	log.Info("Starting helm-blue-green %s...", config.GetVersion())

	if err := internal.Start(); err != nil {
		log.WithError(err).Fatal()
	}
}
