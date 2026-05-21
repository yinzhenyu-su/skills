package daemon

import "encoding/json"

// unmarshalParams re-marshals interface{} params and unmarshals into a typed target.
func unmarshalParams(params interface{}, target interface{}) error {
	data, err := json.Marshal(params)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, target)
}
