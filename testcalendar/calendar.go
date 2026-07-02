// Package testcalendar provides a fixed calendar
// and substitution of calendar macros for integration tests.
package testcalendar

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/n-r-w/itestkit"
	"github.com/tailscale/hujson"
)

// Constants for a fixed point in time and calendar macro format.
const (
	FixedNowYear      = 2026
	FixedNowMonth     = time.March
	FixedNowDay       = 1
	FixedNowHour      = 3
	FixedNowUTCOffset = 6 * 60 * 60
)

const (
	dateMacroPrefix             = "test_date"
	timestampMacroPrefix        = "test_timestamp"
	rfc3339TimestampMacroPrefix = "test_rfc3339_timestamp"
	timezoneSeparator           = "@"
	dateLayout                  = time.DateOnly
	timestampLayout             = "2006-01-02 15:04:05-07:00"
	jsoncExtension              = ".jsonc"
	dollarDateKey               = "$date"
	minOffsetLength             = 3
	maxMacroSuffixParts         = 2
	timezoneLength              = len("+00:00")
	hoursPerDay                 = 24
	minutesPerHour              = 60
	secondsPerMinute            = 60
)

const (
	offsetRankDays = iota
	offsetRankHours
	offsetRankMinutes
)

// Calendar stores the common calendar anchor for integration tests.
type Calendar struct {
	now time.Time
}

// New creates a test calendar with a fixed point in time.
func New() Calendar {
	return Calendar{now: FixedNow()}
}

// FixedNow returns the total fixed point in time for integration tests.
func FixedNow() time.Time {
	return time.Date(
		FixedNowYear,
		FixedNowMonth,
		FixedNowDay,
		FixedNowHour,
		0,
		0,
		0,
		time.FixedZone("UTC+6", FixedNowUTCOffset),
	)
}

// WrapSource wraps the case source and substitutes calendar macros when reading JSONC.
func (calendar Calendar) WrapSource(source itestkit.CaseSource) itestkit.CaseSource {
	return resolvingSource{
		source:   source,
		calendar: calendar,
	}
}

// resolvingSource replaces the contents of JSONC files before passing it to itestkit.
type resolvingSource struct {
	source   itestkit.CaseSource
	calendar Calendar
}

// compileTimeCaseSourceCheck verifies the implementation of itestkit.CaseSource.
var _ itestkit.CaseSource = (*resolvingSource)(nil)

// ReadDir delegates directory reading to the original case source.
func (source resolvingSource) ReadDir(name string) ([]fs.DirEntry, error) {
	return source.source.ReadDir(name)
}

// ReadFile reads the case file and substitutes calendar macros in JSONC only.
func (source resolvingSource) ReadFile(name string) ([]byte, error) {
	content, err := source.source.ReadFile(name)
	if err != nil {
		return nil, err
	}

	if filepath.Ext(name) != jsoncExtension {
		return content, nil
	}

	return source.calendar.transformJSONC(content)
}

// macroKind describes the types of calendar macros supported.
type macroKind uint8

const (
	macroKindDate macroKind = iota + 1
	macroKindTimestamp
	macroKindRFC3339Timestamp
)

// macroToken stores the parsed calendar macro.
type macroToken struct {
	kind     macroKind
	offset   time.Duration
	location *time.Location
}

// transformJSONC substitutes calendar macros into JSONC string values ​​only.
func (calendar Calendar) transformJSONC(content []byte) ([]byte, error) {
	ast, err := hujson.Parse(content)
	if err != nil {
		return nil, err
	}

	if resolveErr := calendar.resolveValue(&ast, ""); resolveErr != nil {
		return nil, resolveErr
	}

	ast.Standardize()

	return ast.Pack(), nil
}

// resolveValue recursively substitutes calendar macros into string literal values.
func (calendar Calendar) resolveValue(value *hujson.Value, key string) error {
	switch typed := value.Value.(type) {
	case hujson.Literal:
		return calendar.resolveLiteral(value, key, typed)
	case *hujson.Object:
		return calendar.resolveObject(typed)
	case *hujson.Array:
		return calendar.resolveArray(typed)
	default:
		return fmt.Errorf("unsupported HuJSON value kind %q", value.Value.Kind())
	}
}

