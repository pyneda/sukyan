package core

import (
	"testing"
)

func intPtr(v int) *int       { return &v }
func floatPtr(v float64) *float64 { return &v }

func TestDataType_IsNumeric(t *testing.T) {
	tests := []struct {
		dt   DataType
		want bool
	}{
		{DataTypeInteger, true},
		{DataTypeNumber, true},
		{DataTypeString, false},
		{DataTypeBoolean, false},
		{DataTypeArray, false},
		{DataTypeObject, false},
		{DataTypeFile, false},
	}
	for _, tt := range tests {
		t.Run(string(tt.dt), func(t *testing.T) {
			if got := tt.dt.IsNumeric(); got != tt.want {
				t.Errorf("DataType(%q).IsNumeric() = %v, want %v", tt.dt, got, tt.want)
			}
		})
	}
}

func TestDataType_IsString(t *testing.T) {
	tests := []struct {
		dt   DataType
		want bool
	}{
		{DataTypeString, true},
		{DataTypeInteger, false},
		{DataTypeNumber, false},
		{DataTypeBoolean, false},
	}
	for _, tt := range tests {
		t.Run(string(tt.dt), func(t *testing.T) {
			if got := tt.dt.IsString(); got != tt.want {
				t.Errorf("DataType(%q).IsString() = %v, want %v", tt.dt, got, tt.want)
			}
		})
	}
}

func TestParameter_GetEffectiveValue(t *testing.T) {
	tests := []struct {
		name  string
		param Parameter
		want  any
	}{
		{
			name: "returns example value when set",
			param: Parameter{
				Name:         "test",
				DataType:     DataTypeString,
				ExampleValue: "example",
				DefaultValue: "default",
			},
			want: "example",
		},
		{
			name: "returns default value when no example",
			param: Parameter{
				Name:         "test",
				DataType:     DataTypeString,
				DefaultValue: "default",
			},
			want: "default",
		},
		{
			name: "generates value when neither set - string",
			param: Parameter{
				Name:     "test",
				DataType: DataTypeString,
			},
			want: "test",
		},
		{
			name: "generates value when neither set - integer",
			param: Parameter{
				Name:     "count",
				DataType: DataTypeInteger,
			},
			want: 1,
		},
		{
			name: "generates value when neither set - boolean",
			param: Parameter{
				Name:     "flag",
				DataType: DataTypeBoolean,
			},
			want: true,
		},
		{
			name: "example value takes priority over default for integer",
			param: Parameter{
				Name:         "id",
				DataType:     DataTypeInteger,
				ExampleValue: 42,
				DefaultValue: 10,
			},
			want: 42,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.param.GetEffectiveValue()
			if got != tt.want {
				t.Errorf("GetEffectiveValue() = %v (%T), want %v (%T)", got, got, tt.want, tt.want)
			}
		})
	}
}

