// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

package main

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"unicode"
)

type APIProfile struct {
	Id   string `json:"id"`
	Name string `json:"name"`
}

func getPlayerUuid(name string) (*APIProfile, error) {
	resp, err := http.Get("https://api.mojang.com/users/profiles/minecraft/" + name)
	if err != nil || resp.StatusCode != 200 {
		return nil, errors.New("Invalid player")
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	apiProfile := APIProfile{}
	err = json.Unmarshal(body, &apiProfile)
	if err != nil {
		return nil, err
	}

	return &apiProfile, nil
}

func capitaliseFirst(s string) string {
	if s == "" {
		return s
	}
	runes := []rune(s)
	if !unicode.IsLetter(runes[0]) {
		return s
	}
	runes[0] = unicode.ToUpper(runes[0])
	return string(runes)
}