// resolveObject processes object members while preserving access to the field name for special macros like $date.
func (calendar Calendar) resolveObject(object *hujson.Object) error {
	for index := range object.Members {
		member := &object.Members[index]
		key, err := objectMemberKey(member)
		if err != nil {
			return err
		}

		if resolveErr := calendar.resolveValue(&member.Value, key); resolveErr != nil {
			return resolveErr
		}
	}

	return nil
}

// resolveArray processes array values ​​without object-key context.
func (calendar Calendar) resolveArray(array *hujson.Array) error {
	for index := range array.Elements {
		if err := calendar.resolveValue(&array.Elements[index], ""); err != nil {
			return err
		}
	}

	return nil
}

// resolveLiteral replaces string literal only when it contains a supported calendar macro.
func (calendar Calendar) resolveLiteral(value *hujson.Value, key string, literal hujson.Literal) error {
	if literal.Kind() != '"' {
		return nil
	}

	resolvedValue, wasResolved, err := calendar.resolveStringValue(literal.String(), key)
	if err != nil {
		return err
	}

	if !wasResolved {
		return nil
	}

	value.Value = hujson.String(resolvedValue)

	return nil
}

// objectMemberKey retrieves the object field name for contextual macro substitution.
func objectMemberKey(member *hujson.ObjectMember) (string, error) {
	nameLiteral, ok := member.Name.Value.(hujson.Literal)
	if !ok || nameLiteral.Kind() != '"' {
		return "", errors.New("HuJSON object member name must be a string literal")
	}

	return nameLiteral.String(), nil
}

// resolveStringValue substitutes a calendar macro for a regular JSONC string.
func (calendar Calendar) resolveStringValue(value, key string) (resolvedValue string, wasResolved bool, err error) {
	token, isMacro, err := parseMacroToken(value)
	if err != nil || !isMacro {
		return value, isMacro, err
	}

	renderTime := calendar.now.Add(token.offset)
	if token.location != nil {
		renderTime = renderTime.In(token.location)
	}

	switch token.kind {
	case macroKindDate:
		if key == dollarDateKey {
			return time.Date(
				renderTime.Year(),
				renderTime.Month(),
				renderTime.Day(),
				0,
				0,
				0,
				0,
				time.UTC,
			).Format(time.RFC3339), true, nil
		}

		return renderTime.Format(dateLayout), true, nil
	case macroKindTimestamp:
		if key == dollarDateKey {
			return "", false, fmt.Errorf("macro %q is not supported inside %s", value, dollarDateKey)
		}

		return renderTime.Format(timestampLayout), true, nil
	case macroKindRFC3339Timestamp:
		if key == dollarDateKey {
			return "", false, fmt.Errorf("macro %q is not supported inside %s", value, dollarDateKey)
		}

		return renderTime.Format(time.RFC3339), true, nil
	default:
		return "", false, errors.New("unsupported macro kind")
	}
}

// parseMacroToken parses a full-string calendar macro such as
// <test_date+9d@+03:00>, <test_timestamp-1d@-05:00>, or <test_rfc3339_timestamp+3h>.
func parseMacroToken(value string) (macroToken, bool, error) {
	if strings.HasPrefix(value, "<test_") && !strings.HasSuffix(value, ">") {
		return macroToken{}, true, fmt.Errorf("invalid calendar macro %q", value)
	}

	if !strings.HasPrefix(value, "<") || !strings.HasSuffix(value, ">") {
		return macroToken{}, false, nil
	}

	body := strings.TrimSuffix(strings.TrimPrefix(value, "<"), ">")
	for _, prefix := range []struct {
		name string
		kind macroKind
	}{
		{name: dateMacroPrefix, kind: macroKindDate},
		{name: timestampMacroPrefix, kind: macroKindTimestamp},
		{name: rfc3339TimestampMacroPrefix, kind: macroKindRFC3339Timestamp},
	} {
		if !strings.HasPrefix(body, prefix.name) {
			continue
		}

		offsetPart, location, err := parseMacroSuffix(strings.TrimPrefix(body, prefix.name))
		if err != nil {
			return macroToken{}, true, err
		}

		offsetValue, err := parseOffset(offsetPart, prefix.kind)
		if err != nil {
			return macroToken{}, true, err
		}

		return macroToken{kind: prefix.kind, offset: offsetValue, location: location}, true, nil
	}

	if strings.HasPrefix(body, "test_") {
		return macroToken{}, true, fmt.Errorf("unsupported calendar macro %q", value)
	}

	return macroToken{}, false, nil
}