func TestParameter_GenerateDefaultValue_String(t *testing.T) {
	tests := []struct {
		name  string
		param Parameter
		want  any
	}{
		{
			name: "plain string",
			param: Parameter{
				Name:     "name",
				DataType: DataTypeString,
			},
			want: "test",
		},
		{
			name: "enum picks first value",
			param: Parameter{
				Name:     "status",
				DataType: DataTypeString,
				Constraints: Constraints{
					Enum: []any{"active", "inactive"},
				},
			},
			want: "active",
		},
		{
			name: "email format",
			param: Parameter{
				Name:     "email",
				DataType: DataTypeString,
				Constraints: Constraints{
					Format: "email",
				},
			},
			want: "test@example.com",
		},
		{
			name: "uri format",
			param: Parameter{
				Name:     "website",
				DataType: DataTypeString,
				Constraints: Constraints{
					Format: "uri",
				},
			},
			want: "https://example.com",
		},
		{
			name: "url format",
			param: Parameter{
				Name:     "callback",
				DataType: DataTypeString,
				Constraints: Constraints{
					Format: "url",
				},
			},
			want: "https://example.com",
		},
		{
			name: "uuid format",
			param: Parameter{
				Name:     "id",
				DataType: DataTypeString,
				Constraints: Constraints{
					Format: "uuid",
				},
			},
			want: "00000000-0000-0000-0000-000000000001",
		},
		{
			name: "date format",
			param: Parameter{
				Name:     "start_date",
				DataType: DataTypeString,
				Constraints: Constraints{
					Format: "date",
				},
			},
			want: "2024-01-01",
		},
		{
			name: "date-time format",
			param: Parameter{
				Name:     "timestamp",
				DataType: DataTypeString,
				Constraints: Constraints{
					Format: "date-time",
				},
			},
			want: "2024-01-01T00:00:00Z",
		},
		{
			name: "ipv4 format",
			param: Parameter{
				Name:     "ip",
				DataType: DataTypeString,
				Constraints: Constraints{
					Format: "ipv4",
				},
			},
			want: "127.0.0.1",
		},
		{
			name: "ipv6 format",
			param: Parameter{
				Name:     "ip6",
				DataType: DataTypeString,
				Constraints: Constraints{
					Format: "ipv6",
				},
			},
			want: "::1",
		},
		{
			name: "min length generates padded string",
			param: Parameter{
				Name:     "code",
				DataType: DataTypeString,
				Constraints: Constraints{
					MinLength: intPtr(5),
				},
			},
			want: "aaaaa",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.param.GenerateDefaultValue()
			if got != tt.want {
				t.Errorf("GenerateDefaultValue() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParameter_GenerateDefaultValue_Integer(t *testing.T) {
	tests := []struct {
		name  string
		param Parameter
		want  any
	}{
		{
			name: "plain integer",
			param: Parameter{
				Name:     "count",
				DataType: DataTypeInteger,
			},
			want: 1,
		},
		{
			name: "enum picks first value",
			param: Parameter{
				Name:     "level",
				DataType: DataTypeInteger,
				Constraints: Constraints{
					Enum: []any{10, 20, 30},
				},
			},
			want: 10,
		},
		{
			name: "respects minimum",
			param: Parameter{
				Name:     "page",
				DataType: DataTypeInteger,
				Constraints: Constraints{
					Minimum: floatPtr(5),
				},
			},
			want: int(5),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.param.GenerateDefaultValue()
			if got != tt.want {
				t.Errorf("GenerateDefaultValue() = %v (%T), want %v (%T)", got, got, tt.want, tt.want)
			}
		})
	}
}

func TestParameter_GenerateDefaultValue_Number(t *testing.T) {
	tests := []struct {
		name  string
		param Parameter
		want  any
	}{
		{
			name: "plain number",
			param: Parameter{
				Name:     "price",
				DataType: DataTypeNumber,
			},
			want: 1.0,
		},
		{
			name: "enum picks first value",
			param: Parameter{
				Name:     "rate",
				DataType: DataTypeNumber,
				Constraints: Constraints{
					Enum: []any{1.5, 2.5},
				},
			},
			want: 1.5,
		},
		{
			name: "respects minimum",
			param: Parameter{
				Name:     "weight",
				DataType: DataTypeNumber,
				Constraints: Constraints{
					Minimum: floatPtr(0.5),
				},
			},
			want: 0.5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.param.GenerateDefaultValue()
			if got != tt.want {
				t.Errorf("GenerateDefaultValue() = %v (%T), want %v (%T)", got, got, tt.want, tt.want)
			}
		})
	}
}

func TestParameter_GenerateDefaultValue_Boolean(t *testing.T) {
	param := Parameter{
		Name:     "enabled",
		DataType: DataTypeBoolean,
	}
	got := param.GenerateDefaultValue()
	if got != true {
		t.Errorf("GenerateDefaultValue() for boolean = %v, want true", got)
	}
}

func TestParameter_GenerateDefaultValue_Array(t *testing.T) {
	t.Run("empty array when no nested params", func(t *testing.T) {
		param := Parameter{
			Name:     "tags",
			DataType: DataTypeArray,
		}
		got := param.GenerateDefaultValue()
		arr, ok := got.([]any)
		if !ok {
			t.Fatalf("GenerateDefaultValue() returned %T, want []any", got)
		}
		if len(arr) != 0 {
			t.Errorf("GenerateDefaultValue() returned array of length %d, want 0", len(arr))
		}
	})

	t.Run("array with nested param value", func(t *testing.T) {
		param := Parameter{
			Name:     "ids",
			DataType: DataTypeArray,
			NestedParams: []Parameter{
				{Name: "item", DataType: DataTypeInteger},
			},
		}
		got := param.GenerateDefaultValue()
		arr, ok := got.([]any)
		if !ok {
			t.Fatalf("GenerateDefaultValue() returned %T, want []any", got)
		}
		if len(arr) != 1 {
			t.Fatalf("GenerateDefaultValue() returned array of length %d, want 1", len(arr))
		}
		if arr[0] != 1 {
			t.Errorf("GenerateDefaultValue() array[0] = %v, want 1", arr[0])
		}
	})
}

func TestParameter_GenerateDefaultValue_Object(t *testing.T) {
	t.Run("empty object when no nested params", func(t *testing.T) {
		param := Parameter{
			Name:     "metadata",
			DataType: DataTypeObject,
		}
		got := param.GenerateDefaultValue()
		obj, ok := got.(map[string]any)
		if !ok {
			t.Fatalf("GenerateDefaultValue() returned %T, want map[string]any", got)
		}
		if len(obj) != 0 {
			t.Errorf("GenerateDefaultValue() returned object with %d keys, want 0", len(obj))
		}
	})

	t.Run("object with nested params", func(t *testing.T) {
		param := Parameter{
			Name:     "user",
			DataType: DataTypeObject,
			NestedParams: []Parameter{
				{Name: "name", DataType: DataTypeString},
				{Name: "age", DataType: DataTypeInteger},
			},
		}
		got := param.GenerateDefaultValue()
		obj, ok := got.(map[string]any)
		if !ok {
			t.Fatalf("GenerateDefaultValue() returned %T, want map[string]any", got)
		}
		if len(obj) != 2 {
			t.Fatalf("GenerateDefaultValue() returned object with %d keys, want 2", len(obj))
		}
		if obj["name"] != "test" {
			t.Errorf("obj[name] = %v, want 'test'", obj["name"])
		}
		if obj["age"] != 1 {
			t.Errorf("obj[age] = %v, want 1", obj["age"])
		}
	})
}

func TestParameter_GenerateDefaultValue_UnknownType(t *testing.T) {
	param := Parameter{
		Name:     "unknown",
		DataType: DataType("custom"),
	}
	got := param.GenerateDefaultValue()
	if got != nil {
		t.Errorf("GenerateDefaultValue() for unknown type = %v, want nil", got)
	}
}

func TestParameter_GenerateDefaultValue_FileType(t *testing.T) {
	param := Parameter{
		Name:     "upload",
		DataType: DataTypeFile,
	}
	got := param.GenerateDefaultValue()
	if got != nil {
		t.Errorf("GenerateDefaultValue() for file type = %v, want nil", got)
	}
}

func TestParameter_GenerateDefaultValue_NestedDepthLimit(t *testing.T) {
	deeplyNested := Parameter{Name: "leaf", DataType: DataTypeString}
	for i := 0; i < 12; i++ {
		deeplyNested = Parameter{
			Name:         "level",
			DataType:     DataTypeObject,
			NestedParams: []Parameter{deeplyNested},
		}
	}

	got := deeplyNested.GenerateDefaultValue()
	if got == nil {
		t.Fatal("GenerateDefaultValue() for deeply nested object returned nil at top level")
	}

	current := got
	depth := 0
	for {
		obj, ok := current.(map[string]any)
		if !ok {
			break
		}
		val, exists := obj["level"]
		if !exists {
			val = obj["leaf"]
			if val != nil {
				depth++
			}
			break
		}
		depth++
		current = val
	}

	if depth > maxNestedDepth+1 {
		t.Errorf("nesting went %d levels deep, expected capping at maxNestedDepth (%d)", depth, maxNestedDepth)
	}
}
