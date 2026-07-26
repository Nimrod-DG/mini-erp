package httpx

import (
	"bytes"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// decimalPattern is the text form of a NUMERIC. Anchored, and deliberately
// without an exponent: PostgreSQL renders NUMERIC in plain notation, and
// accepting `1e400` from a client would hand the database a value that parses
// but cannot be stored.
var decimalPattern = regexp.MustCompile(`^-?(\d+(\.\d*)?|\.\d+)$`)

// Numeric is a PostgreSQL NUMERIC carried as its exact decimal text.
//
// I8 says money is NUMERIC(18,2) and quantities NUMERIC(18,4), never float, and
// the prohibition does not stop at the column: a NUMERIC scanned into a float64
// has already lost whatever it is going to lose before any arithmetic happens.
// This type never converts. It holds the digits PostgreSQL produced, hands the
// same digits back on the way in, and marshals them as a JSON number without
// ever constructing one.
//
// It does no arithmetic at all, deliberately. Every sum, difference, and
// comparison in the inventory module is written in SQL, where the values are
// still NUMERIC and still exact — `qty_on_hand < p.reorder_point` is decided by
// PostgreSQL, not by Go. A Compare method here would be the first step towards
// deciding a business rule in float64.
type Numeric string

// Zero is what an absent NUMERIC renders as. A missing balance is a balance of
// nothing, and `null` in that slot makes every consumer write the same
// coalesce.
const Zero Numeric = "0"

// ParseNumeric validates client-supplied decimal text.
func ParseNumeric(s string) (Numeric, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", fmt.Errorf("a number is required")
	}
	if !decimalPattern.MatchString(s) {
		return "", fmt.Errorf("%q is not a decimal number", s)
	}
	return Numeric(s), nil
}

// String renders the value, or "0" when it was never set.
func (n Numeric) String() string {
	if n == "" {
		return string(Zero)
	}
	return string(n)
}

// IsZero reports whether the value is numerically zero, whatever scale or sign
// it was written with — `0`, `-0.0000`, and `.00` are all zero.
func (n Numeric) IsZero() bool {
	if n == "" {
		return true
	}
	return strings.Trim(string(n), "-+0.") == "" && strings.ContainsAny(string(n), "0.")
}

// IsNegative reports whether the value carries a minus sign and is not zero.
func (n Numeric) IsNegative() bool {
	return strings.HasPrefix(string(n), "-") && !n.IsZero()
}

// MarshalJSON writes the digits as a JSON number.
//
// Unquoted, so the frontend receives 47.0000 rather than "47.0000" and can
// format it without parsing a string first. The exactness guarantee this type
// exists for is a *server-side* one: the value is never a float in Go and never
// a float in PostgreSQL, which is where the rules are decided (I12 applies to
// arithmetic too — the browser's copy is for display).
func (n Numeric) MarshalJSON() ([]byte, error) {
	if n == "" {
		return []byte(Zero), nil
	}
	if !decimalPattern.MatchString(string(n)) {
		// Unreachable from the database, which only ever produces the pattern
		// above. Refusing rather than emitting is what keeps a bug from
		// producing a response body that is not JSON.
		return nil, fmt.Errorf("numeric %q is not decimal text", string(n))
	}
	return []byte(n), nil
}

// UnmarshalJSON accepts a JSON number or a decimal string.
//
// The number case takes the raw token bytes and keeps them as text — it never
// goes through float64, which is the whole point. A string is accepted too, for
// clients that carry decimals as strings on purpose.
func (n *Numeric) UnmarshalJSON(data []byte) error {
	data = bytes.TrimSpace(data)
	if string(data) == "null" {
		*n = ""
		return nil
	}
	text := string(data)
	if len(data) > 1 && data[0] == '"' {
		var quoted string
		if err := json.Unmarshal(data, &quoted); err != nil {
			return err
		}
		text = quoted
	}
	parsed, err := ParseNumeric(text)
	if err != nil {
		return err
	}
	*n = parsed
	return nil
}

// Scan reads a NUMERIC column. The driver hands it over as text; anything else
// would mean it had already been through a float, so there is no float64 case
// here on purpose.
func (n *Numeric) Scan(src any) error {
	switch v := src.(type) {
	case nil:
		*n = ""
	case string:
		*n = Numeric(v)
	case []byte:
		*n = Numeric(v)
	case int64:
		*n = Numeric(fmt.Sprint(v))
	default:
		return fmt.Errorf("cannot scan %T into a Numeric — the driver returned "+
			"something other than decimal text, which means the value has already "+
			"lost precision", src)
	}
	return nil
}

// Value sends the digits back as text, which PostgreSQL parses into NUMERIC
// exactly.
func (n Numeric) Value() (driver.Value, error) {
	if n == "" {
		return nil, nil
	}
	return string(n), nil
}
