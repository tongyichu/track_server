package admin

import (
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"os"
)

// Render only vector geometry; never copy scripts, styles or external resources
// from an uploaded SVG. The original file and cached source remain unchanged.
func colorTrackLineSVG(file, color string) ([]byte, error) {
	source, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer source.Close()
	const maxSVGBytes = 8 << 20
	data, err := io.ReadAll(io.LimitReader(source, maxSVGBytes+1))
	if err != nil || len(data) > maxSVGBytes {
		return nil, errors.New("cannot read track SVG")
	}
	decoder := xml.NewDecoder(bytes.NewReader(data))
	var output bytes.Buffer
	encoder := xml.NewEncoder(&output)
	depth, lines := 0, 0
	rootSeen := false
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		switch element := token.(type) {
		case xml.StartElement:
			if !rootSeen {
				if element.Name.Local != "svg" {
					return nil, errors.New("missing SVG root")
				}
				rootSeen = true
			} else if depth == 0 || element.Name.Local == "svg" {
				return nil, errors.New("unexpected SVG root")
			}
			switch element.Name.Local {
			case "svg", "g", "polyline", "path", "line", "polygon", "rect", "circle", "ellipse":
			default:
				if err := decoder.Skip(); err != nil {
					return nil, err
				}
				continue
			}
			clean := xml.StartElement{Name: xml.Name{Local: element.Name.Local}}
			if depth == 0 {
				clean.Attr = append(clean.Attr, xml.Attr{Name: xml.Name{Local: "xmlns"}, Value: "http://www.w3.org/2000/svg"})
			}
			for _, attr := range element.Attr {
				if attr.Name.Space != "" {
					continue
				}
				switch attr.Name.Local {
				case "viewBox", "width", "height", "preserveAspectRatio", "points", "d", "transform",
					"x", "y", "x1", "y1", "x2", "y2", "cx", "cy", "r", "rx", "ry",
					"stroke-width", "stroke-linecap", "stroke-linejoin", "stroke-miterlimit", "opacity", "stroke-opacity":
					clean.Attr = append(clean.Attr, attr)
				}
			}
			clean.Attr = append(clean.Attr, xml.Attr{Name: xml.Name{Local: "fill"}, Value: "none"})
			if element.Name.Local != "svg" && element.Name.Local != "g" {
				clean.Attr = append(clean.Attr, xml.Attr{Name: xml.Name{Local: "stroke"}, Value: color})
				lines++
			}
			if err := encoder.EncodeToken(clean); err != nil {
				return nil, err
			}
			depth++
		case xml.EndElement:
			if err := encoder.EncodeToken(xml.EndElement{Name: xml.Name{Local: element.Name.Local}}); err != nil {
				return nil, err
			}
			depth--
		}
	}
	if !rootSeen || depth != 0 || lines == 0 {
		return nil, errors.New("missing track geometry")
	}
	if err := encoder.Flush(); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}
