// Copyright 2019 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package testing

import (
	"fmt"
	"reflect"
	"strings"
)

// Axis defines an axis for combining Params.
type Axis struct {
	// The name of the axis.
	Name string

	// The values.
	Values []interface{}
}

// NewAxis creates a new axis with the name and the values.
func NewAxis(name string, values ...interface{}) Axis {
	return Axis{Name: name, Values: values}
}

func getName(names []string, m map[string]interface{}) (string, error) {
	values := make([]string, 0, len(names)*2)
	for _, name := range names {
		value := m[name]
		var valueStr string
		if stringer, ok := value.(fmt.Stringer); ok {
			valueStr = stringer.String()
		} else {
			typ := reflect.TypeOf(value)
			if typ.Kind() >= reflect.Bool && typ.Kind() <= reflect.Complex128 {
				valueStr = fmt.Sprintf("%v", value)
			} else if typ.Kind() == reflect.String {
				valueStr = value.(string)
			} else {
				return "", fmt.Errorf("can't get name for %#v", value)
			}
		}
		values = append(values, strings.ToLower(name), strings.ToLower(valueStr))
	}
	return strings.Join(values, "_"), nil
}

// Combine combines multiple axises and creates a list of Param. Each Param
// will have a slice of values picked up from the specified data and the name
// generated from the slice of values.
func Combine(axises ...Axis) []Param {
	values := []map[string]interface{}{map[string]interface{}{}}
	names := make([]string, 0, len(axises))
	for _, axis := range axises {
		if len(axis.Values) == 0 {
			continue
		}

		newValues := make([]map[string]interface{}, 0, len(values))
		for _, vs := range values {
			for _, i := range axis.Values {
				nvs := make(map[string]interface{}, len(vs))
				for k, v := range vs {
					nvs[k] = v
				}
				nvs[axis.Name] = i
				newValues = append(newValues, nvs)
			}
		}
		values = newValues
		names = append(names, axis.Name)
	}
	results := make([]Param, 0, len(values))
	for i, vs := range values {
		name, err := getName(names, vs)
		if err != nil {
			name = fmt.Sprintf("%d", i)
		}
		results = append(results, Param{Name: name, Val: vs})
	}
	return results
}
