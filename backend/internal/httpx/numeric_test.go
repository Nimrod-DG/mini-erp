package httpx

import (
	"encoding/json"
	"testing"
)

// The property this type exists for: a decimal that arrives as JSON goes to the
// database as the same digits, and comes back to the client as the same digits.
// No float64 anywhere on that path (I8).
//
// The value below has more precision than a float64 can hold, so a single
// implicit conversion anywhere in the round trip changes it visibly.
func TestNumericRoundTripsWithoutLosingDigits(t *testing.T) {
	const exact = "9007199254740993.0001" // 2^53 + 1, plus a fraction

	var decoded struct {
		Qty Numeric `json:"qty"`
	}
	if err := json.Unmarshal([]byte(`{"qty":`+exact+`}`), &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if string(decoded.Qty) != exact {
		t.Fatalf("after decoding: %q, want %q", decoded.Qty, exact)
	}

	// What the driver would send to PostgreSQL.
	value, err := decoded.Qty.Value()
	if err != nil {
		t.Fatalf("Value: %v", err)
	}
	if value != exact {
		t.Errorf("Value() = %v, want %q", value, exact)
	}

	encoded, err := json.Marshal(decoded)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(encoded) != `{"qty":`+exact+`}` {
		t.Errorf("re-encoded as %s, want the digits unchanged", encoded)
	}
}

// A quoted decimal is accepted too, for clients that carry them as strings on
// purpose — and it marshals back as a number either way, so the wire form does
// not depend on what the client happened to send.
func TestNumericAcceptsStringsAndEmitsNumbers(t *testing.T) {
	var decoded struct {
		Qty Numeric `json:"qty"`
	}
	if err := json.Unmarshal([]byte(`{"qty":"-12.5000"}`), &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.Qty != "-12.5000" {
		t.Fatalf("decoded %q, want -12.5000", decoded.Qty)
	}

	encoded, _ := json.Marshal(decoded)
	if string(encoded) != `{"qty":-12.5000}` {
		t.Errorf("encoded as %s, want an unquoted number", encoded)
	}
}

// Garbage is refused at the edge rather than handed to PostgreSQL to refuse.
// `1e400` is the interesting one: it parses as a number in JSON and would be
// accepted by a float64 field as +Inf.
func TestNumericRejectsWhatIsNotADecimal(t *testing.T) {
	for _, input := range []string{
		`"banana"`, `"1e400"`, `1e400`, `"12.5.6"`, `""`, `"  "`, `"-"`, `"0x10"`,
	} {
		var n Numeric
		if err := json.Unmarshal([]byte(input), &n); err == nil {
			t.Errorf("%s was accepted as %q, want a refusal", input, n)
		}
	}
}

// A missing NUMERIC is zero, not null. The alternative makes every consumer —
// the frontend included — write the same coalesce.
func TestNumericTreatsAbsentAsZero(t *testing.T) {
	var n Numeric
	encoded, err := json.Marshal(n)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(encoded) != "0" {
		t.Errorf("the zero value encoded as %s, want 0", encoded)
	}
	if n.String() != "0" {
		t.Errorf("String() = %q, want 0", n.String())
	}
}

// IsZero and IsNegative decide two refusals in the adjustment endpoint, so the
// scales and signs PostgreSQL actually produces are each named here.
func TestNumericIsZeroAndIsNegative(t *testing.T) {
	for _, c := range []struct {
		in       Numeric
		zero     bool
		negative bool
	}{
		{"0", true, false},
		{"0.0000", true, false},
		{"-0.00", true, false},
		{".00", true, false},
		{"", true, false},
		{"0.0001", false, false},
		{"-0.0001", false, true},
		{"-12.5", false, true},
		{"12.5", false, false},
	} {
		if got := c.in.IsZero(); got != c.zero {
			t.Errorf("%q.IsZero() = %t, want %t", c.in, got, c.zero)
		}
		if got := c.in.IsNegative(); got != c.negative {
			t.Errorf("%q.IsNegative() = %t, want %t", c.in, got, c.negative)
		}
	}
}

// Scan takes the text forms a driver produces, and refuses a float64 loudly
// rather than accepting a value that has already lost precision.
func TestNumericScan(t *testing.T) {
	var n Numeric
	if err := n.Scan("47.0000"); err != nil || n != "47.0000" {
		t.Errorf("Scan(string) = %q, %v", n, err)
	}
	if err := n.Scan([]byte("48.5")); err != nil || n != "48.5" {
		t.Errorf("Scan([]byte) = %q, %v", n, err)
	}
	if err := n.Scan(nil); err != nil || n != "" {
		t.Errorf("Scan(nil) = %q, %v", n, err)
	}
	if err := n.Scan(47.0); err == nil {
		t.Error("Scan(float64) succeeded — a float has already lost whatever it lost")
	}
}
