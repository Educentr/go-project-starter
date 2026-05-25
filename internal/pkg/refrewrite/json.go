package refrewrite

import (
	"fmt"
	"regexp"
)

// jsonRefRE matches `"$ref": "<value>"` where <value> has no double-quotes
// or backslashes. JSON forbids unescaped quotes and multi-line strings in
// values, so a single-line regex is sufficient for OpenAPI/JSON Schema specs.
var jsonRefRE = regexp.MustCompile(`("\$ref"\s*:\s*")([^"\\]+)(")`)

// rewriteJSONBytes scans JSON for "$ref": "..." pairs and applies evaluateRef
// to each value. It only updates the matched value; whitespace, key order, and
// surrounding structure are preserved verbatim.
func rewriteJSONBytes(src []byte, dir string, allowed map[string]struct{}, fileName string, report *Report) ([]byte, bool, error) {
	changed := false

	out := jsonRefRE.ReplaceAllFunc(src, func(match []byte) []byte {
		submatches := jsonRefRE.FindSubmatch(match)
		if len(submatches) != 4 {
			return match
		}

		prefix := submatches[1]
		value := string(submatches[2])
		suffix := submatches[3]

		outcome := evaluateRef(value, dir, allowed)
		switch {
		case outcome.Rewrote:
			report.RefsRewritten++
			changed = true
			result := make([]byte, 0, len(prefix)+len(outcome.NewValue)+len(suffix))
			result = append(result, prefix...)
			result = append(result, outcome.NewValue...)
			result = append(result, suffix...)
			return result
		case outcome.SkipReason != "":
			report.RefsSkipped++
			report.Warnings = append(report.Warnings,
				fmt.Sprintf("%s: $ref %q skipped: %s", fileName, value, outcome.SkipReason))
		}
		return match
	})

	if !changed {
		return src, false, nil
	}
	return out, true, nil
}
