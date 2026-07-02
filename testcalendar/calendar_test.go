package testcalendar

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTransformJSONC_RewritesDateMacros tests the replacement of calendar dates in regular string fields.
func TestTransformJSONC_RewritesDateMacros(t *testing.T) {
	t.Parallel()

	calendar := New()
	content := []byte(`{
  "date": "<test_date+9d>",
  "checkInDate": "<test_date+9d>",
  "checkOutDate": "<test_date+10d>"
}`)

	transformed, err := calendar.transformJSONC(content)
	require.NoError(t, err)

	assert.JSONEq(t, `{
  "date": "2026-03-10",
  "checkInDate": "2026-03-10",
  "checkOutDate": "2026-03-11"
}`, string(transformed))
}

// TestTransformJSONC_RewritesDateMacrosWithExplicitTimezone checks that the explicit zone offset affects the selected calendar day.
func TestTransformJSONC_RewritesDateMacrosWithExplicitTimezone(t *testing.T) {
	t.Parallel()

	calendar := New()
	content := []byte(`{
  "dateInUTCMinusFive": "<test_date@-05:00>",
  "dateWithRelativeShift": "<test_date+1d@-05:00>"
}`)

	transformed, err := calendar.transformJSONC(content)
	require.NoError(t, err)

	assert.JSONEq(t, `{
  "dateInUTCMinusFive": "2026-02-28",
  "dateWithRelativeShift": "2026-03-01"
}`, string(transformed))
}

// TestTransformJSONC_RewritesMongoDateMacros tests RFC3339 macro rendering inside $date.
func TestTransformJSONC_RewritesMongoDateMacros(t *testing.T) {
	t.Parallel()

	calendar := New()
	content := []byte(`{
  "expected": {
    "date": {
      "$date": "<test_date+9d>"
    }
  }
}`)

	transformed, err := calendar.transformJSONC(content)
	require.NoError(t, err)

	assert.JSONEq(t, `{
  "expected": {
    "date": {
      "$date": "2026-03-10T00:00:00Z"
    }
  }
}`, string(transformed))
}

// TestTransformJSONC_RewritesMongoDateMacrosWithExplicitTimezone checks that the zone affects the selected day before converting to UTC midnight.
func TestTransformJSONC_RewritesMongoDateMacrosWithExplicitTimezone(t *testing.T) {
	t.Parallel()

	calendar := New()
	content := []byte(`{
	"expected": {
		"date": {
			"$date": "<test_date@-05:00>"
		}
	}
}`)

	transformed, err := calendar.transformJSONC(content)
	require.NoError(t, err)

	assert.JSONEq(t, `{
	"expected": {
		"date": {
			"$date": "2026-02-28T00:00:00Z"
		}
	}
}`, string(transformed))
}

// TestTransformJSONC_RewritesTimestampMacros checks the substitution of timestamp macros for response fields.
func TestTransformJSONC_RewritesTimestampMacros(t *testing.T) {
	t.Parallel()

	calendar := New()
	content := []byte(`{
  "penalty_begins_at": "<test_timestamp+7d20h30m>"
}`)

	transformed, err := calendar.transformJSONC(content)
	require.NoError(t, err)

	assert.JSONEq(t, `{
  "penalty_begins_at": "2026-03-08 23:30:00+06:00"
}`, string(transformed))
}

// TestTransformJSONC_RewritesTimestampMacrosWithExplicitTimezone tests the rendering of timestamp macros in an explicitly specified zone.
func TestTransformJSONC_RewritesTimestampMacrosWithExplicitTimezone(t *testing.T) {
	t.Parallel()

	calendar := New()
	content := []byte(`{
  "penalty_begins_at": "<test_timestamp+7d20h30m@+03:00>"
}`)

	transformed, err := calendar.transformJSONC(content)
	require.NoError(t, err)

	assert.JSONEq(t, `{
  "penalty_begins_at": "2026-03-08 20:30:00+03:00"
}`, string(transformed))
}

// TestTransformJSONC_RewritesRFC3339TimestampMacros checks RFC3339 timestamp rendering for API fields such as HTTP query parameters.
func TestTransformJSONC_RewritesRFC3339TimestampMacros(t *testing.T) {
	t.Parallel()

	calendar := New()
	content := []byte(`{
  "query": {
    "from": "<test_rfc3339_timestamp+92d3h>",
    "to": "<test_rfc3339_timestamp+122d3h>"
  }
}`)

	transformed, err := calendar.transformJSONC(content)
	require.NoError(t, err)

	assert.JSONEq(t, `{
  "query": {
    "from": "2026-06-01T06:00:00+06:00",
    "to": "2026-07-01T06:00:00+06:00"
  }
}`, string(transformed))
}

// TestTransformJSONC_RewritesRFC3339TimestampMacrosWithExplicitTimezone checks that explicit zones are preserved in RFC3339 output.
func TestTransformJSONC_RewritesRFC3339TimestampMacrosWithExplicitTimezone(t *testing.T) {
	t.Parallel()

	calendar := New()
	content := []byte(`{
  "from": "<test_rfc3339_timestamp+7d20h30m@+03:00>"
}`)

	transformed, err := calendar.transformJSONC(content)
	require.NoError(t, err)

	assert.JSONEq(t, `{
  "from": "2026-03-08T20:30:00+03:00"
}`, string(transformed))
}

