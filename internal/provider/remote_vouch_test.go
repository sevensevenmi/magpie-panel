package provider

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"testing"
)

// keyedSeen is a server that records the keys it was sent; one that
// holds key proves it (as a magpie's remote_prove.go does), and one
// that doesn't answers everything 200, as anything at that address on a
// café's network could.
type keyedSeen struct {
	mu    sync.Mutex
	keys  []string
	hosts []string
	srv   *httptest.Server
}

func newKeyedSeen(t *testing.T, holds string) *keyedSeen {
	k := &keyedSeen{}
	k.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == RemoteProvePath {
			var in struct {
				Nonce string `json:"nonce"`
				KeyID string `json:"key_id"`
			}
			json.NewDecoder(r.Body).Decode(&in)
			at := r.Context().Value(http.LocalAddrContextKey).(net.Addr).String()
			if holds == "" || in.KeyID != RemoteKeyID(holds) {
				w.Write([]byte(`{"proof":"00"}`))
				return
			}
			json.NewEncoder(w).Encode(map[string]string{"proof": hex.EncodeToString(RemoteProof(holds, in.Nonce, at))})
			return
		}
		k.mu.Lock()
		if a := r.Header.Get("Authorization") + r.Header.Get("X-Api-Key"); a != "" {
			k.keys = append(k.keys, a)
		}
		k.hosts = append(k.hosts, r.Host)
		k.mu.Unlock()
		w.Write([]byte(`{"data":[]}`))
	}))
	t.Cleanup(k.srv.Close)
	return k
}

func (k *keyedSeen) seen() ([]string, []string) {
	k.mu.Lock()
	defer k.mu.Unlock()
	return append([]string(nil), k.keys...), append([]string(nil), k.hosts...)
}

func vouchOnLoopback(t *testing.T) {
	oldHost, oldNets, oldLookup := vouchHost, vouchNets, vouchLookup
	vouchHost = func(string) bool { return true } // as a home network's address would be
	vouchNets = func() string { return t.Name() }
	t.Cleanup(func() {
		vouchHost, vouchNets, vouchLookup = oldHost, oldNets, oldLookup
		vouchSeen.Lock()
		clear(vouchSeen.m)
		vouchSeen.Unlock()
	})
}

// A Remote magpie at home's address, asked from a café: whatever answers
// at that address there is sent nothing keyed, the remote's other
// address, which proves it holds the key, is asked, and the impostor
// isn't asked first from then on.
func TestRemoteMagpieAddressProvesBeforeItsKey(t *testing.T) {
	azureHome(t)
	dialsOut(t)
	vouchOnLoopback(t)
	const key = "sk-magpie-office"
	impostor, real := newKeyedSeen(t, ""), newKeyedSeen(t, key)
	if err := Save(Provider{ID: "office", Name: "Office", Preset: RemoteMagpiePreset, Key: key, Chat: impostor.srv.URL, Addresses: []string{real.srv.URL}}); err != nil {
		t.Fatal(err)
	}
	p, err := Find("office")
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if _, err := remoteBody(context.Background(), *p, "GET", "/v1/models", seekAnswer); err != nil {
			t.Fatal(err)
		}
	}
	if keys, _ := impostor.seen(); len(keys) > 0 {
		t.Fatalf("the address that didn't prove was sent %q", keys)
	}
	if keys, _ := real.seen(); len(keys) != 2 || !strings.Contains(keys[0], key) {
		t.Fatalf("the remote was sent %q; want its key, twice", keys)
	}
}

