package storage

import (
	"encoding/binary"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// ColumnType определяет тип колонки
type ColumnType int

const (
	TypeInt ColumnType = iota
	TypeFloat
	TypeText
	TypeBool
)

// ColumnDef — определение колонки таблицы
type ColumnDef struct {
    Name          string
    ColType       ColumnType
    Nullable      bool
    PrimaryKey    bool
    AutoIncrement bool
    Unique        bool
    Default       string
    Check         string
    References    string
}

// TableSchema — схема таблицы
type TableSchema struct {
	Name    string
	Columns []ColumnDef
}

// Row представляет строку данных
type Row struct {
	RID    RID
	Values []interface{}
}

func SerializeRow(row *Row, schema *TableSchema) ([]byte, error) {
	if len(row.Values) != len(schema.Columns) {
		return nil, fmt.Errorf("несоответствие числа колонок: %d vs %d", len(row.Values), len(schema.Columns))
	}

	nullBitmapSize := (len(schema.Columns) + 7) / 8
	nullBitmap := make([]byte, nullBitmapSize)

	fixedPart := make([]byte, 0)
	varData := make([]byte, 0)
	varOffsets := make([]uint16, len(schema.Columns))
	currentVarOffset := uint16(0)

	for i, col := range schema.Columns {
		isNull := row.Values[i] == nil
		if isNull {
			byteIdx := i / 8
			bitIdx := i % 8
			nullBitmap[byteIdx] |= (1 << bitIdx)
			fixedPart = append(fixedPart, make([]byte, colFixedSize(col))...)
			varOffsets[i] = 0
			continue
		}

		switch col.ColType {
		case TypeInt:
			val := toInt32(row.Values[i])
			b := make([]byte, 4)
			binary.LittleEndian.PutUint32(b, uint32(val))
			fixedPart = append(fixedPart, b...)
			varOffsets[i] = 0

		case TypeFloat:
			val := float64(0)
			switch v := row.Values[i].(type) {
			case float64:
				val = v
			case float32:
				val = float64(v)
			case int:
				val = float64(v)
			}
			b := make([]byte, 8)
			binary.LittleEndian.PutUint64(b, math.Float64bits(val))
			fixedPart = append(fixedPart, b...)
			varOffsets[i] = 0

		case TypeBool:
			val := false
			if b, ok := row.Values[i].(bool); ok {
				val = b
			}
			if val {
				fixedPart = append(fixedPart, 1)
			} else {
				fixedPart = append(fixedPart, 0)
			}
			varOffsets[i] = 0

		case TypeText:
			str := fmt.Sprintf("%v", row.Values[i])
			strBytes := []byte(str)

			b := make([]byte, 2)
			binary.LittleEndian.PutUint16(b, currentVarOffset)
			fixedPart = append(fixedPart, b...)

			varOffsets[i] = currentVarOffset
			currentVarOffset += uint16(2 + len(strBytes))

			lenB := make([]byte, 2)
			binary.LittleEndian.PutUint16(lenB, uint16(len(strBytes)))
			varData = append(varData, lenB...)
			varData = append(varData, strBytes...)
		}
	}

	result := make([]byte, 0, nullBitmapSize+len(fixedPart)+len(schema.Columns)*2+len(varData))
	result = append(result, nullBitmap...)
	result = append(result, fixedPart...)

	for i := 0; i < len(schema.Columns); i++ {
		b := make([]byte, 2)
		binary.LittleEndian.PutUint16(b, varOffsets[i])
		result = append(result, b...)
	}

	result = append(result, varData...)
	return result, nil
}

func DeserializeRow(data []byte, schema *TableSchema) (*Row, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("пустые данные строки")
	}

	pos := 0

	nullBitmapSize := (len(schema.Columns) + 7) / 8
	if pos+nullBitmapSize > len(data) {
		return nil, fmt.Errorf("недостаточно данных для null bitmap")
	}
	nullBitmap := data[pos : pos+nullBitmapSize]
	pos += nullBitmapSize

	values := make([]interface{}, len(schema.Columns))
	varOffsets := make(map[int]uint16)

	for i, col := range schema.Columns {
		byteIdx := i / 8
		bitIdx := i % 8
		isNull := byteIdx < len(nullBitmap) && (nullBitmap[byteIdx]&(1<<bitIdx)) != 0

		if isNull {
			values[i] = nil
			pos += colFixedSize(col)
			continue
		}

		switch col.ColType {
		case TypeInt:
			if pos+4 > len(data) {
				return nil, fmt.Errorf("недостаточно данных для INT в колонке %d", i)
			}
			values[i] = int32(binary.LittleEndian.Uint32(data[pos : pos+4]))
			pos += 4

		case TypeFloat:
			if pos+8 > len(data) {
				return nil, fmt.Errorf("недостаточно данных для FLOAT в колонке %d", i)
			}
			bits := binary.LittleEndian.Uint64(data[pos : pos+8])
			values[i] = math.Float64frombits(bits)
			pos += 8

		case TypeBool:
			if pos+1 > len(data) {
				return nil, fmt.Errorf("недостаточно данных для BOOL в колонке %d", i)
			}
			values[i] = data[pos] == 1
			pos += 1

		case TypeText:
			if pos+2 > len(data) {
				return nil, fmt.Errorf("недостаточно данных для TEXT в колонке %d", i)
			}
			offset := binary.LittleEndian.Uint16(data[pos : pos+2])
			varOffsets[i] = offset
			pos += 2
		}
	}

	varDataPos := pos + len(schema.Columns)*2

	for colIdx, offset := range varOffsets {
		if schema.Columns[colIdx].ColType != TypeText {
			continue
		}

		dataStart := varDataPos + int(offset)
		if dataStart+2 > len(data) {
			continue
		}

		strLen := binary.LittleEndian.Uint16(data[dataStart : dataStart+2])
		dataStart += 2

		if dataStart+int(strLen) > len(data) {
			continue
		}

		values[colIdx] = string(data[dataStart : dataStart+int(strLen)])
	}

	return &Row{Values: values}, nil
}

func toInt32(v interface{}) int32 {
	switch val := v.(type) {
	case int:
		return int32(val)
	case int32:
		return val
	case float64:
		return int32(val)
	default:
		return 0
	}
}

func colFixedSize(col ColumnDef) int {
	switch col.ColType {
	case TypeInt:
		return 4
	case TypeFloat:
		return 8
	case TypeBool:
		return 1
	case TypeText:
		return 2
	default:
		return 0
	}
}

// Helper: парсинг строки в значение нужного типа
func ParseValue(str string, colType ColumnType) (interface{}, error) {
	switch colType {
	case TypeInt:
		val, err := strconv.Atoi(str)
		return int32(val), err
	case TypeFloat:
		val, err := strconv.ParseFloat(str, 64)
		return val, err
	case TypeBool:
		lower := strings.ToLower(str)
		if lower == "true" || lower == "1" || lower == "t" {
			return true, nil
		}
		if lower == "false" || lower == "0" || lower == "f" {
			return false, nil
		}
		return nil, fmt.Errorf("неверное булево значение: %s", str)
	case TypeText:
		return str, nil
	default:
		return nil, fmt.Errorf("неизвестный тип колонки")
	}
}
