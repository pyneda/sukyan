package active

import (
	"github.com/pyneda/sukyan/lib"
	"github.com/pyneda/sukyan/pkg/scan"
	scan_options "github.com/pyneda/sukyan/pkg/scan/options"
)

// insertionPointsForMode selects which of the extracted points an audit covers.
//
// Smart mode keeps the points that carry user input by construction even when the
// behaviour probes found nothing: body fields, query parameters, the XML whole-body
// point (the only XXE surface), per-element XML values and GraphQL variables/inline
// arguments. Reflection analysis cannot measure XML points, so keying on Behaviour
// alone would drop every SOAP parameter, and it leaves GraphQL operands — the only
// GraphQL surface a resolver ever sees — behind the dynamic probe.
func insertionPointsForMode(mode scan_options.ScanMode, points []scan.InsertionPoint) []scan.InsertionPoint {
	switch mode {
	case scan_options.ScanModeFuzz:
		return points
	case scan_options.ScanModeSmart, scan_options.ScanModeFast:
	default:
		return nil
	}

	selected := make([]scan.InsertionPoint, 0, len(points))
	for _, point := range points {
		if point.Behaviour.IsDynamic || point.Behaviour.IsReflected {
			selected = append(selected, point)
			continue
		}
		if mode == scan_options.ScanModeSmart && carriesInputByConstruction(point) {
			selected = append(selected, point)
		}
	}
	return selected
}

func carriesInputByConstruction(point scan.InsertionPoint) bool {
	switch point.Type {
	case scan.InsertionPointTypeBody, scan.InsertionPointTypeParameter,
		scan.InsertionPointTypeXMLElement, scan.InsertionPointTypeXMLAttribute,
		scan.InsertionPointTypeGraphQLVariable, scan.InsertionPointTypeGraphQLInlineArg:
		return true
	case scan.InsertionPointTypeFullBody:
		return point.ValueType == lib.TypeXML
	default:
		return false
	}
}
