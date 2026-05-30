package bookingcalendar

import (
	"context"
	"errors"
	"fmt"
	"time"
)

const (
	// timestampLayout matches testcalendar output so fixtures and runtime values compare byte-for-byte.
	timestampLayout = "2006-01-02 15:04:05-07:00"
	// dayDuration keeps date arithmetic explicit.
	// The example works with calendar days, not durations from the payload.
	dayDuration = 24 * time.Hour
)

// seedBooking describes one booking record preloaded before the action step.
type seedBooking struct {
	BookingID    string `json:"booking_id"`
	HotelID      string `json:"hotel_id"`
	CheckInDate  string `json:"check_in_date"`
	CheckOutDate string `json:"check_out_date"`
}

// seedDataRequest contains bookings that initialize the in-memory state.
type seedDataRequest struct {
	Bookings []seedBooking `json:"bookings"`
}

// seedDataResponse confirms how many bookings were stored.
type seedDataResponse struct {
	StoredBookings int `json:"stored_bookings"`
}

// externalQuoteRequest models the outbound request sent to the external pricing API.
type externalQuoteRequest struct {
	HotelID      string `json:"hotel_id"`
	CheckInDate  string `json:"check_in_date"`
	CheckOutDate string `json:"check_out_date"`
	RequestedAt  string `json:"requested_at"`
}

// externalQuoteResponse models the stubbed external pricing response.
type externalQuoteResponse struct {
	RateID    string `json:"rate_id"`
	ExpiresAt string `json:"expires_at"`
}

// planExternalQuoteRequest stores the expected outbound request and stub response.
type planExternalQuoteRequest struct {
	ExpectedRequest externalQuoteRequest  `json:"expected_request"`
	StubResponse    externalQuoteResponse `json:"stub_response"`
}

// planExternalQuoteResponse confirms that the external expectation is configured.
type planExternalQuoteResponse struct {
	Planned bool `json:"planned"`
}

// createBookingQuoteRequest describes the inbound API call that creates a booking quote.
type createBookingQuoteRequest struct {
	BookingID string `json:"booking_id"`
}

// createBookingQuoteResponse returns the booking quote enriched with computed date fields.
type createBookingQuoteResponse struct {
	BookingID        string `json:"booking_id"`
	HotelID          string `json:"hotel_id"`
	CheckInDate      string `json:"check_in_date"`
	CheckOutDate     string `json:"check_out_date"`
	Nights           int    `json:"nights"`
	DaysUntilCheckIn int    `json:"days_until_check_in"`
	RateID           string `json:"rate_id"`
	QuoteExpiresAt   string `json:"quote_expires_at"`
}

// verifyExternalQuoteRequest identifies which booking should have triggered the outbound call.
type verifyExternalQuoteRequest struct {
	BookingID string `json:"booking_id"`
}

// verifyExternalQuoteResponse returns the observed outbound call details.
type verifyExternalQuoteResponse struct {
	Called      bool                 `json:"called"`
	CallsCount  int                  `json:"calls_count"`
	LastRequest externalQuoteRequest `json:"last_request"`
}

// bookingClient stores in-memory state for both the SUT and the external API mock.
type bookingClient struct {
	now                  func() time.Time
	bookings             map[string]seedBooking
	quotePlanned         bool
	expectedQuoteRequest externalQuoteRequest
	stubQuoteResponse    externalQuoteResponse
	quoteCalls           int
	lastQuotedBookingID  string
	lastQuoteRequest     externalQuoteRequest
}

// newBookingClient creates isolated state with an injectable now-provider.
func newBookingClient(now func() time.Time) *bookingClient {
	client := &bookingClient{
		now:          now,
		bookings:     nil,
		quotePlanned: false,
		expectedQuoteRequest: externalQuoteRequest{
			HotelID:      "",
			CheckInDate:  "",
			CheckOutDate: "",
			RequestedAt:  "",
		},
		stubQuoteResponse:   externalQuoteResponse{RateID: "", ExpiresAt: ""},
		quoteCalls:          0,
		lastQuotedBookingID: "",
		lastQuoteRequest: externalQuoteRequest{
			HotelID:      "",
			CheckInDate:  "",
			CheckOutDate: "",
			RequestedAt:  "",
		},
	}
	client.reset(0)

	return client
}

// reset clears per-case state so every fixture starts from the same in-memory baseline.
func (client *bookingClient) reset(bookingsCapacity int) {
	client.bookings = make(map[string]seedBooking, bookingsCapacity)
	client.quotePlanned = false
	client.expectedQuoteRequest = externalQuoteRequest{
		HotelID:      "",
		CheckInDate:  "",
		CheckOutDate: "",
		RequestedAt:  "",
	}
	client.stubQuoteResponse = externalQuoteResponse{RateID: "", ExpiresAt: ""}
	client.quoteCalls = 0
	client.lastQuotedBookingID = ""
	client.lastQuoteRequest = externalQuoteRequest{
		HotelID:      "",
		CheckInDate:  "",
		CheckOutDate: "",
		RequestedAt:  "",
	}
}

// SeedData resets the in-memory storage for the next case and stores fixture bookings.
func (client *bookingClient) SeedData(_ context.Context, request *seedDataRequest) (*seedDataResponse, error) {
	client.reset(len(request.Bookings))

	for _, booking := range request.Bookings {
		client.bookings[booking.BookingID] = booking
	}

	return &seedDataResponse{StoredBookings: len(request.Bookings)}, nil
}

