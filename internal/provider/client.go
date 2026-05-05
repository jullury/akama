package provider

import (
	"net/http"
	"time"
)

// httpClient is used for all provider API calls with a 30-second timeout.
// http.DefaultClient has no timeout, which causes indefinite hangs on dead connections.
var httpClient = &http.Client{Timeout: 30 * time.Second}
