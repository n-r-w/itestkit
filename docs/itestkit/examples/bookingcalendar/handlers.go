package bookingcalendar

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/n-r-w/itestkit"
)

// seedDataHandler binds the prepare step to booking state initialization.
type seedDataHandler struct{}

var _ itestkit.Handler[*bookingClient] = (*seedDataHandler)(nil)

// DecodeRequest decodes prepare-step payload for initial booking records.
func (seedDataHandler) DecodeRequest(raw json.RawMessage) (any, error) {
	request := &seedDataRequest{Bookings: make([]seedBooking, 0)}
	if err := decodeStrictJSON(raw, request); err != nil {
		return nil, fmt.Errorf("decode SeedData request: %w", err)
	}

	return request, nil
}

// DecodeExpectedResponse decodes expected prepare-step response.
func (seedDataHandler) DecodeExpectedResponse(raw json.RawMessage) (any, error) {
	response := &seedDataResponse{StoredBookings: 0}
	if len(raw) == 0 {
		return response, nil
	}
	if err := decodeStrictJSON(raw, response); err != nil {
		return nil, fmt.Errorf("decode SeedData response: %w", err)
	}

	return response, nil
}

// Invoke initializes booking state for the current case.
func (seedDataHandler) Invoke(ctx context.Context, client *bookingClient, request any) (any, error) {
	typedRequest, ok := request.(*seedDataRequest)
	if !ok {
		return nil, fmt.Errorf("SeedData received invalid request type: %T", request)
	}

	return client.SeedData(ctx, typedRequest)
}

// NormalizeResponse returns the response unchanged for stable fixture comparison.
func (seedDataHandler) NormalizeResponse(response any) (any, error) {
	return response, nil
}

// planExternalQuoteHandler binds the prepare step to external pricing expectations.
type planExternalQuoteHandler struct{}

var _ itestkit.Handler[*bookingClient] = (*planExternalQuoteHandler)(nil)

// DecodeRequest decodes expected outbound payload and stubbed response.
func (planExternalQuoteHandler) DecodeRequest(raw json.RawMessage) (any, error) {
	request := &planExternalQuoteRequest{
		ExpectedRequest: externalQuoteRequest{
			HotelID:      "",
			CheckInDate:  "",
			CheckOutDate: "",
			RequestedAt:  "",
		},
		StubResponse: externalQuoteResponse{RateID: "", ExpiresAt: ""},
	}
	if err := decodeStrictJSON(raw, request); err != nil {
		return nil, fmt.Errorf("decode PlanExternalQuote request: %w", err)
	}

	return request, nil
}

// DecodeExpectedResponse decodes the planning confirmation for the prepare step.
func (planExternalQuoteHandler) DecodeExpectedResponse(raw json.RawMessage) (any, error) {
	response := &planExternalQuoteResponse{Planned: false}
	if len(raw) == 0 {
		return response, nil
	}
	if err := decodeStrictJSON(raw, response); err != nil {
		return nil, fmt.Errorf("decode PlanExternalQuote response: %w", err)
	}

	return response, nil
}

// Invoke stores external pricing expectations inside case state.
func (planExternalQuoteHandler) Invoke(ctx context.Context, client *bookingClient, request any) (any, error) {
	typedRequest, ok := request.(*planExternalQuoteRequest)
	if !ok {
		return nil, fmt.Errorf("PlanExternalQuote received invalid request type: %T", request)
	}

	return client.PlanExternalQuote(ctx, typedRequest)
}

// NormalizeResponse returns planning response unchanged.
func (planExternalQuoteHandler) NormalizeResponse(response any) (any, error) {
	return response, nil
}

// createBookingQuoteHandler binds the action step to the booking quote API.
type createBookingQuoteHandler struct{}

var _ itestkit.Handler[*bookingClient] = (*createBookingQuoteHandler)(nil)

// DecodeRequest decodes action-step input for CreateBookingQuote.
func (createBookingQuoteHandler) DecodeRequest(raw json.RawMessage) (any, error) {
	request := &createBookingQuoteRequest{BookingID: ""}
	if err := decodeStrictJSON(raw, request); err != nil {
		return nil, fmt.Errorf("decode CreateBookingQuote request: %w", err)
	}

	return request, nil
}

// DecodeExpectedResponse decodes expected action response for assert.response.
func (createBookingQuoteHandler) DecodeExpectedResponse(raw json.RawMessage) (any, error) {
	response := &createBookingQuoteResponse{
		BookingID:        "",
		HotelID:          "",
		CheckInDate:      "",
		CheckOutDate:     "",
		Nights:           0,
		DaysUntilCheckIn: 0,
		RateID:           "",
		QuoteExpiresAt:   "",
	}
	if len(raw) == 0 {
		return response, nil
	}
	if err := decodeStrictJSON(raw, response); err != nil {
		return nil, fmt.Errorf("decode CreateBookingQuote response: %w", err)
	}

	return response, nil
}

// Invoke executes the example booking quote API call.
func (createBookingQuoteHandler) Invoke(ctx context.Context, client *bookingClient, request any) (any, error) {
	typedRequest, ok := request.(*createBookingQuoteRequest)
	if !ok {
		return nil, fmt.Errorf("CreateBookingQuote received invalid request type: %T", request)
	}

	return client.CreateBookingQuote(ctx, typedRequest)
}

// NormalizeResponse returns action response unchanged.
func (createBookingQuoteHandler) NormalizeResponse(response any) (any, error) {
	return response, nil
}

// verifyExternalQuoteHandler binds the verify step to outbound call inspection.
type verifyExternalQuoteHandler struct{}

var _ itestkit.Handler[*bookingClient] = (*verifyExternalQuoteHandler)(nil)

// DecodeRequest decodes which booking should have triggered the external pricing request.
func (verifyExternalQuoteHandler) DecodeRequest(raw json.RawMessage) (any, error) {
	request := &verifyExternalQuoteRequest{BookingID: ""}
	if err := decodeStrictJSON(raw, request); err != nil {
		return nil, fmt.Errorf("decode VerifyExternalQuote request: %w", err)
	}

	return request, nil
}

// DecodeExpectedResponse decodes expected verify-step response.
func (verifyExternalQuoteHandler) DecodeExpectedResponse(raw json.RawMessage) (any, error) {
	response := &verifyExternalQuoteResponse{
		Called:     false,
		CallsCount: 0,
		LastRequest: externalQuoteRequest{
			HotelID:      "",
			CheckInDate:  "",
			CheckOutDate: "",
			RequestedAt:  "",
		},
	}
	if len(raw) == 0 {
		return response, nil
	}
	if err := decodeStrictJSON(raw, response); err != nil {
		return nil, fmt.Errorf("decode VerifyExternalQuote response: %w", err)
	}

	return response, nil
}

// Invoke verifies outbound-call fact and payload.
func (verifyExternalQuoteHandler) Invoke(ctx context.Context, client *bookingClient, request any) (any, error) {
	typedRequest, ok := request.(*verifyExternalQuoteRequest)
	if !ok {
		return nil, fmt.Errorf("VerifyExternalQuote received invalid request type: %T", request)
	}

	return client.VerifyExternalQuote(ctx, typedRequest)
}

// NormalizeResponse returns verify response unchanged.
func (verifyExternalQuoteHandler) NormalizeResponse(response any) (any, error) {
	return response, nil
}

// decodeStrictJSON enforces strict parsing for request and response fixture payloads.
func decodeStrictJSON(raw json.RawMessage, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("decode json: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("decode json: trailing data")
	}

	return nil
}
