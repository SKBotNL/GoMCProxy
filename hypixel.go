// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

package main

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
)

type Locraw struct {
	Server   string `json:"server"`
	GameType string `json:"gametype"`
	Mode     string `json:"mode"`
}

type Hypixel struct {
	apiKey string
}

func newHypixel(apiKey string) *Hypixel {
	return &Hypixel{apiKey}
}

// True if valid API key
func (h *Hypixel) testKey() (bool, error) {
	req, err := http.NewRequest("GET", "https://api.hypixel.net/v2/player?uuid=0", nil)
	if err != nil {
		return false, err
	}

	req.Header.Add("API-Key", h.apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, err
	}

	if resp.StatusCode != 422 {
		return false, nil
	}
	return true, nil
}

type PlayerStats struct {
	Success bool `json:"success"`
	Player  struct {
		Achievements struct {
			BedwarsLevel int `json:"bedwars_level"`
		} `json:"achievements"`
		Stats struct {
			Bedwars struct {
				// Solo
				EightOneKillsBedwars       int `json:"eight_one_kills_bedwars"`
				EightOneDeathsBedwars      int `json:"eight_one_deaths_bedwars"`
				EightOneFinalKillsBedwars  int `json:"eight_one_final_kills_bedwars"`
				EightOneFinalDeathsBedwars int `json:"eight_one_final_deaths_bedwars"`
				EightOneWinsBedwars        int `json:"eight_one_wins_bedwars"`
				EightOneLossesBedwars      int `json:"eight_one_losses_bedwars"`
				EightOneWinstreak          int `json:"eight_one_winstreak"`
				EightOneBedsBroken         int `json:"eight_one_beds_broken_bedwars"`

				// Doubles
				EightTwoKillsBedwars       int `json:"eight_two_kills_bedwars"`
				EightTwoDeathsBedwars      int `json:"eight_two_deaths_bedwars"`
				EightTwoFinalKillsBedwars  int `json:"eight_two_final_kills_bedwars"`
				EightTwoFinalDeathsBedwars int `json:"eight_two_final_deaths_bedwars"`
				EightTwoWinsBedwars        int `json:"eight_two_wins_bedwars"`
				EightTwoLossesBedwars      int `json:"eight_two_losses_bedwars"`
				EightTwoWinstreak          int `json:"eight_two_winstreak"`
				EightTwoBedsBroken         int `json:"eight_two_beds_broken_bedwars"`

				// 3v3v3v3
				FourThreeKillsBedwars       int `json:"four_three_kills_bedwars"`
				FourThreeDeathsBedwars      int `json:"four_three_deaths_bedwars"`
				FourThreeFinalKillsBedwars  int `json:"four_three_final_kills_bedwars"`
				FourThreeFinalDeathsBedwars int `json:"four_three_final_deaths_bedwars"`
				FourThreeWinsBedwars        int `json:"four_three_wins_bedwars"`
				FourThreeLossesBedwars      int `json:"four_three_losses_bedwars"`
				FourThreeWinstreak          int `json:"four_three_winstreak"`
				FourThreeBedsBroken         int `json:"four_three_beds_broken_bedwars"`

				// 4v4v4v4
				FourFourKillsBedwars       int `json:"four_four_kills_bedwars"`
				FourFourDeathsBedwars      int `json:"four_four_deaths_bedwars"`
				FourFourFinalKillsBedwars  int `json:"four_four_final_kills_bedwars"`
				FourFourFinalDeathsBedwars int `json:"four_four_final_deaths_bedwars"`
				FourFourWinsBedwars        int `json:"four_four_wins_bedwars"`
				FourFourLossesBedwars      int `json:"four_four_losses_bedwars"`
				FourFourWinstreak          int `json:"four_four_winstreak"`
				FourFourBedsBroken         int `json:"four_four_beds_broken_bedwars"`

				// 4v4
				TwoFourKillsBedwars       int `json:"two_four_kills_bedwars"`
				TwoFourDeathsBedwars      int `json:"two_four_deaths_bedwars"`
				TwoFourFinalKillsBedwars  int `json:"two_four_final_kills_bedwars"`
				TwoFourFinalDeathsBedwars int `json:"two_four_final_deaths_bedwars"`
				TwoFourWinsBedwars        int `json:"two_four_wins_bedwars"`
				TwoFourLossesBedwars      int `json:"two_four_losses_bedwars"`
				TwoFourWinstreak          int `json:"two_four_winstreak"`
				TwoFourBedsBroken         int `json:"two_four_beds_broken_bedwars"`
			} `json:"Bedwars"`
		} `json:"stats"`
	} `json:"player"`
}

