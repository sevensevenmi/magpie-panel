package provider

// A Remote magpie may be reached from more than one network: at home at
// its LAN address, from outside through Tailscale or EasyTier (evjohnicu
// on X). Away from home, its home network's address is a private one on
// whatever network this computer is on now, a café's or a hotel's, where
// any host there, one that took that address or answers ARP for it,
// would take the connection and be handed the provider's key in clear,
// and, answering, become the address asked first from then on, given
// the user's prompts and files. A look for a moved remote
// (remote_moved.go) guards against just that, and so does this: an
// address reached in plain http at a private or link-local IP, a .local
// name, or any plain-http address that isn't Tailscale is sent nothing
// keyed until it proves it holds the provider's gateway key (proves),
// the proof asked the way the request itself would go. The same holds
// for https when InsecureTLS leaves its certificate unchecked and no
// ServerPin names it. This applies whether the remote has one address or
// several. A direct name is resolved here and connected to at the IP that
// proved; through a provider proxy the name is left to the proxy. A
// direct certificate-checked https address is the server its certificate
// says it is, and a direct Tailscale address (100.64.0.0/10) is the peer
// WireGuard says it is: neither is asked to prove. A verdict is kept for
// the networks this computer is on (vouchPrint) for a while. A provider
// proxy or a Responses socket asks first even for an address it would
// otherwise trust: a proxy can answer an unreachable address with its
// own 502, and a socket handshake carries the key before Dispatch can
// fail over.

import (
	"context"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/yetone/magpie/internal/netproxy"
)

// vouchHost says host is on a network another network can't tell apart
// from its own: a private or link-local IP, a .local name. Swapped in
// tests, whose magpies are on loopback.
var vouchHost = func(host string) bool {
	h := strings.ToLower(strings.TrimSuffix(host, "."))
	if strings.HasSuffix(h, ".local") {
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && !ip.IsLoopback() && (ip.IsPrivate() || ip.IsLinkLocalUnicast())
}

// vouchLookup resolves a name to the addresses it is asked to prove at;
// swapped in tests.
var vouchLookup = func(ctx context.Context, host string) ([]string, error) {
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, ip := range ips {
		out = append(out, ip.IP.String())
	}
	return out, nil
}

// vouchNets names the networks this computer is on now; swapped in tests.
var vouchNets = func() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ""
	}
	var names []string
	for _, a := range addrs {
		if n, ok := a.(*net.IPNet); ok && !n.IP.IsLoopback() {
			names = append(names, n.String())
		}
	}
	slices.Sort(names)
	return strings.Join(names, ",")
}

// vouchKept is how long an address that proved is taken as proved, on
// the same networks; one that didn't is asked again after vouchMissed.
var vouchKept, vouchMissed = 10 * time.Minute, 30 * time.Second

type vouched struct {
	at    string
	ok    bool
	until time.Time
}

var vouchSeen = struct {
	sync.Mutex
	m map[string]vouched
}{m: map[string]vouched{}}

// vouch is p's netproxy.Vouch: what of its addresses may be sent its key.
func (p Provider) vouch() netproxy.Vouch {
	key := p.Key
	proxy := strings.TrimSpace(p.ProxyChoice())
	throughProxy := proxy != "" && !strings.EqualFold(proxy, "direct")
	probeAll := throughProxy || p.SocketMode() != ""
	return func(ctx context.Context, root *url.URL, ask func(*http.Request) (*http.Response, error)) (string, bool) {
		if key == "" {
			return "", true
		}
		plainHTTP := strings.EqualFold(root.Scheme, "http")
		secureHTTP := strings.EqualFold(root.Scheme, "https")
		private := plainHTTP && vouchHost(root.Hostname())
		// Plain http carries no server identity, and InsecureTLS without a
		// pin leaves https with none either. Both must prove, even when
		// this is the remote's only address.
		untrusted := plainHTTP && !tailscaleHost(root.Hostname()) && !loopbackHost(root.Hostname()) || secureHTTP && p.InsecureTLS && p.ServerPin == ""
		if !private && !untrusted && !probeAll {
			return "", true
		}
		seen := strings.Join([]string{root.String(), RemoteKeyID(key), vouchNets(), proxy, p.SocketMode(),
			strconv.FormatBool(p.InsecureTLS), p.ServerPin}, "\n")
		vouchSeen.Lock()
		v, had := vouchSeen.m[seen]
		vouchSeen.Unlock()
		if had && time.Now().Before(v.until) {
			return v.at, v.ok
		}
		v = vouched{until: time.Now().Add(vouchMissed)}
		proof := private || plainHTTP && !tailscaleHost(root.Hostname()) || secureHTTP && p.InsecureTLS && p.ServerPin == ""
		if proof {
			port := root.Port()
			if port == "" {
				port = "80"
				if secureHTTP {
					port = "443"
				}
			}
			hosts := []string{root.Hostname()}
			if net.ParseIP(root.Hostname()) == nil && !throughProxy {
				lctx, cancel := context.WithTimeout(ctx, seekAnswer)
				hosts, _ = vouchLookup(lctx, root.Hostname())
				cancel()
				if len(hosts) > 4 {
					hosts = hosts[:4]
				}
			}
			for _, h := range hosts {
				at := net.JoinHostPort(h, port)
				if provesVia(ctx, root.Scheme, at, strings.TrimRight(root.Path, "/"), key, ask) {
					v = vouched{at: at, ok: true, until: time.Now().Add(vouchKept)}
					break
				}
			}
		} else if healthyVia(ctx, root, ask) {
			v = vouched{ok: true, until: time.Now().Add(vouchKept)}
		}
		vouchSeen.Lock()
		if len(vouchSeen.m) >= 256 {
			clear(vouchSeen.m)
		}
		vouchSeen.m[seen] = v
		vouchSeen.Unlock()
		return v.at, v.ok
	}
}

// tailscaleHost says host is in Tailscale's CGNAT range, the peer
// WireGuard says it is.
func tailscaleHost(host string) bool {
	ip := net.ParseIP(strings.TrimSuffix(host, ".")).To4()
	return ip != nil && ip[0] == 100 && ip[1] >= 64 && ip[1] <= 127
}

// loopbackHost is this computer itself, which no other host on a café's
// network can take over.
func loopbackHost(host string) bool {
	host = strings.TrimSuffix(strings.ToLower(host), ".")
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// healthyVia asks root for an unkeyed HTTP answer. Through https, a
// response after TLS is from the server the certificate names; through
// http, a proxy's own gateway errors don't count as the remote answering.
func healthyVia(ctx context.Context, root *url.URL, ask func(*http.Request) (*http.Response, error)) bool {
	u := *root
	u.Path = strings.TrimRight(root.Path, "/") + RemoteProvePath
	u.RawPath = ""
	ctx, cancel := context.WithTimeout(ctx, seekAnswer)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return false
	}
	req.Header.Set("Accept", "application/json")
	res, err := ask(req)
	if err != nil {
		return false
	}
	defer res.Body.Close()
	if strings.EqualFold(root.Scheme, "https") {
		return res.TLS != nil
	}
	if res.StatusCode == http.StatusProxyAuthRequired || res.StatusCode >= 500 {
		return false
	}
	return true
}
