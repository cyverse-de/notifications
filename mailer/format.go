package mailer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	html "html/template"
	"io"
	"io/fs"
	"net/http"
	"os"
	"strconv"
	text "text/template"
	"time"

	"github.com/mitchellh/mapstructure"
)

// htmlTemplateDir and textTemplateDir are resolved relative to the working directory, which
// the image sets to the directory the templates are copied into.
const htmlTemplateDir = "./templates/html/"
const textTemplateDir = "./templates/text/"

// DESettings holds the DE base URL and the UI path fragments used to build the links that
// appear in email bodies.
type DESettings struct {
	Base        string
	Data        string
	Analyses    string
	Teams       string
	Tools       string
	Collections string
	Apps        string
	Admin       string
	DOI         string
	VICE        string
}

// EmailRequest is an incoming request to send an email, as received over either transport.
type EmailRequest struct {
	FromAddr    string
	To          string
	Cc          []string
	Bcc         []string
	Template    string
	Subject     string
	Attachments []Attachment
	Values      json.RawMessage
}

// Templater is the subset of html/template and text/template that message formatting needs.
type Templater interface {
	Execute(io.Writer, any) error
}

// VICERequestCompleteDetails contains the request detail fields that we need to extract when a
// VICE access request is marked as complete.
type VICERequestCompleteDetails struct {
	ConcurrentJobs int64  `mapstructure:"concurrent_jobs"`
	UseCase        string `mapstructure:"intended_use"`
}

// ToolRequestDetails contains the request detail fields that we need to extract for tool
// requests.
type ToolRequestDetails struct {
	Description   string `mapstructure:"description"`
	Documentation string `mapstructure:"documentation_url"`
	Source        string `mapstructure:"source_url"`
	Name          string `mapstructure:"name"`
	TestData      string `mapstructure:"test_data_path"`
	SubmittedBy   string `mapstructure:"submitted_by"`
}

// RequestSubmittedDetails contains the request detail fields that we need to extract for
// request submissions.
type RequestSubmittedDetails struct {
	Name           string `mapstructure:"name"`
	Email          string `mapstructure:"email"`
	UseCase        string `mapstructure:"intended_use"`
	ConcurrentJobs int64  `mapstructure:"concurrent_jobs"`
}

// ExtractDetails extracts fields from a nested object in the payload.
func ExtractDetails(payload map[string]any, dest any, fieldNames ...string) error {
	for _, fieldName := range fieldNames {
		source, ok := payload[fieldName]
		if ok && source != nil {
			return mapstructure.Decode(source, dest)
		}
	}

	return fmt.Errorf("required payload field not found: %s", fieldNames)
}

// addLinks adds the DE links that every template may reference to the template payload.
func addLinks(payload map[string]any, de DESettings) {
	payload["DELink"] = de.Base
	payload["DEDataLink"] = de.Base + de.Data
	payload["DETeamsLink"] = de.Base + de.Teams
	payload["DEAdminDoiRequestLink"] = de.Base + de.Admin + de.DOI
	payload["DEToolsLink"] = de.Base + de.Tools
	payload["DECollectionsLink"] = de.Base + de.Collections
	payload["DEAppsLink"] = de.Base + de.Apps
	payload["DEAnalysesLink"] = de.Base + de.Analyses
	payload["DEPublicationRequestsLink"] = de.Base + de.Admin + de.Apps
	payload["DEPidRequestLink"] = de.Base + de.Admin + de.DOI
}

// loadTemplate finds the named template, preferring the HTML version. The returned flag
// reports whether the template that was found is HTML.
func loadTemplate(name string) (Templater, bool, error) {
	htmlPath := htmlTemplateDir + name + ".tmpl"
	textPath := textTemplateDir + name + ".tmpl"

	// Only a definitively absent template is the caller's fault; any other stat failure
	// (permissions, bad mount) is a server-side 500 so it can't be mistaken for bad input.
	if _, statErr := os.Stat(htmlPath); statErr == nil {
		tmpl, err := html.ParseFiles(htmlPath, htmlTemplateDir+"header.tmpl", htmlTemplateDir+"footer.tmpl")
		return tmpl, true, err
	} else if !errors.Is(statErr, fs.ErrNotExist) {
		return nil, false, statErr
	}

	if _, statErr := os.Stat(textPath); statErr == nil {
		tmpl, err := text.ParseFiles(textPath)
		return tmpl, false, err
	} else if !errors.Is(statErr, fs.ErrNotExist) {
		return nil, false, statErr
	}

	return nil, false, NewHTTPError(http.StatusBadRequest, "unknown template: %q", name)
}

