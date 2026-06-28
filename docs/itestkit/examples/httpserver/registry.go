package httpserverexample

import (
	"fmt"

	"github.com/n-r-w/itestkit"
	itestkithttpserver "github.com/n-r-w/itestkit/httpserver"
)

// xlsxResponseBodyType selects report summary normalization for XLSX-like responses.
const xlsxResponseBodyType = "xlsx"

// reportBodyConfig is the JSONC response_body contract used by report cases.
type reportBodyConfig struct {
	Type      string `json:"type"`
	Sheet     string `json:"sheet"`
	HeaderRow int    `json:"header_row"`
}

// reportBodyNormalizer converts binary report bytes into a stable JSON-safe summary.
type reportBodyNormalizer struct{}

// compileTimeReportBodyNormalizerCheck verifies the implementation of httpserver.BodyNormalizer.
var _ itestkithttpserver.BodyNormalizer = reportBodyNormalizer{}

// NormalizeHTTPBody handles report response bodies declared with response_body.type=xlsx.
func (reportBodyNormalizer) NormalizeHTTPBody(
	ctx itestkithttpserver.BodyNormalizationContext,
) (body any, handled bool, err error) {
	if len(ctx.Request.ResponseBody) == 0 {
		return nil, false, nil
	}

	config := reportBodyConfig{Type: "", Sheet: "", HeaderRow: 0}
	decodeErr := itestkit.DecodeStrictJSON(ctx.Request.ResponseBody, &config)
	if decodeErr != nil {
		return nil, false, fmt.Errorf("decode report response_body: %w", decodeErr)
	}
	if config.Type != xlsxResponseBodyType {
		return nil, false, nil
	}

	return map[string]any{
		"type":         config.Type,
		"sheet":        config.Sheet,
		"header_row":   float64(config.HeaderRow),
		"content_type": ctx.Response.Header.Get(contentTypeHeader),
		"byte_size":    float64(len(ctx.Body)),
	}, true, nil
}

// newRegistry registers the preset CallHTTP handler for inbound HTTP API calls.
func newRegistry() itestkit.MapRegistry[*harness] {
	return itestkithttpserver.NewRegistry[*harness](
		nil,
		itestkithttpserver.WithBaseURL("http://api.example.test"),
		itestkithttpserver.WithBodyNormalizer(reportBodyNormalizer{}),
	)
}
