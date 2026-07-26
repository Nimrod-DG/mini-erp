package db_test

import (
	"os"
	"testing"

	"github.com/DGosal/mini-erp/backend/testsupport"
)

// One PostgreSQL container for the whole package, torn down here.
func TestMain(m *testing.M) {
	code := m.Run()
	testsupport.Shutdown()
	os.Exit(code)
}