// parseMacroSuffix separates relative offset from explicit time zone offset.
func parseMacroSuffix(suffix string) (offsetPart string, location *time.Location, err error) {
	parts := strings.Split(suffix, timezoneSeparator)
	if len(parts) > maxMacroSuffixParts {
		return "", nil, fmt.Errorf("invalid timezone separator usage in %q", suffix)
	}

	if len(parts) == 1 {
		return parts[0], nil, nil
	}

	location, err = parseFixedTimezone(parts[1])
	if err != nil {
		return "", nil, err
	}

	return parts[0], location, nil
}

// parseOffset parses the offset suffix for calendar macros.
func parseOffset(offset string, kind macroKind) (time.Duration, error) {
	if offset == "" {
		return 0, nil
	}

	if len(offset) < minOffsetLength {
		return 0, fmt.Errorf("invalid offset %q", offset)
	}

	sign := offset[0]
	if sign != '+' && sign != '-' {
		return 0, fmt.Errorf("invalid offset sign in %q", offset)
	}

	body := offset[1:]
	if body == "" {
		return 0, fmt.Errorf("invalid offset %q", offset)
	}

	totalDuration := time.Duration(0)
	lastRank := -1
	seenRanks := [3]bool{}
	for body != "" {
		unitIndex := strings.IndexAny(body, "dhm")
		if unitIndex <= 0 {
			return 0, fmt.Errorf("invalid offset token in %q", offset)
		}

		amount, err := strconv.Atoi(body[:unitIndex])
		if err != nil {
			return 0, err
		}

		rank, unitDuration, err := offsetUnitSpec(kind, body[unitIndex], offset)
		if err != nil {
			return 0, err
		}

		if seenRanks[rank] {
			return 0, fmt.Errorf("duplicate offset unit %q in %q", body[unitIndex], offset)
		}

		if rank < lastRank {
			return 0, fmt.Errorf("invalid offset unit order in %q", offset)
		}

		seenRanks[rank] = true
		lastRank = rank
		totalDuration += time.Duration(amount) * unitDuration
		body = body[unitIndex+1:]
	}

	if sign == '-' {
		return -totalDuration, nil
	}

	return totalDuration, nil
}

// parseFixedTimezone parses the offset of a time zone like +03:00 or -05:30.
func parseFixedTimezone(raw string) (*time.Location, error) {
	if len(raw) != timezoneLength {
		return nil, fmt.Errorf("invalid timezone offset %q", raw)
	}

	if raw[3] != ':' {
		return nil, fmt.Errorf("invalid timezone offset %q", raw)
	}

	sign := raw[0]
	if sign != '+' && sign != '-' {
		return nil, fmt.Errorf("invalid timezone offset %q", raw)
	}

	hours, err := strconv.Atoi(raw[1:3])
	if err != nil {
		return nil, fmt.Errorf("invalid timezone offset %q", raw)
	}

	minutes, err := strconv.Atoi(raw[4:6])
	if err != nil {
		return nil, fmt.Errorf("invalid timezone offset %q", raw)
	}

	if hours >= hoursPerDay || minutes >= minutesPerHour {
		return nil, fmt.Errorf("invalid timezone offset %q", raw)
	}

	offsetSeconds := (hours*minutesPerHour + minutes) * secondsPerMinute
	if sign == '-' {
		offsetSeconds = -offsetSeconds
	}

	return time.FixedZone(raw, offsetSeconds), nil
}

// offsetUnitSpec returns the rank and unit size for strict order checking and support for units by macro type.
func offsetUnitSpec(kind macroKind, unit byte, offset string) (int, time.Duration, error) {
	switch unit {
	case 'd':
		return offsetRankDays, hoursPerDay * time.Hour, nil
	case 'h':
		if !macroKindSupportsTimeOffset(kind) {
			return 0, 0, fmt.Errorf("hours are not supported in %q", offset)
		}

		return offsetRankHours, time.Hour, nil
	case 'm':
		if !macroKindSupportsTimeOffset(kind) {
			return 0, 0, fmt.Errorf("minutes are not supported in %q", offset)
		}

		return offsetRankMinutes, time.Minute, nil
	default:
		return 0, 0, fmt.Errorf("invalid offset unit in %q", offset)
	}
}

// macroKindSupportsTimeOffset reports whether the macro represents a timestamp with hour and minute precision.
func macroKindSupportsTimeOffset(kind macroKind) bool {
	return kind == macroKindTimestamp || kind == macroKindRFC3339Timestamp
}
