package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/coalaura/minicraft/internal/berlinheightprep"
)

const (
	defaultSource = "https://gdi.berlin.de/data/bdom/atom/"
	defaultOutput = "data/berlin"
)

type options struct {
	source string
	output string
}

func main() {
	configuration := parseOptions()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	_, err := berlinheightprep.Run(ctx, berlinheightprep.Options{
		Source:  configuration.source,
		Output:  configuration.output,
		Dataset: "bDOM surface",
	})
	if err != nil {
		log.Fatal(err)
	}
}

func parseOptions() options {
	configuration := options{}

	flag.StringVar(&configuration.source, "source", defaultSource, "Berlin bDOM/DGM Atom feed, local XYZ/ZIP file, or local directory")
	flag.StringVar(&configuration.output, "output", defaultOutput, "output directory for prepared Minicraft height tiles")
	flag.Parse()

	return configuration
}
