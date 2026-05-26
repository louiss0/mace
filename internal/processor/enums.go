package processor

import (
	"fmt"
	"strconv"
)

type enumRegistry struct{}

func scalarValueKey(value Value) (string, bool) {
	switch value.Kind {
	case ValueString:
		return "string:" + value.String, true
	case ValueInt:
		return "int:" + strconv.FormatInt(value.Int, 10), true
	case ValueFloat:
		return "float:" + strconv.FormatFloat(value.Float, 'f', 1, 64), true
	case ValueHexInt:
		return "hex_int:" + formatHexInt(value.Int), true
	case ValueHexFloat:
		return "hex_float:" + formatHexFloat(value.Float), true
	case ValueBoolean:
		return "boolean:" + strconv.FormatBool(value.Boolean), true
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
		return strconv.FormatFloat(value.Float, 'f', 1, 64)
	case ValueHexInt:
		return formatHexInt(value.Int)
	case ValueHexFloat:
		return formatHexFloat(value.Float)
	case ValueBoolean:
		return strconv.FormatBool(value.Boolean)
	default:
		return "unknown"
	}
}
