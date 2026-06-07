package processor

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

func scalarValueKey(value Value) (string, bool) {
	switch value.Kind {
	case ValueString:
		return "string:" + value.String, true
	case ValueInt:
		return "int:" + strconv.FormatInt(value.Int, 10), true
	case ValueFloat:
		return "float:" + strconv.FormatUint(math.Float64bits(value.Float), 16), true
	case ValueHexInt:
		return "hex_int:" + formatHexInt(value.Int), true
	case ValueHexFloat:
		return "hex_float:" + strconv.FormatUint(math.Float64bits(value.Float), 16), true
	case ValueBoolean:
		return "boolean:" + strconv.FormatBool(value.Boolean), true
	case ValueNull:
		return "null", true
	default:
		return "", false
	}
}

func scalarValueDisplay(value Value) string {
	switch value.Kind {
	case ValueString:
		return fmt.Sprintf("%q", value.String)
	case ValueInt:
		return strconv.FormatInt(value.Int, 10)
	case ValueFloat:
		return decimalFloatLiteral(value.Float)
	case ValueHexInt:
		return formatHexInt(value.Int)
	case ValueHexFloat:
		return formatHexFloat(value.Float)
	case ValueBoolean:
		return strconv.FormatBool(value.Boolean)
	case ValueNull:
		return "null"
	default:
		return "unknown"
	}
}

func decimalFloatLiteral(value float64) string {
	formatted := strconv.FormatFloat(value, 'g', -1, 64)
	if strings.ContainsAny(formatted, ".eE") {
		return formatted
	}
	return formatted + ".0"
}
