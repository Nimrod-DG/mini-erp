package middleware

import (
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// HeaderRequestID is the correlation header, both directions: honoured on the
// way in so a caller's ID survives into our logs, echoed on the way out so a
// user can quote it in a bug report.
const HeaderRequestID = "X-Request-Id"

// RequestID (step 1) assigns a correlation ID to the request.
//
// An inbound value is trusted only as a label — it is truncated and never
// reaches a query, a path, or a log format string as anything but data.
func RequestID() fiber.Handler {
	return func(c *fiber.Ctx) error {
		id := c.Get(HeaderRequestID)
		if id == "" || len(id) > 64 {
			id = uuid.NewString()
		}
		c.Locals(requestIDKey, id)
		c.Set(HeaderRequestID, id)
		return c.Next()
	}
}

const requestIDKey ctxKey = "http.request_id"

func requestID(c *fiber.Ctx) string {
	id, _ := c.Locals(requestIDKey).(string)
	return id
}
