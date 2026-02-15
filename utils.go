// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

package main

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"unicode"
)

type APIProfile struct {
	Id   string `json:"id"`
	Name string `json:"name"`
}

var InvalidPlayer = errors.New("Invalid player")

var apiProfileCache = make(map[string]*APIProfile)

func getPlayerProfile(name string) (*APIProfile, error) {
	if apiProfile, ok := apiProfileCache[name]; ok {
		if apiProfile != nil {
			return apiProfile, nil
		}
	}
	resp, err := http.Get("https://api.mojang.com/users/profiles/minecraft/" + name)
	if err != nil || resp.StatusCode != 200 {
		apiProfileCache[name] = nil
		return nil, InvalidPlayer
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

	apiProfileCache[name] = &apiProfile

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

// returns: key, text, next price
func getUpgradeInformation(upgrade string, bedwarsType BedwarsType) (string, string, int) {
	if upgrade == "Sharpened Swords" {
		return "sharp", "Sharpened Swords", 0
	} else if strings.HasPrefix(upgrade, "Reinforced Armor") {
		if strings.HasSuffix(upgrade, " I") {
			if bedwarsType == BedwarsTypeSolo || bedwarsType == BedwarsTypeDoubles {
				return "prot", "Reinforced Armor 1", 4
			} else {
				return "prot", "Reinforced Armor 1", 10
			}
		} else if strings.HasSuffix(upgrade, " II") {
			if bedwarsType == BedwarsTypeSolo || bedwarsType == BedwarsTypeDoubles {
				return "prot", "Reinforced Armor 2", 8
			} else {
				return "prot", "Reinforced Armor 2", 20
			}
		} else if strings.HasSuffix(upgrade, " III") {
			if bedwarsType == BedwarsTypeSolo || bedwarsType == BedwarsTypeDoubles {
				return "prot", "Reinforced Armor 3", 16
			} else {
				return "prot", "Reinforced Armor 3", 30
			}
		} else if strings.HasSuffix(upgrade, " IV") {
			return "prot", "Reinforced Armor 4", 0
		}
	} else if strings.HasPrefix(upgrade, "Maniac Miner") {
		if strings.HasSuffix(upgrade, " I") {
			if bedwarsType == BedwarsTypeSolo || bedwarsType == BedwarsTypeDoubles {
				return "haste", "Maniac Miner 1", 4
			} else {
				return "haste", "Maniac Miner 1", 6
			}
		} else if strings.HasSuffix(upgrade, " II") {
			if bedwarsType == BedwarsTypeSolo || bedwarsType == BedwarsTypeDoubles {
				return "haste", "Maniac Miner 2", 0
			} else {
				return "haste", "Maniac Miner 2", 0
			}
		}
	} else if strings.HasSuffix(upgrade, "Forge") {
		if strings.HasPrefix(upgrade, "Iron") {
			if bedwarsType == BedwarsTypeSolo || bedwarsType == BedwarsTypeDoubles {
				return "forge", "Iron Forge", 4
			} else {
				return "forge", "Iron Forge", 8
			}
		} else if strings.HasPrefix(upgrade, "Gold") {
			if bedwarsType == BedwarsTypeSolo || bedwarsType == BedwarsTypeDoubles {
				return "forge", "Gold Forge", 6
			} else {
				return "forge", "Gold Forge", 12
			}
		} else if strings.HasPrefix(upgrade, "Emerald") {
			if bedwarsType == BedwarsTypeSolo || bedwarsType == BedwarsTypeDoubles {
				return "forge", "Emerald Forge", 8
			} else {
				return "forge", "Emerald Forge", 16
			}
		} else if strings.HasPrefix(upgrade, "Molten") {
			return "forge", "Molten Forge", 0
		}
	} else if upgrade == "Heal Pool" {
		return "healpool", "Heal Pool", 0
	} else if strings.HasPrefix(upgrade, "Cushioned Boots") {
		if strings.HasSuffix(upgrade, " I") {
			if bedwarsType == BedwarsTypeSolo || bedwarsType == BedwarsTypeDoubles {
				return "featherfalling", "Cushioned Boots 1", 2
			} else {
				return "featherfalling", "Cushioned Boots 1", 4
			}
		} else if strings.HasSuffix(upgrade, " II") {
			return "featherfalling", "Cushioned Boots 2", 0
		}
	}
	return "", "", 0
}