// A .local name is connected to at the address that proved, the name
// still its Host; a remote that can't prove, an older one, is sent
// nothing, and the request says why.
func TestRemoteMagpieNameProvesAtItsAddress(t *testing.T) {
	azureHome(t)
	dialsOut(t)
	vouchOnLoopback(t)
	const key = "sk-magpie-office"
	real, old := newKeyedSeen(t, key), newKeyedSeen(t, "")
	_, port, _ := net.SplitHostPort(strings.TrimPrefix(real.srv.URL, "http://"))
	vouchLookup = func(context.Context, string) ([]string, error) { return []string{"127.0.0.1"}, nil }
	if err := Save(Provider{ID: "office", Name: "Office", Preset: RemoteMagpiePreset, Key: key, Chat: "http://office.local:" + port, Addresses: []string{old.srv.URL}}); err != nil {
		t.Fatal(err)
	}
	p, _ := Find("office")
	if _, err := remoteBody(context.Background(), *p, "GET", "/v1/models", seekAnswer); err != nil {
		t.Fatal(err)
	}
	if keys, hosts := real.seen(); len(keys) != 1 || len(hosts) != 1 || hosts[0] != "office.local:"+port {
		t.Fatalf("the remote was sent %q at %q; want its key once, under its name", keys, hosts)
	}
	vouchLookup = func(context.Context, string) ([]string, error) {
		return nil, &net.DNSError{Err: "no such host", IsNotFound: true}
	}
	vouchSeen.Lock()
	clear(vouchSeen.m)
	vouchSeen.Unlock()
	_, err := remoteBody(context.Background(), *p, "GET", "/v1/models", seekAnswer)
	if keys, _ := old.seen(); len(keys) > 0 {
		t.Fatalf("an address that didn't prove was sent %q", keys)
	}
	if err == nil || !strings.Contains(err.Error(), "did not prove") {
		t.Fatalf("err %v; want it to say the address didn't prove", err)
	}
}

// A provider proxy answers an address it can't reach with its own 502.
// That is not the remote's answer: nothing keyed goes to it, and the
// other address that proves is asked instead.
func TestRemoteMagpieProxyDoesNotSendKeyToUnreachableAddress(t *testing.T) {
	azureHome(t)
	dialsOut(t)
	const key = "sk-magpie-office"
	const unreachable = "192.168.255.254"
	const fallbackHost = "10.20.30.40"
	var mu sync.Mutex
	keys := map[string][]string{}
	asked := map[string]int{}
	var fallbackKeys []string
	fallback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == RemoteProvePath {
			var in struct {
				Nonce string `json:"nonce"`
				KeyID string `json:"key_id"`
			}
			json.NewDecoder(r.Body).Decode(&in)
			if in.KeyID != RemoteKeyID(key) {
				http.Error(w, "no such key", http.StatusNotFound)
				return
			}
			json.NewEncoder(w).Encode(map[string]string{"proof": hex.EncodeToString(RemoteProof(key, in.Nonce, fallbackHost+":80"))})
			return
		}
		if sent := r.Header.Get("Authorization") + "\n" + r.Header.Get("X-Api-Key"); strings.TrimSpace(sent) != "" {
			mu.Lock()
			fallbackKeys = append(fallbackKeys, sent)
			mu.Unlock()
		}
		if r.URL.Path == "/v1/models" {
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"data":[]}`)
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(fallback.Close)
	fallbackURL, err := url.Parse(fallback.URL)
	if err != nil {
		t.Fatal(err)
	}
	fallbackProxy := httputil.NewSingleHostReverseProxy(fallbackURL)
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		target := r.URL.Hostname()
		if target == "" {
			target = r.Host
		}
		mu.Lock()
		asked[target]++
		mu.Unlock()
		if sent := r.Header.Get("Authorization") + "\n" + r.Header.Get("X-Api-Key"); strings.TrimSpace(sent) != "" {
			mu.Lock()
			keys[target] = append(keys[target], sent)
			mu.Unlock()
		}
		if target == fallbackHost {
			fallbackProxy.ServeHTTP(w, r)
			return
		}
		http.Error(w, "the proxy could not reach it", http.StatusBadGateway)
	}))
	t.Cleanup(proxy.Close)
	if err := Save(Provider{ID: "office", Name: "Office", Preset: RemoteMagpiePreset, Key: key,
		Chat: "http://" + unreachable, Addresses: []string{"http://" + fallbackHost}, Proxy: proxy.URL}); err != nil {
		t.Fatal(err)
	}
	p, err := Find("office")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := remoteBody(context.Background(), *p, "GET", "/v1/models", seekAnswer); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	atPrimary := append([]string(nil), keys[unreachable]...)
	probes := asked[unreachable]
	atFallback := append([]string(nil), fallbackKeys...)
	mu.Unlock()
	if probes == 0 {
		t.Fatal("the proxy wasn't asked to reach the unreachable address")
	}
	if len(atPrimary) > 0 {
		t.Fatalf("the unreachable address was sent %q", atPrimary)
	}
	if len(atFallback) != 1 || !strings.Contains(atFallback[0], key) {
		t.Fatalf("the address that proved was sent %q; want its key once", atFallback)
	}
}
