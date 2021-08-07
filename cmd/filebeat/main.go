package main

import (
	"github.com/elastic/beats/v7/filebeat/cmd"
	inputs "github.com/elastic/beats/v7/filebeat/input/default-inputs"
	_ "github.com/shoothzj/beats_output_websocket/pkg"
	"os"
)

func main() {
	if err := cmd.Filebeat(inputs.Init, cmd.FilebeatSettings()).Execute(); err != nil {
		os.Exit(1)
	}
}