type BedwarsType string

const (
	BedwarsTypeSolo    BedwarsType = "solo"
	BedwarsTypeDoubles BedwarsType = "doubles"
	BedwarsType3v3v3v3 BedwarsType = "3v3v3v3"
	BedwarsType4v4v4v4 BedwarsType = "4v4v4v4"
	BedwarsType4v4     BedwarsType = "4v4"
)

var bedwarsTypeStrings = map[string]BedwarsType{
	"solo":               BedwarsTypeSolo,
	"doubles":            BedwarsTypeDoubles,
	"3v3v3v3":            BedwarsType3v3v3v3,
	"4v4v4v4":            BedwarsType4v4v4v4,
	"4v4":                BedwarsType4v4,
	"BEDWARS_EIGHT_ONE":  BedwarsTypeSolo,
	"BEDWARS_EIGHT_TWO":  BedwarsTypeDoubles,
	"BEDWARS_FOUR_THREE": BedwarsType3v3v3v3,
	"BEDWARS_FOUR_FOUR":  BedwarsType4v4v4v4,
	"BEDWARS_TWO_FOUR":   BedwarsType4v4,
}

type BedwarsStats struct {
	Stars       int
	Kills       int
	Deaths      int
	KD          float32
	FinalKills  int
	FinalDeaths int
	FinalKD     float32
	Wins        int
	Losses      int
	WL          float32
	Winstreak   int
	BedsBroken  int
}

func GetBedwarsType(s string) (BedwarsType, bool) {
	bedwarsType, ok := bedwarsTypeStrings[s]
	return bedwarsType, ok
}