// TestTransformJSONC_RewritesMacrosInsideArrays tests macro substitution in arrays without object-key context.
func TestTransformJSONC_RewritesMacrosInsideArrays(t *testing.T) {
	t.Parallel()

	calendar := New()
	content := []byte(`{
  "dates": ["<test_date+9d>", "<test_timestamp+7d21h>"]
}`)

	transformed, err := calendar.transformJSONC(content)
	require.NoError(t, err)

	assert.JSONEq(t, `{
  "dates": ["2026-03-10", "2026-03-09 00:00:00+06:00"]
}`, string(transformed))
}

// TestTransformJSONC_KeepsCommentsUntouched checks that comments are not involved in macro substitution.
func TestTransformJSONC_IgnoresCommentContent(t *testing.T) {
	t.Parallel()

	calendar := New()
	content := []byte(`{
  // comment with <test_unknown> should not break macro resolution
  "date": "<test_date+9d>"
}`)

	transformed, err := calendar.transformJSONC(content)
	require.NoError(t, err)

	assert.JSONEq(t, `{"date": "2026-03-10"}`, string(transformed))
}

// TestTransformJSONC_ProducesStandardJSON checks that the macro resolution output is compatible with the strict JSON decoder.
func TestTransformJSONC_ProducesStandardJSON(t *testing.T) {
	t.Parallel()

	calendar := New()
	content := []byte(`{
  "date": "<test_date+9d>",
}`)

	transformed, err := calendar.transformJSONC(content)
	require.NoError(t, err)
	assert.True(t, json.Valid(transformed))
	assert.JSONEq(t, `{"date": "2026-03-10"}`, string(transformed))
}

// TestTransformJSONC_RejectsMalformedCalendarPlaceholder checks that the unclosed <test_... macro does not go further as a regular string.
func TestTransformJSONC_RejectsMalformedCalendarPlaceholder(t *testing.T) {
	t.Parallel()

	calendar := New()
	_, err := calendar.transformJSONC([]byte(`{"date": "<test_date+9d"}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid calendar macro")
}

// TestResolveStringValue_RejectsTimestampInsideMongoDate checks for an explicit error for an unsupported context.
func TestResolveStringValue_RejectsTimestampInsideMongoDate(t *testing.T) {
	t.Parallel()

	calendar := New()
	_, _, err := calendar.resolveStringValue("<test_timestamp>", "$date")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "is not supported inside $date")
}

// TestResolveStringValue_RejectsRFC3339TimestampInsideMongoDate keeps $date reserved for day-based macros.
func TestResolveStringValue_RejectsRFC3339TimestampInsideMongoDate(t *testing.T) {
	t.Parallel()

	calendar := New()
	_, _, err := calendar.resolveStringValue("<test_rfc3339_timestamp>", "$date")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "is not supported inside $date")
}

// TestResolveStringValue_RejectsUnsupportedCalendarMacro checks for an apparent error for the unknown test_* macro.
func TestResolveStringValue_RejectsUnsupportedCalendarMacro(t *testing.T) {
	t.Parallel()

	calendar := New()
	_, _, err := calendar.resolveStringValue("<test_unknown>", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported calendar macro")
}

// TestResolveStringValue_RejectsInvalidTimezoneOffset checks for an apparent error for an unsupported zone format.
func TestResolveStringValue_RejectsInvalidTimezoneOffset(t *testing.T) {
	t.Parallel()

	calendar := New()
	_, _, err := calendar.resolveStringValue("<test_timestamp@Europe/Moscow>", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid timezone offset")
}

// TestResolveStringValue_RejectsDuplicateTimezoneSeparator checks that duplicate zone separator is considered an error.
func TestResolveStringValue_RejectsDuplicateTimezoneSeparator(t *testing.T) {
	t.Parallel()

	calendar := New()
	_, _, err := calendar.resolveStringValue("<test_timestamp@+03:00@+04:00>", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid timezone separator usage")
}

// TestResolveStringValue_RejectsTimeUnitsForDateMacro checks that the date macro does not accept hours and minutes.
func TestResolveStringValue_RejectsTimeUnitsForDateMacro(t *testing.T) {
	t.Parallel()

	calendar := New()
	for _, testCase := range []struct {
		name          string
		macro         string
		errorFragment string
	}{
		{name: "hours", macro: "<test_date+1h>", errorFragment: "hours are not supported"},
		{name: "minutes", macro: "<test_date+1m>", errorFragment: "minutes are not supported"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			_, _, err := calendar.resolveStringValue(testCase.macro, "")
			require.Error(t, err)
			assert.Contains(t, err.Error(), testCase.errorFragment)
		})
	}
}

// TestParseOffset_RejectsDuplicateUnits checks that duplicate units are considered an error rather than silently added together.
func TestParseOffset_RejectsDuplicateUnits(t *testing.T) {
	t.Parallel()

	_, err := parseOffset("+1d2d", macroKindDate)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate")
}

// TestParseOffset_RejectsOutOfOrderUnits checks the canonical order of units for timestamp macros.
func TestParseOffset_RejectsOutOfOrderUnits(t *testing.T) {
	t.Parallel()

	_, err := parseOffset("+30m7d20h", macroKindTimestamp)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "order")
}
