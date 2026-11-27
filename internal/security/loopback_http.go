package security

import (
	"net"
	"net/http"
	"strings"
)

// LoopbackHTTP restricts local servers to loopback Host headers and protects
// state-changing routes against cross-origin browser requests.
func LoopbackHTTP(next http.Handler) http.Handler {
	protected := http.NewCrossOriginProtection().Handler(next)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := r.Host
		if name, _, err := net.SplitHostPort(host); err == nil {
			host = name
		}
		ip := net.ParseIP(strings.Trim(host, "[]"))
		if !strings.EqualFold(host, "localhost") && (ip == nil || !ip.IsLoopback()) {
			http.Error(w, "loopback host required", http.StatusForbidden)
			return
		}
		protected.ServeHTTP(w, r)
	})
}
