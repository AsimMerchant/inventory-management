package register

import (
	"os"
	"testing"
	"time"
)

// The fixture is built in IST. json.Unmarshal parses "+05:30" back to time.Local
// when the offsets match, so pinning time.Local to the fixture's own zone is what
// makes reflect.DeepEqual round-trip comparisons hold on any developer machine.
func TestMain(m *testing.M) {
	time.Local = IST
	os.Exit(m.Run())
}
