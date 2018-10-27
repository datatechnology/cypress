package cypress

import (
	"errors"
	"reflect"
	"sync"
)

var (
	// ErrPointerRequired a pointer is required
	ErrPointerRequired = errors.New("a pointer is required")

	// ErrUnknownColumn no field to map the column
	ErrUnknownColumn = errors.New("don't know how to map the column")
)
var fieldNameCache = sync.Map{}

type smartMapper struct {
	value interface{}
}

// NewSmartMapper creates a smart row mapper for data row
func NewSmartMapper(value interface{}) RowMapper {
	return &smartMapper{value}
}

// Map maps the data row to a value object
func (mapper *smartMapper) Map(row DataRow) (interface{}, error) {
	columns, err := row.Columns()
	if err != nil {
		return nil, err
	}

	columnTypes, err := row.ColumnTypes()
	if err != nil {
		return nil, err
	}

	if len(columnTypes) == 1 {
		t := reflect.TypeOf(mapper.value)
		if t.Kind() == reflect.Ptr {
			t = t.Elem()
		}

		if t.Kind() != reflect.Struct {
			value := reflect.New(t)
			row.Scan(value.Interface())
			return value.Elem().Interface(), nil
		}
	}

	valueType := reflect.TypeOf(mapper.value)
	if valueType.Kind() != reflect.Ptr {
		return nil, ErrPointerRequired
	}

	valueType = valueType.Elem()
	typeID := valueType.PkgPath() + "/" + valueType.Name()
	value := reflect.New(valueType)
	entryValue, _ := fieldNameCache.LoadOrStore(typeID, &sync.Map{})
	nameCache := entryValue.(*sync.Map)
	values := make([]interface{}, len(columns))

	for index, name := range columns {
		var fieldName string
		entryValue, ok := nameCache.Load(name)
		if !ok {
			_, ok := valueType.FieldByName(name)
			if !ok {
				for i := 0; i < valueType.NumField(); i = i + 1 {
					f := valueType.Field(i)
					if name == f.Tag.Get("col") {
						fieldName = f.Name
						break
					}
				}
			} else {
				fieldName = name
			}
		} else {
			fieldName = entryValue.(string)
		}

		if fieldName == "" {
			return nil, ErrUnknownColumn
		}

		nameCache.Store(name, fieldName)
		fieldValue := value.Elem().FieldByName(fieldName)
		if fieldValue.Type().Kind() == reflect.Ptr {
			values[index] = fieldValue.Interface()
		} else {
			values[index] = fieldValue.Addr().Interface()
		}
	}

	row.Scan(values...)
	return value.Interface(), nil
}
