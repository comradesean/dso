// Package dnsredirect is a small DNS server for pointing a real PS3 console at
// this server. A real console has no IP/Hosts switch (unlike RPCS3), so the PS3
// is configured with this as its Primary DNS. It answers the FromSoftware
// hostnames with our advertise address and forwards every other query upstream.
package dnsredirect

import (
	"context"
	"fmt"
	"net"
	"strings"

	"github.com/miekg/dns"

	"github.com/sstreight/dso/internal/server/core"
)

// Service is the DNS redirect server.
type Service struct {
	srv       *core.Server
	overrides map[string]net.IP // fqdn (lowercase) -> IP
}

// New creates the DNS service, mapping the configured hostnames to the advertise
// address.
func New(srv *core.Server) *Service {
	ip := net.ParseIP(srv.Config.AdvertiseAddress)
	ov := make(map[string]net.IP)
	for _, h := range srv.Config.DNSRedirectHosts {
		ov[dns.Fqdn(strings.ToLower(h))] = ip
	}
	return &Service{srv: srv, overrides: ov}
}

// Name implements core.Service.
func (s *Service) Name() string { return "dns" }

// Serve runs the UDP and TCP DNS listeners until ctx is cancelled.
func (s *Service) Serve(ctx context.Context) error {
	addr := fmt.Sprintf("%s:%d", s.srv.Config.BindAddress, s.srv.Config.DNSPort)
	mux := dns.NewServeMux()
	mux.HandleFunc(".", s.handle)

	udp := &dns.Server{Addr: addr, Net: "udp", Handler: mux}
	tcp := &dns.Server{Addr: addr, Net: "tcp", Handler: mux}

	errc := make(chan error, 2)
	go func() { errc <- udp.ListenAndServe() }()
	go func() { errc <- tcp.ListenAndServe() }()

	hosts := make([]string, 0, len(s.overrides))
	for h := range s.overrides {
		hosts = append(hosts, h)
	}
	s.srv.Logger.Info("dns redirect listening", "addr", addr,
		"advertise", s.srv.Config.AdvertiseAddress, "upstream", s.srv.Config.DNSUpstream, "hosts", hosts)

	go func() {
		<-ctx.Done()
		_ = udp.Shutdown()
		_ = tcp.Shutdown()
	}()

	select {
	case <-ctx.Done():
		return nil
	case err := <-errc:
		// Port 53 needs privilege; make this non-fatal so the rest of the server
		// keeps running.
		s.srv.Logger.Error("dns bind failed; port 53 needs privilege (sudo sysctl -w net.ipv4.ip_unprivileged_port_start=53)",
			"addr", addr, "err", err)
		return nil
	}
}

func (s *Service) handle(w dns.ResponseWriter, r *dns.Msg) {
	if len(r.Question) == 1 {
		q := r.Question[0]
		if ip, ok := s.overrides[strings.ToLower(q.Name)]; ok {
			m := new(dns.Msg)
			m.SetReply(r)
			m.Authoritative = true
			switch q.Qtype {
			case dns.TypeA:
				if v4 := ip.To4(); v4 != nil {
					m.Answer = append(m.Answer, &dns.A{
						Hdr: dns.RR_Header{Name: q.Name, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 60},
						A:   v4,
					})
				}
				s.srv.Logger.Info("dns redirect hit", "name", q.Name, "type", "A", "answer", ip.String())
			default:
				// For AAAA and others on our hosts, answer with no records so the
				// console falls back to the A record.
				s.srv.Logger.Info("dns redirect hit (no-data)", "name", q.Name, "qtype", dns.TypeToString[q.Qtype])
			}
			_ = w.WriteMsg(m)
			return
		}
	}
	s.forward(w, r)
}

func (s *Service) forward(w dns.ResponseWriter, r *dns.Msg) {
	c := new(dns.Client)
	resp, _, err := c.Exchange(r, s.srv.Config.DNSUpstream)
	if err != nil || resp == nil {
		m := new(dns.Msg)
		m.SetRcode(r, dns.RcodeServerFailure)
		_ = w.WriteMsg(m)
		return
	}
	_ = w.WriteMsg(resp)
}
