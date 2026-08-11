package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go-backend-template/internal/httpserver/middleware"
)

func resolvedIP(t *testing.T, trusted []string, remoteAddr string, xff ...string) string {
	t.Helper()

	prefixes, err := middleware.ParseTrustedProxies(trusted)
	require.NoError(t, err)

	var got string
	h := middleware.RealIP(prefixes)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = middleware.ClientIP(r)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = remoteAddr
	for _, v := range xff {
		req.Header.Add("X-Forwarded-For", v)
	}
	h.ServeHTTP(httptest.NewRecorder(), req)
	return got
}

func TestRealIP_NoTrustedProxies_IgnoresForwardedHeader(t *testing.T) {
	assert.Equal(t, "203.0.113.9", resolvedIP(t, nil, "203.0.113.9:5555", "1.2.3.4"))
}

func TestRealIP_UntrustedPeer_IgnoresForwardedHeader(t *testing.T) {
	// The spoofing case: an internet client sends its own X-Forwarded-For.
	assert.Equal(t, "203.0.113.9", resolvedIP(t, []string{"172.16.0.0/12"}, "203.0.113.9:5555", "1.2.3.4"))
}

func TestRealIP_TrustedPeer_UsesRightmostUntrustedHop(t *testing.T) {
	// Client forged "9.9.9.9"; real client 203.0.113.9 was appended by the
	// edge proxy, then 172.18.0.5 (Caddy) forwarded it.
	got := resolvedIP(t, []string{"172.16.0.0/12"}, "172.18.0.5:40000", "9.9.9.9, 203.0.113.9, 172.18.0.4")
	assert.Equal(t, "203.0.113.9", got)
}

func TestRealIP_TrustedPeer_AllHopsTrusted_FallsBackToPeer(t *testing.T) {
	got := resolvedIP(t, []string{"172.16.0.0/12"}, "172.18.0.5:40000", "172.18.0.4")
	assert.Equal(t, "172.18.0.5", got)
}

func TestRealIP_TrustedPeer_NoHeader_UsesPeer(t *testing.T) {
	assert.Equal(t, "172.18.0.5", resolvedIP(t, []string{"172.16.0.0/12"}, "172.18.0.5:40000"))
}

func TestRealIP_StripsPortFromForwardedHop(t *testing.T) {
	got := resolvedIP(t, []string{"172.16.0.0/12"}, "172.18.0.5:40000", "203.0.113.9:1234")
	assert.Equal(t, "203.0.113.9", got)
}

func TestParseTrustedProxies(t *testing.T) {
	p, err := middleware.ParseTrustedProxies([]string{" 10.0.0.0/8 ", "", "192.168.1.7"})
	require.NoError(t, err)
	assert.Len(t, p, 2)

	_, err = middleware.ParseTrustedProxies([]string{"not-an-ip"})
	assert.Error(t, err)
}

func TestClientIP_WithoutMiddleware_FallsBackToRemoteAddr(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "198.51.100.7:9999"
	assert.Equal(t, "198.51.100.7", middleware.ClientIP(req))
}
