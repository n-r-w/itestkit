package bookingcalendar

import (
	"embed"
	"testing"

	"github.com/n-r-w/itestkit"
	"github.com/n-r-w/itestkit/testcalendar"
	"github.com/stretchr/testify/require"
)

const casesRootDir = "cases"

//go:embed cases/*
var casesFS embed.FS

// TestBookingCalendarExample shows how calendar macros and an injected now-provider work together.
func TestBookingCalendarExample(t *testing.T) {
	t.Parallel()

	codec := bookingStatusCodec{}
	calendar := testcalendar.New()
	cases, err := itestkit.LoadCases(calendar.WrapSource(casesFS), casesRootDir, newRegistry(), codec)
	require.NoError(t, err)

	itests := harnessFactory{}
	itestkit.RunCases(
		t,
		cases,
		itests,
		itests,
		bookingErrorInspector{},
		codec,
	)
}
