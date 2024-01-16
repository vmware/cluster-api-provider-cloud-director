/*
   Copyright 2021 VMware, Inc.
   SPDX-License-Identifier: Apache-2.0
*/

package util

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/vmware/go-vcloud-director/v2/types/v56"
	"github.com/vmware/go-vcloud-director/v2/util"
)

// indentJSONBody indents raw JSON body for easier readability
func indentJSONBody(body []byte) ([]byte, error) {
	var prettyJSON bytes.Buffer
	err := json.Indent(&prettyJSON, body, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("error indenting response JSON: %s", err)
	}
	body = prettyJSON.Bytes()
	return body, nil
}

// DecodeXMLBody is used to decode a response body of types.BodyType
func DecodeXMLBody(bodyType types.BodyType, resp *http.Response, out interface{}) error {
	body, err := ioutil.ReadAll(resp.Body)

	// In case of JSON, body does not have indents in response therefore it must be indented
	if bodyType == types.BodyTypeJSON {
		body, err = indentJSONBody(body)
		if err != nil {
			return err
		}
	}

	util.ProcessResponseOutput(util.FuncNameCallStack(), resp, string(body))
	if err != nil {
		return err
	}

	// only attempt to unmarshal if body is not empty
	if len(body) > 0 {
		switch bodyType {
		case types.BodyTypeXML:
			if err = xml.Unmarshal(body, &out); err != nil {
				return err
			}
		case types.BodyTypeJSON:
			if err = json.Unmarshal(body, &out); err != nil {
				return err
			}

		default:
			panic(fmt.Sprintf("unknown body type: %d", bodyType))
		}
	}

	return nil
}

func Bool2BoolPtr(val bool) *bool {
	return &val
}

func Int2IntPtr(val int) *int {
	return &val
}

func Float2FloatPtr(val float64) *float64 {
	return &val
}

// Str2Bool returns true if the string value is not false
func Str2Bool(val string) bool {
	return strings.ToLower(val) == "true"
}
