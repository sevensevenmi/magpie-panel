package gateway

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/yetone/magpie/internal/provider"
)

// evjohnicu on X: one computer's magpie reached at home by its LAN
// address, from outside through Tailscale or EasyTier. With its own
// address taking no connection (nothing listens on 127.0.0.1:1), a turn
// goes to its other address and is answered there, and nothing rests.
// In a group it isn't passed over as on another network either: its
// first address may be, while another takes the connection.
func TestRemoteMagpieAskedAtItsOtherAddress(t *testing.T) {
	fresh(t)
	var nets atomic.Value
	nets.Store("192.168.51.7/24")
	lanNetsForTest(t, &nets)
	// the reach check finds nothing at its first address
	reachDial = func(ctx context.Context, hostport string) error {
		if hostport == "127.0.0.1:1" {
			return &net.OpError{Op: "dial", Net: "tcp", Err: context.DeadlineExceeded}
		}
		return nil
	}
	var officeHits, homeHits atomic.Int32
	office := remoteMagpieAt(t, "office", &officeHits)
	home := remoteMagpieAt(t, "home", &homeHits)
	for _, p := range []provider.Provider{
		{ID: "office", Name: "Office", Preset: provider.RemoteMagpiePreset, Key: "sk-magpie-office", Chat: "http://127.0.0.1:1", Addresses: []string{office.URL}, Models: []string{"m"}},
		{ID: "home", Name: "Home", Preset: provider.RemoteMagpiePreset, Key: "sk-magpie-home", Chat: home.URL, Models: []string{"m"}},
	} {
		if err := provider.Save(p); err != nil {
			t.Fatal(err)
		}
	}
	if err := provider.SaveGroup(provider.Group{ID: "g", Name: "G", Members: []string{"office/m", "home/m"}, Routing: provider.Ordered}); err != nil {
		t.Fatal(err)
	}
	h := New().Handler()
	// the group first, before a turn of its own has shown office answers
	if got := askLANGroup(t, h); !strings.Contains(got, "from office") {
		t.Fatalf("group: %s; want office, at its other address, asked first as its place says", got)
	}
	for range 2 {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{"model":"office/m","messages":[{"role":"user","content":"hi"}]}`)))
		if rec.Code != 200 || !strings.Contains(rec.Body.String(), "from office") {
			t.Fatalf("office/m: %d %s", rec.Code, rec.Body)
		}
	}
	if officeHits.Load() != 3 || homeHits.Load() != 0 {
		t.Errorf("office asked %d times, home %d", officeHits.Load(), homeHits.Load())
	}
	if r := restingNow(t); len(r) > 0 {
		t.Errorf("rests: %v", r)
	}
}

// A socket handshake carries the key before Dispatch can fail over. The
// turn asks the unkeyed proof first: an address that doesn't prove is
// sent no key, the other address answers, and the socket turn goes over
// HTTP there.
func TestRemoteMagpieSocketProvesBeforeItsKey(t *testing.T) {
	fresh(t)
	socketOn(t)
	const key = "sk-magpie-office"
	var primaryKeys atomic.Int32
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == provider.RemoteProvePath {
			http.Error(w, "not this remote", http.StatusNotFound)
			return
		}
		if r.Header.Get("Authorization") != "" || r.Header.Get("X-Api-Key") != "" {
			primaryKeys.Add(1)
		}
		http.Error(w, "not this remote", http.StatusBadRequest)
	}))
	t.Cleanup(primary.Close)
	f := &fakeSocketBackend{whole: true, path: "/v1/responses"}
	var fallbackKey atomic.Value
	fallback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == provider.RemoteProvePath {
			var in struct {
				Nonce string `json:"nonce"`
				KeyID string `json:"key_id"`
			}
			json.NewDecoder(r.Body).Decode(&in)
			if in.KeyID != provider.RemoteKeyID(key) {
				http.Error(w, "no such key", http.StatusNotFound)
				return
			}
			at := r.Context().Value(http.LocalAddrContextKey).(net.Addr).String()
			json.NewEncoder(w).Encode(map[string]string{"proof": hex.EncodeToString(provider.RemoteProof(key, in.Nonce, at))})
			return
		}
		if sent := r.Header.Get("Authorization") + "\n" + r.Header.Get("X-Api-Key"); strings.TrimSpace(sent) != "" {
			fallbackKey.Store(sent)
		}
		f.ServeHTTP(w, r)
	}))
	t.Cleanup(fallback.Close)
	if err := provider.Save(provider.Provider{ID: "office", Name: "Office", Preset: provider.RemoteMagpiePreset, Key: key,
		Chat: primary.URL, Addresses: []string{fallback.URL}, Socket: provider.SocketOn, Models: []string{"m"}}); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/v1/responses", strings.NewReader(`{"model":"office/m","stream":true,"input":"ping"}`))
	req.Header.Set("Content-Type", "application/json")
	New().Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "pong") {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	if primaryKeys.Load() != 0 {
		t.Fatalf("the address that didn't prove was sent the key %d times", primaryKeys.Load())
	}
	if v, _ := fallbackKey.Load().(string); !strings.Contains(v, key) {
		t.Fatalf("the address that proved was sent %q; want its key", v)
	}
	if posts, sockets := f.counts(); posts != 1 || sockets != 0 {
		t.Fatalf("%d HTTP posts, %d socket turns; want the turn over HTTP at the address that proved", posts, sockets)
	}
}
