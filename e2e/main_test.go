package e2e_test

import (
	"flag"
	"testing"

	"github.com/maksim-paskal/helm-blue-green/internal"
	log "github.com/sirupsen/logrus"
)

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

		if err := internal.Start(); err != nil {
			t.Fatal(err)
		}
	}
}
