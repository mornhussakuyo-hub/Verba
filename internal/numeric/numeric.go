package numeric

import (
	"fmt"
	"math"
	"math/big"
	"regexp"
	"strconv"
)

type Kind uint8

const (
	Invalid Kind = iota
	Integer
	Real
)

var integerPattern = regexp.MustCompile(`^[+-]?[0-9]+$`)
var realPattern = regexp.MustCompile(`^[+-]?(?:[0-9]+\.[0-9]+|[0-9]+(?:\.[0-9]+)?[eE][+-]?[0-9]+)$`)

var integerWidths = map[string]struct {
	bits     uint
	unsigned bool
}{
	"int": {bits: 64}, "int8": {bits: 8}, "int16": {bits: 16}, "int32": {bits: 32}, "int64": {bits: 64},
	"uint": {bits: 64, unsigned: true}, "uint8": {bits: 8, unsigned: true}, "uint16": {bits: 16, unsigned: true},
	"uint32": {bits: 32, unsigned: true}, "uint64": {bits: 64, unsigned: true},
}

func Classify(value string) Kind {
	if integerPattern.MatchString(value) {
		return Integer
	}
	if realPattern.MatchString(value) {
		return Real
	}
	return Invalid
}

func DefaultType(kind Kind) string {
	if kind == Real {
		return "float64"
	}
	return "int"
}

func IsType(name string) bool {
	if _, exists := integerWidths[name]; exists {
		return true
	}
	return name == "float32" || name == "float64" || name == "decimal"
}

func IsIntegerType(name string) bool {
	_, exists := integerWidths[name]
	return exists
}

func IsUnsignedType(name string) bool {
	value, exists := integerWidths[name]
	return exists && value.unsigned
}

func CheckLiteral(value, target string) error {
	kind := Classify(value)
	if kind == Invalid {
		return fmt.Errorf("%q is not a numeric literal", value)
	}
	if width, exists := integerWidths[target]; exists {
		if kind != Integer {
			return fmt.Errorf("%s requires an integer literal", target)
		}
		parsed, ok := new(big.Int).SetString(value, 10)
		if !ok {
			return fmt.Errorf("%q is not an integer literal", value)
		}
		if width.unsigned {
			maximum := new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), width.bits), big.NewInt(1))
			if parsed.Sign() < 0 || parsed.Cmp(maximum) > 0 {
				return fmt.Errorf("%s is outside the range of %s", value, target)
			}
			return nil
		}
		maximum := new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), width.bits-1), big.NewInt(1))
		minimum := new(big.Int).Neg(new(big.Int).Lsh(big.NewInt(1), width.bits-1))
		if parsed.Cmp(minimum) < 0 || parsed.Cmp(maximum) > 0 {
			return fmt.Errorf("%s is outside the range of %s", value, target)
		}
		return nil
	}
	if target == "float32" || target == "float64" {
		bits := 64
		if target == "float32" {
			bits = 32
		}
		parsed, err := strconv.ParseFloat(value, bits)
		if err != nil || math.IsInf(parsed, 0) {
			return fmt.Errorf("%s is outside the range of %s", value, target)
		}
		return nil
	}
	if target == "decimal" {
		if _, ok := new(big.Rat).SetString(value); !ok {
			return fmt.Errorf("%q is not an exact decimal literal", value)
		}
		return nil
	}
	return fmt.Errorf("%s is not a numeric type", target)
}
