package service

import (
	"bytes"
	"encoding/xml"
	"strings"
)

func exportedKMLCoordinateSystem(data []byte) string {
	if !bytes.Contains(data, []byte("<kml")) {
		return ""
	}
	var document struct {
		Document struct {
			ExtendedData struct {
				Data []struct {
					Name  string `xml:"name,attr"`
					Value string `xml:"value"`
				} `xml:"Data"`
			} `xml:"ExtendedData"`
		} `xml:"Document"`
	}
	if xml.Unmarshal(data, &document) != nil {
		return ""
	}
	for _, entry := range document.Document.ExtendedData.Data {
		if entry.Name == "track_export_coordinate_system" && strings.TrimSpace(entry.Value) == "WGS84" {
			return "WGS84"
		}
	}
	return ""
}