// addTemplateSpecificValues derives the extra payload entries that individual templates need.
func addTemplateSpecificValues(templateName string, payload map[string]any, de DESettings) error {
	switch templateName {
	case "analysis_status_change", "analysis_periodic_notification":
		var startDateText, resultFolderPath, analysisID string

		if err := ExtractDetails(payload, &startDateText, "startdate"); err != nil {
			log.Errorf("unable to extract the analysis start date: %s", err)
			startDateText = ""
		}
		millisec, err := strconv.ParseInt(startDateText, 10, 64)
		if err != nil {
			log.Errorf("unable to parse the analysis start date %q: %s", startDateText, err)
		}
		payload["startdate"] = time.Unix(0, millisec*int64(time.Millisecond))

		if err := ExtractDetails(payload, &resultFolderPath, "analysisresultsfolder", "result_folder_path"); err != nil {
			log.Errorf("unable to extract the analysis result folder path: %s", err)
		}
		payload["DEOutputFolderLink"] = de.Base + de.Data + resultFolderPath

		if err := ExtractDetails(payload, &analysisID, "analysisid"); err != nil {
			log.Errorf("unable to extract the analysis ID: %s", err)
		}
		payload["DEAnalysisDetailsLink"] = de.Base + de.Analyses + "/" + analysisID

	case "added_to_team":
		var teamName string
		if err := ExtractDetails(payload, &teamName, "team_name"); err != nil {
			return NewHTTPError(http.StatusBadRequest, "unable to extract the team name: %s", err)
		}
		payload["DETeamsLink"] = de.Base + de.Teams + "/" + teamName

	case "request_complete", "request_rejected":
		// A missing request_type used to panic here, so no legitimate payload omits it;
		// silently rendering as non-VICE would send an email without the VICE details.
		requestType, ok := payload["request_type"].(string)
		if !ok {
			return NewHTTPError(http.StatusBadRequest, "missing or non-string request_type in %s payload", templateName)
		}
		if requestType == "vice" {
			var viceRequestDetails VICERequestCompleteDetails
			if err := ExtractDetails(payload, &viceRequestDetails, "request_details"); err != nil {
				return NewHTTPError(http.StatusBadRequest, "unable to extract the VICE request details: %s", err)
			}
			payload["ConcurrentJobs"] = viceRequestDetails.ConcurrentJobs
			payload["UseCase"] = viceRequestDetails.UseCase
			payload["DEAppsLink"] = de.Base + de.Apps + "?selectedFilter={\"value\":\"Interactive\",\"display\":\"VICE\"}&selectedCategory={\"name\":\"Browse All Apps\",\"id\":\"pppppppp-pppp-pppp-pppp-pppppppppppp\"}"
		}

	case "tool_request":
		var reqDetails ToolRequestDetails
		if err := ExtractDetails(payload, &reqDetails, "toolrequestdetails"); err != nil {
			return NewHTTPError(http.StatusBadRequest, "unable to extract the tool request details: %s", err)
		}
		payload["user"] = "Admin"
		payload["Description"] = reqDetails.Description
		payload["Documentation"] = reqDetails.Documentation
		payload["Source"] = reqDetails.Source
		payload["Name"] = reqDetails.Name
		payload["TestData"] = reqDetails.TestData
		payload["SubmittedBy"] = reqDetails.SubmittedBy
		payload["DEToolRequestLink"] = de.Base + de.Admin + de.Tools

	case "request_submitted":
		var reqDetails RequestSubmittedDetails
		if err := ExtractDetails(payload, &reqDetails, "request_details"); err != nil {
			return NewHTTPError(http.StatusBadRequest, "unable to extract the request details: %s", err)
		}
		payload["Name"] = reqDetails.Name
		payload["Email"] = reqDetails.Email
		payload["UseCase"] = reqDetails.UseCase
		payload["ConcurrentJobs"] = reqDetails.ConcurrentJobs
		payload["user"] = "Admin"
	}

	return nil
}

// FormatMessage renders the template named by the request against the given payload. The
// returned flag reports whether the rendered body is HTML.
func FormatMessage(ctx context.Context, emailReq EmailRequest, payload map[string]any, de DESettings) (bytes.Buffer, bool, error) {
	log := log.WithContext(ctx)
	log.Infof("received formatting request with template %s", emailReq.Template)

	var output bytes.Buffer

	addLinks(payload, de)

	tmpl, isHTML, err := loadTemplate(emailReq.Template)
	if err != nil {
		log.Error(err)
		return output, isHTML, err
	}

	if err := addTemplateSpecificValues(emailReq.Template, payload, de); err != nil {
		return output, isHTML, err
	}

	if err := tmpl.Execute(&output, payload); err != nil {
		log.Error(err)
		return output, isHTML, err
	}

	return output, isHTML, nil
}
