package storage

import (
	"encoding/binary"
	"fmt"
	"testing"
)

func TestSerializeDeserialize(t *testing.T) {
	schema := &TableSchema{
		Name: "test",
		Columns: []ColumnDef{
			{Name: "id", ColType: TypeInt},
			{Name: "name", ColType: TypeText},
			{Name: "age", ColType: TypeInt},
			{Name: "active", ColType: TypeBool},
		},
	}

	original := &Row{
		Values: []interface{}{int32(42), "Alice", int32(30), true},
	}

	data, err := SerializeRow(original, schema)
	if err != nil {
		t.Fatalf("SerializeRow: %v", err)
	}

	// Выводим байты для отладки
	fmt.Printf("Всего байт: %d\n", len(data))
	fmt.Printf("Raw bytes: %v\n", data[:min(50, len(data))])
	
	// Поясним структуру
	nullBitmapSize := (len(schema.Columns) + 7) / 8
	fmt.Printf("Null bitmap (%d байт): %08b\n", nullBitmapSize, data[0:nullBitmapSize])
	
	pos := nullBitmapSize
	fmt.Printf("Pos after null bitmap: %d\n", pos)
	
	// INT id = 4 байта
	fmt.Printf("INT id (4 байта): %v = %d\n", data[pos:pos+4], int32(binary.LittleEndian.Uint32(data[pos:pos+4])))
	pos += 4
	
	// TEXT offset = 2 байта
	textOffset := binary.LittleEndian.Uint16(data[pos : pos+2])
	fmt.Printf("TEXT offset (2 байта): %v = %d\n", data[pos:pos+2], textOffset)
	pos += 2
	
	// INT age = 4 байта
	fmt.Printf("INT age (4 байта): %v = %d\n", data[pos:pos+4], int32(binary.LittleEndian.Uint32(data[pos:pos+4])))
	pos += 4
	
	// BOOL active = 1 байт
	fmt.Printf("BOOL active (1 байт): %v = %v\n", data[pos:pos+1], data[pos] == 1)
	pos += 1
	
	// Var offsets table (4 колонки * 2 байта = 8 байт)
	fmt.Printf("Var offsets table (8 байт): %v\n", data[pos:pos+8])
	for i := 0; i < 4; i++ {
		off := binary.LittleEndian.Uint16(data[pos+i*2 : pos+i*2+2])
		fmt.Printf("  var_offset[%d] = %d\n", i, off)
	}
	pos += 4 * 2
	
	// Var data
	fmt.Printf("Var data starts at: %d\n", pos)
	fmt.Printf("Var data: %v\n", data[pos:])
	
	// Парсим var data вручную
	if pos < len(data) {
		strLen := binary.LittleEndian.Uint16(data[pos : pos+2])
		fmt.Printf("TEXT length: %d\n", strLen)
		fmt.Printf("TEXT value: %s\n", string(data[pos+2:pos+2+int(strLen)]))
	}

	if len(data) == 0 {
		t.Fatal("Сериализованные данные пусты")
	}

	restored, err := DeserializeRow(data, schema)
	if err != nil {
		t.Fatalf("DeserializeRow: %v", err)
	}

	fmt.Printf("Исходные: %v\n", original.Values)
	fmt.Printf("Восстановленные: %v\n", restored.Values)

	if fmt.Sprint(restored.Values[0]) != "42" {
		t.Errorf("id: ожидалось 42, получено %v", restored.Values[0])
	}
	if restored.Values[1] != "Alice" {
		t.Errorf("name: ожидалось 'Alice', получено '%v'", restored.Values[1])
	}
	if fmt.Sprint(restored.Values[2]) != "30" {
		t.Errorf("age: ожидалось 30, получено %v", restored.Values[2])
	}
	if restored.Values[3] != true {
		t.Errorf("active: ожидалось true, получено %v", restored.Values[3])
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func TestSerializeNullValues(t *testing.T) {
	schema := &TableSchema{
		Name: "test",
		Columns: []ColumnDef{
			{Name: "id", ColType: TypeInt},
			{Name: "name", ColType: TypeText, Nullable: true},
			{Name: "age", ColType: TypeInt, Nullable: true},
		},
	}

	original := &Row{
		Values: []interface{}{int32(1), nil, nil},
	}

	data, err := SerializeRow(original, schema)
	if err != nil {
		t.Fatalf("SerializeRow: %v", err)
	}

	fmt.Printf("Сериализовано (с null): %d байт\n", len(data))

	restored, err := DeserializeRow(data, schema)
	if err != nil {
		t.Fatalf("DeserializeRow: %v", err)
	}

	fmt.Printf("Исходные (с null): %v\n", original.Values)
	fmt.Printf("Восстановленные (с null): %v\n", restored.Values)

	if fmt.Sprint(restored.Values[0]) != "1" {
		t.Errorf("id должен быть 1, получено %v", restored.Values[0])
	}
	if restored.Values[1] != nil {
		t.Errorf("name должен быть nil, получено %v", restored.Values[1])
	}
	if restored.Values[2] != nil {
		t.Errorf("age должен быть nil, получено %v", restored.Values[2])
	}
}

func TestSerializeTextTypes(t *testing.T) {
	schema := &TableSchema{
		Name: "test",
		Columns: []ColumnDef{
			{Name: "description", ColType: TypeText},
		},
	}

	longText := "Это очень длинный текст, который должен корректно сериализоваться и десериализоваться, " +
		"включая пробелы, цифры 12345 и специальные символы !@#$%^&*()"

	original := &Row{
		Values: []interface{}{longText},
	}

	data, err := SerializeRow(original, schema)
	if err != nil {
		t.Fatalf("SerializeRow: %v", err)
	}

	restored, err := DeserializeRow(data, schema)
	if err != nil {
		t.Fatalf("DeserializeRow: %v", err)
	}

	if restored.Values[0] != longText {
		t.Errorf("Тексты не совпадают")
	}
}

func TestSerializeRowCountMismatch(t *testing.T) {
	schema := &TableSchema{
		Name: "test",
		Columns: []ColumnDef{
			{Name: "id", ColType: TypeInt},
		},
	}

	row := &Row{
		Values: []interface{}{1, 2, 3}, // больше значений чем колонок
	}

	_, err := SerializeRow(row, schema)
	if err == nil {
		t.Error("Ожидалась ошибка несоответствия числа колонок")
	}
}