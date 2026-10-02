package domain

import "encoding/json"

func jsonMarshal(value any) (string, error) {
	encoded, err := json.Marshal(value)
	return string(encoded), err
}
