package service

import (
	"bytes"
	"encoding/xml"
	"strings"

	"github.com/tongyichu/track_server/internal/models"
)

func exportedKMLCoordinateSystem(data []byte, track *models.Track) string {
	if track == nil || !bytes.Contains(data, []byte("<kml")) {
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
	// Our GCJ02 declaration and WGS84-normalizing exporter were introduced
	// together. Earlier blank declarations and manual imports are not inferred.
	if track.SourceTag == "" && track.CoordinateSystem == "GCJ02" && bytes.Contains(data, []byte("ff52ffd6")) && bytes.Contains(data, []byte("GxTrackExtendedData")) && bytes.Contains(data, []byte("<gx:Track>")) {
		return "WGS84"
	}
	return ""
}
