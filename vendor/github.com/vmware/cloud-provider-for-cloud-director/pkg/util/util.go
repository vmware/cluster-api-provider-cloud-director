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
	passwordgenerator "github.com/sethvargo/go-password/password"
	"github.com/vmware/go-vcloud-director/v2/types/v56"
	"github.com/vmware/go-vcloud-director/v2/util"
	"io/ioutil"
	"net/http"
)

const (
	VCDLowerLetters = `abcdefghijkmnopqrstuvwxyz`
	VCDUpperLetters = `ABCDEFGHIJKLMNPQRSTUVWXYZ`
	VCDDigits       = `23456789`
	VCDSymbols      = `!$#%`
)

// indentJsonBody indents raw JSON body for easier readability
func indentJsonBody(body []byte) ([]byte, error) {
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
		body, err = indentJsonBody(body)
		if err != nil {
			return err
		}
	}

	util.ProcessResponseOutput(util.FuncNameCallStack(), resp, fmt.Sprintf("%s", body))
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

func GeneratePassword(length int, numDigits int, numSymbols int, noUpper bool, allowRepeat bool) (string, error) {

	// these are the inputs used by VMware Cloud Director
	generatorInput := passwordgenerator.GeneratorInput{
		LowerLetters: VCDLowerLetters,
		UpperLetters: VCDUpperLetters,
		Digits:       VCDDigits,
		Symbols:      VCDSymbols,
		Reader:       nil,
	}

	passwordGenerator, err := passwordgenerator.NewGenerator(&generatorInput)
	if err != nil {
		return "", fmt.Errorf("unable to create password generator with character-set [%v]: [%v]",
			generatorInput, err)
	}

	passwd, err := passwordGenerator.Generate(length, numDigits, numSymbols, noUpper, allowRepeat)
	if err != nil {
		return "", fmt.Errorf("failed to generate a password with character-set [%v]: [%v]",
			generatorInput, err)
	}

	return passwd, nil
}
