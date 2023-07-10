package db

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
)

type StringSlice []string

func (c *StringSlice) Scan(value interface{}) error {
	b, ok := value.([]byte)
	if !ok {
		return errors.New("type assertion to []byte failed")
	}
	return json.Unmarshal(b, &c)
}

func (c StringSlice) Value() (driver.Value, error) {
	return json.Marshal(c)
}

type IntSlice []int

func (c *IntSlice) Scan(value interface{}) error {
	b, ok := value.([]byte)
	if !ok {
		return errors.New("type assertion to []byte failed")
	}
	return json.Unmarshal(b, &c)
}

func (c IntSlice) Value() (driver.Value, error) {
	return json.Marshal(c)
}