func (h *Hypixel) getPlayerStats(uuid string) (*PlayerStats, error) {
	params := url.Values{}
	params.Add("uuid", uuid)

	req, err := http.NewRequest("GET", "https://api.hypixel.net/v2/player"+"?"+params.Encode(), nil)
	if err != nil {
		return nil, err
	}

	req.Header.Add("API-Key", h.apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != 200 {
		return nil, errors.New("Bad response")
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	playerStats := PlayerStats{}
	err = json.Unmarshal(body, &playerStats)
	if err != nil {
		return nil, err
	}

	return &playerStats, nil
}

func (h *Hypixel) getBedwarsStats(uuid string, bedwarsType BedwarsType) (*BedwarsStats, error) {
	playerStats, err := h.getPlayerStats(uuid)
	if err != nil {
		return nil, err
	}

	switch bedwarsType {
	case BedwarsTypeSolo:
		statsBedwars := playerStats.Player.Stats.Bedwars
		KD := float32(statsBedwars.EightOneKillsBedwars) / float32(statsBedwars.EightOneDeathsBedwars)
		FinalKD := float32(statsBedwars.EightOneFinalKillsBedwars) / float32(statsBedwars.EightOneFinalDeathsBedwars)
		WL := float32(statsBedwars.EightOneWinsBedwars) / float32(statsBedwars.EightOneLossesBedwars)
		return &BedwarsStats{
			playerStats.Player.Achievements.BedwarsLevel,
			statsBedwars.EightOneKillsBedwars,
			statsBedwars.EightOneDeathsBedwars,
			KD,
			statsBedwars.EightOneFinalKillsBedwars,
			statsBedwars.EightOneFinalDeathsBedwars,
			FinalKD,
			statsBedwars.EightOneWinsBedwars,
			statsBedwars.EightOneLossesBedwars,
			WL,
			statsBedwars.EightOneWinstreak,
			statsBedwars.EightOneBedsBroken,
		}, nil
	case BedwarsTypeDoubles:
		statsBedwars := playerStats.Player.Stats.Bedwars
		KD := float32(statsBedwars.EightTwoKillsBedwars) / float32(statsBedwars.EightTwoDeathsBedwars)
		FinalKD := float32(statsBedwars.EightTwoFinalKillsBedwars) / float32(statsBedwars.EightTwoFinalDeathsBedwars)
		WL := float32(statsBedwars.EightTwoWinsBedwars) / float32(statsBedwars.EightTwoLossesBedwars)
		return &BedwarsStats{
			playerStats.Player.Achievements.BedwarsLevel,
			statsBedwars.EightTwoKillsBedwars,
			statsBedwars.EightTwoDeathsBedwars,
			KD,
			statsBedwars.EightTwoFinalKillsBedwars,
			statsBedwars.EightTwoFinalDeathsBedwars,
			FinalKD,
			statsBedwars.EightTwoWinsBedwars,
			statsBedwars.EightTwoLossesBedwars,
			WL,
			statsBedwars.EightTwoWinstreak,
			statsBedwars.EightTwoBedsBroken,
		}, nil
	case BedwarsType3v3v3v3:
		statsBedwars := playerStats.Player.Stats.Bedwars
		KD := float32(statsBedwars.FourThreeKillsBedwars) / float32(statsBedwars.FourThreeDeathsBedwars)
		FinalKD := float32(statsBedwars.FourThreeFinalKillsBedwars) / float32(statsBedwars.FourThreeFinalDeathsBedwars)
		WL := float32(statsBedwars.FourThreeWinsBedwars) / float32(statsBedwars.FourThreeLossesBedwars)
		return &BedwarsStats{
			playerStats.Player.Achievements.BedwarsLevel,
			statsBedwars.FourThreeKillsBedwars,
			statsBedwars.FourThreeDeathsBedwars,
			KD,
			statsBedwars.FourThreeFinalKillsBedwars,
			statsBedwars.FourThreeFinalDeathsBedwars,
			FinalKD,
			statsBedwars.FourThreeWinsBedwars,
			statsBedwars.FourThreeLossesBedwars,
			WL,
			statsBedwars.FourThreeWinstreak,
			statsBedwars.FourThreeBedsBroken,
		}, nil
	case BedwarsType4v4v4v4:
		statsBedwars := playerStats.Player.Stats.Bedwars
		KD := float32(statsBedwars.FourFourKillsBedwars) / float32(statsBedwars.FourFourDeathsBedwars)
		FinalKD := float32(statsBedwars.FourFourFinalKillsBedwars) / float32(statsBedwars.FourFourFinalDeathsBedwars)
		WL := float32(statsBedwars.FourFourWinsBedwars) / float32(statsBedwars.FourFourLossesBedwars)
		return &BedwarsStats{
			playerStats.Player.Achievements.BedwarsLevel,
			statsBedwars.FourFourKillsBedwars,
			statsBedwars.FourFourDeathsBedwars,
			KD,
			statsBedwars.FourFourFinalKillsBedwars,
			statsBedwars.FourFourFinalDeathsBedwars,
			FinalKD,
			statsBedwars.FourFourWinsBedwars,
			statsBedwars.FourFourLossesBedwars,
			WL,
			statsBedwars.FourFourWinstreak,
			statsBedwars.FourFourBedsBroken,
		}, nil
	case BedwarsType4v4:
		statsBedwars := playerStats.Player.Stats.Bedwars
		KD := float32(statsBedwars.TwoFourKillsBedwars) / float32(statsBedwars.TwoFourDeathsBedwars)
		FinalKD := float32(statsBedwars.TwoFourFinalKillsBedwars) / float32(statsBedwars.TwoFourFinalDeathsBedwars)
		WL := float32(statsBedwars.TwoFourWinsBedwars) / float32(statsBedwars.TwoFourLossesBedwars)
		return &BedwarsStats{
			playerStats.Player.Achievements.BedwarsLevel,
			statsBedwars.TwoFourKillsBedwars,
			statsBedwars.TwoFourDeathsBedwars,
			KD,
			statsBedwars.TwoFourFinalKillsBedwars,
			statsBedwars.TwoFourFinalDeathsBedwars,
			FinalKD,
			statsBedwars.TwoFourWinsBedwars,
			statsBedwars.TwoFourLossesBedwars,
			WL,
			statsBedwars.TwoFourWinstreak,
			statsBedwars.TwoFourBedsBroken,
		}, nil
	default:
		return nil, errors.New("Invalid BedwarsType")
	}
}