// PlanExternalQuote stores expectations for the outbound pricing call.
func (client *bookingClient) PlanExternalQuote(
	_ context.Context,
	request *planExternalQuoteRequest,
) (*planExternalQuoteResponse, error) {
	client.quotePlanned = true
	client.expectedQuoteRequest = request.ExpectedRequest
	client.stubQuoteResponse = request.StubResponse
	client.quoteCalls = 0
	client.lastQuotedBookingID = ""
	client.lastQuoteRequest = externalQuoteRequest{
		HotelID:      "",
		CheckInDate:  "",
		CheckOutDate: "",
		RequestedAt:  "",
	}

	return &planExternalQuoteResponse{Planned: true}, nil
}

// CreateBookingQuote validates the stay window, performs the outbound call, and returns a deterministic response.
func (client *bookingClient) CreateBookingQuote(
	_ context.Context,
	request *createBookingQuoteRequest,
) (*createBookingQuoteResponse, error) {
	booking, exists := client.bookings[request.BookingID]
	if !exists {
		return nil, newBookingStatusError("booking %q is not found", request.BookingID)
	}

	checkInDate, checkOutDate, todayDate, err := client.validateStayWindow(booking)
	if err != nil {
		return nil, err
	}

	requestPayload := externalQuoteRequest{
		HotelID:      booking.HotelID,
		CheckInDate:  booking.CheckInDate,
		CheckOutDate: booking.CheckOutDate,
		RequestedAt:  client.now().Format(timestampLayout),
	}
	quoteResponse, err := client.invokeExternalQuote(booking.BookingID, requestPayload)
	if err != nil {
		return nil, newBookingStatusError("external quote failed: %v", err)
	}

	return &createBookingQuoteResponse{
		BookingID:        booking.BookingID,
		HotelID:          booking.HotelID,
		CheckInDate:      booking.CheckInDate,
		CheckOutDate:     booking.CheckOutDate,
		Nights:           int(checkOutDate.Sub(checkInDate) / dayDuration),
		DaysUntilCheckIn: int(checkInDate.Sub(todayDate) / dayDuration),
		RateID:           quoteResponse.RateID,
		QuoteExpiresAt:   quoteResponse.ExpiresAt,
	}, nil
}

// VerifyExternalQuote confirms that the expected booking triggered the outbound pricing request.
func (client *bookingClient) VerifyExternalQuote(
	_ context.Context,
	request *verifyExternalQuoteRequest,
) (*verifyExternalQuoteResponse, error) {
	if client.quoteCalls == 0 {
		return nil, newBookingStatusError("external pricing API was not called")
	}

	if client.lastQuotedBookingID != request.BookingID {
		return nil, newBookingStatusError(
			"unexpected booking for external quote: got %q",
			client.lastQuotedBookingID,
		)
	}

	return &verifyExternalQuoteResponse{
		Called:      true,
		CallsCount:  client.quoteCalls,
		LastRequest: client.lastQuoteRequest,
	}, nil
}

// validateStayWindow ensures the booking uses a future or same-day check-in and a later check-out.
func (client *bookingClient) validateStayWindow(
	booking seedBooking,
) (checkInDate, checkOutDate, todayDate time.Time, err error) {
	checkInDate, err = parseDateOnly(booking.CheckInDate)
	if err != nil {
		return time.Time{}, time.Time{}, time.Time{}, newBookingStatusError("parse check-in date: %v", err)
	}

	checkOutDate, err = parseDateOnly(booking.CheckOutDate)
	if err != nil {
		return time.Time{}, time.Time{}, time.Time{}, newBookingStatusError("parse check-out date: %v", err)
	}

	todayDate, err = parseDateOnly(client.now().Format(time.DateOnly))
	if err != nil {
		return time.Time{}, time.Time{}, time.Time{}, newBookingStatusError("parse today date: %v", err)
	}

	if checkInDate.Before(todayDate) {
		return time.Time{}, time.Time{}, time.Time{}, newBookingStatusError("check-in date must not be before today")
	}

	if !checkOutDate.After(checkInDate) {
		return time.Time{}, time.Time{}, time.Time{}, newBookingStatusError("check-out date must be after check-in date")
	}

	return checkInDate, checkOutDate, todayDate, nil
}

// invokeExternalQuote validates the outbound payload against the prepared expectation.
func (client *bookingClient) invokeExternalQuote(
	bookingID string,
	request externalQuoteRequest,
) (externalQuoteResponse, error) {
	if !client.quotePlanned {
		return externalQuoteResponse{}, errors.New("external expectation is not planned")
	}

	if request != client.expectedQuoteRequest {
		return externalQuoteResponse{}, fmt.Errorf(
			"external request mismatch: want %+v got %+v",
			client.expectedQuoteRequest,
			request,
		)
	}

	client.quoteCalls++
	client.lastQuotedBookingID = bookingID
	client.lastQuoteRequest = request

	return client.stubQuoteResponse, nil
}

// parseDateOnly converts fixture date strings into normalized midnight timestamps.
func parseDateOnly(raw string) (time.Time, error) {
	parsed, err := time.Parse(time.DateOnly, raw)
	if err != nil {
		return time.Time{}, err
	}

	return parsed, nil
}
