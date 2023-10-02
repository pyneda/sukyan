package scan

import (
	"fmt"
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib"
	"strings"
)

func analyzeInsertionPoints(item *db.History, insertionPoints []InsertionPoint) {
	var base64Data []InsertionPoint
	var base32Data []InsertionPoint
	var base36Data []InsertionPoint
	for _, insertionPoint := range insertionPoints {
		if insertionPoint.ValueType == lib.TypeBase64 {
			base64Data = append(base64Data, insertionPoint)
			// NOTE: If at some time, we have a way to tell the scanner checks to encode payloads,
			// we could check which data type is the decoded data, find insertion points and instruct
			// the scanner checks to base64 encode the original insertion point data.
		} else if insertionPoint.ValueType == lib.TypeBase32 {
			base32Data = append(base32Data, insertionPoint)
		} else if insertionPoint.ValueType == lib.TypeBase36 {
			base36Data = append(base36Data, insertionPoint)
		}

	}

	if len(base64Data) > 0 {
		var sb strings.Builder
		for _, point := range base64Data {
			sb.WriteString(fmt.Sprintf("Found Base64 encoded data in a %s named '%s'. The current value is '%s'.\n", point.Type, point.Name, point.Value))
		}
		db.CreateIssueFromHistoryAndTemplate(item, db.Base64EncodedDataInParameterCode, sb.String(), 90, "", item.WorkspaceID)
	}
	if len(base32Data) > 0 {
		var sb strings.Builder
		for _, point := range base64Data {
			sb.WriteString(fmt.Sprintf("Found Base32 encoded data in a %s named '%s'. The current value is '%s'.\n", point.Type, point.Name, point.Value))
		}
		db.CreateIssueFromHistoryAndTemplate(item, db.Base64EncodedDataInParameterCode, sb.String(), 90, "", item.WorkspaceID)
	}

	if len(base36Data) > 0 {
		var sb strings.Builder
		for _, point := range base64Data {
			sb.WriteString(fmt.Sprintf("Found Base36 encoded data in a %s named '%s'. The current value is '%s'.\n", point.Type, point.Name, point.Value))
		}
		db.CreateIssueFromHistoryAndTemplate(item, db.Base64EncodedDataInParameterCode, sb.String(), 90, "", item.WorkspaceID)
	}
}
