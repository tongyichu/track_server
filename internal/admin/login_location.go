package admin

import (
	"io"
	"net"
	"net/netip"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/oschwald/maxminddb-golang"
)

type loginLocationDatabase struct {
	reader *maxminddb.Reader
}

func (db *loginLocationDatabase) isDBIP() bool {
	return db != nil && strings.HasPrefix(strings.ToLower(db.reader.Metadata.DatabaseType), "dbip")
}

func (m *Module) ConfigureLoginLocation(path string) (io.Closer, error) {
	if path == "" || m == nil || m.Auth == nil || m.Auth.manager == nil {
		return nil, nil
	}
	reader, err := maxminddb.Open(path)
	if err != nil {
		return nil, err
	}
	m.Auth.loginGeo = &loginLocationDatabase{reader: reader}
	return reader, nil
}

func adminLoginIP(c *app.RequestContext) string {
	ip, err := netip.ParseAddr(adminPeerHost(c))
	if err != nil {
		return ""
	}
	ip = ip.WithZone("").Unmap()
	if ip.IsUnspecified() {
		return ""
	}
	return ip.String()
}

type geoIPPlace struct {
	Names map[string]string `maxminddb:"names"`
	Code  string            `maxminddb:"iso_code"`
}

func (p geoIPPlace) name() string {
	if name := strings.TrimSpace(p.Names["zh-CN"]); name != "" {
		return name
	}
	return strings.TrimSpace(p.Names["en"])
}

func (db *loginLocationDatabase) lookup(address string) string {
	ip, err := netip.ParseAddr(address)
	if err != nil {
		return ""
	}
	if ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
		return "内网地址"
	}
	if db == nil || !ip.IsGlobalUnicast() {
		return ""
	}
	var record struct {
		Country      geoIPPlace   `maxminddb:"country"`
		Subdivisions []geoIPPlace `maxminddb:"subdivisions"`
		City         geoIPPlace   `maxminddb:"city"`
	}
	if err := db.reader.Lookup(net.IP(ip.AsSlice()), &record); err != nil {
		return ""
	}
	country := record.Country.name()
	if country == "" {
		country = record.Country.Code
	}
	parts := make([]string, 0, 2+len(record.Subdivisions))
	seen := make(map[string]bool, cap(parts))
	appendName := func(name string) {
		key := strings.ToLower(name)
		if name != "" && !seen[key] {
			parts = append(parts, name)
			seen[key] = true
		}
	}
	appendName(country)
	for _, subdivision := range record.Subdivisions {
		appendName(subdivision.name())
	}
	appendName(record.City.name())
	return strings.Join(parts, " / ")
}
