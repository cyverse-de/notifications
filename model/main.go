package model

// RootResponse describes the response of the root endpoint.
type RootResponse struct {

	// The name of the service.
	Service string `json:"service"`

	// The service title.
	Title string `json:"title"`

	// The service version.
	Version string `json:"version"`
}

// ErrorResponse describes an error response for any endpoint.
type ErrorResponse struct {
	Message string `json:"message"`
}

// VersionRootResponse describes the response of the root enpdpoint for an API version.
type VersionRootResponse struct {

	// The name of the service.
	Service string `json:"service"`

	// The service title.
	Title string `json:"title"`

	// The service version.
	Version string `json:"version"`

	// The API version.
	APIVersion string `json:"api_version"`
}
