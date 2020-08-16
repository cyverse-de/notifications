package main

import (
	"encoding/json"
	"os"

	"github.com/pkg/errors"
)

// SwaggerServiceInfo is a structure containing service information from a Swagger JSON document.
type SwaggerServiceInfo struct {
	Description string `json:"description"`
	Title       string `json:"title"`
	Version     string `json:"version"`
}

// PartialSwagger is a structure that can be used to read just the service information from a Swagger JSON document.
type PartialSwagger struct {
	Info *SwaggerServiceInfo `json:"info"`
}

// getSwaggerServiceInfo obtains service information from a file called swagger.json in the current working directory.
func getSwaggerServiceInfo() (*SwaggerServiceInfo, error) {
	wrapMsg := "unable to load the Swagger JSON"

	// Open the file containing the Swagger JSON.
	file, err := os.Open("swagger.json")
	if err != nil {
		return nil, errors.Wrap(err, wrapMsg)
	}

	// Parse the bits we want out of the Swagger JSON.
	decoder := json.NewDecoder(file)
	partialSwagger := &PartialSwagger{}
	err = decoder.Decode(partialSwagger)
	if err != nil {
		return nil, errors.Wrap(err, wrapMsg)
	}
	return partialSwagger.Info, nil
}
