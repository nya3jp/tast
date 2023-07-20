// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package cellularconst defines the constants for Cellular
// This package is defined under common/ as they might be used in both
// local and remote tests.
package cellularconst

import (
	"fmt"

	"go.chromium.org/tast/core/errors"
)

// ModemType is the type of modem used in a device.
type ModemType uint32

// Supported modem types.
const (
	ModemTypeUnknown ModemType = iota
	ModemTypeL850
	ModemTypeNL668
	ModemTypeFM350
	ModemTypeFM101
	ModemTypeSC7180 // trogdor
	ModemTypeSC7280 // herobrine
)

func (e ModemType) String() string {
	switch e {
	case ModemTypeL850:
		return "L850"
	case ModemTypeNL668:
		return "NL668"
	case ModemTypeFM350:
		return "FM350"
	case ModemTypeFM101:
		return "FM101"
	case ModemTypeSC7180:
		return "SC7180"
	case ModemTypeSC7280:
		return "SC7280"
	default:
		return fmt.Sprintf("%d", int(e))
	}
}

// DeviceInfo provides a mapping between variant and modem type.
type DeviceInfo struct {
	ModemVariant string
	Board        string
	Modem        ModemType
}

var (
	// KnownVariants mapping between variant and modem type.
	KnownVariants = map[string]DeviceInfo{
		"anahera_l850":       {"anahera_l850", "brya", ModemTypeL850},
		"brya_fm350":         {"brya_fm350", "brya", ModemTypeFM350},
		"brya_l850":          {"brya_l850", "brya", ModemTypeL850},
		"crota_fm101":        {"crota_fm101", "brya", ModemTypeFM101},
		"primus_l850":        {"primus_l850", "brya", ModemTypeL850},
		"redrix_fm350":       {"redrix_fm350", "brya", ModemTypeFM350},
		"redrix_l850":        {"redrix_l850", "brya", ModemTypeL850},
		"vell_fm350":         {"vell_fm350", "brya", ModemTypeFM350},
		"astronaut":          {"astronaut", "coral", ModemTypeL850},
		"krabby_fm101":       {"krabby_fm101", "corsola", ModemTypeFM101},
		"rusty_fm101":        {"rusty_fm101", "corsola", ModemTypeFM101},
		"steelix_fm101":      {"steelix_fm101", "corsola", ModemTypeFM101},
		"beadrix_nl668am":    {"beadrix_nl668am", "dedede", ModemTypeNL668},
		"boten":              {"boten", "dedede", ModemTypeL850},
		"bugzzy_l850gl":      {"bugzzy_l850gl", "dedede", ModemTypeL850},
		"bugzzy_nl668am":     {"bugzzy_nl668am", "dedede", ModemTypeNL668},
		"cret":               {"cret", "dedede", ModemTypeL850},
		"drallion":           {"drallion", "drallion", ModemTypeL850},
		"drawper_l850gl":     {"drawper_l850gl", "dedede", ModemTypeL850},
		"kracko_nl668am":     {"kracko_nl668am", "dedede", ModemTypeNL668},
		"kracko_fm101_cat12": {"kracko_fm101_cat12", "dedede", ModemTypeFM101},
		"kracko_fm101_cat6":  {"kracko_fm101_cat6", "dedede", ModemTypeFM101},
		"metaknight":         {"metaknight", "dedede", ModemTypeL850},
		"sasuke":             {"sasuke", "dedede", ModemTypeL850},
		"sasuke_nl668am":     {"sasuke_nl668am", "dedede", ModemTypeNL668},
		"sasukette":          {"sasukette", "dedede", ModemTypeL850},
		"storo360_l850gl":    {"storo360_l850gl", "dedede", ModemTypeL850},
		"storo360_nl668am":   {"storo360_nl668am", "dedede", ModemTypeNL668},
		"storo_l850gl":       {"storo_l850gl", "dedede", ModemTypeL850},
		"storo_nl668am":      {"storo_nl668am", "dedede", ModemTypeNL668},
		"guybrush360_l850":   {"guybrush360_l850", "guybrush", ModemTypeL850},
		"guybrush_fm350":     {"guybrush_fm350", "guybrush", ModemTypeFM350},
		"nipperkin":          {"nipperkin", "guybrush", ModemTypeL850},
		"jinlon":             {"jinlon", "hatch", ModemTypeL850},
		"evoker_sc7280":      {"evoker_sc7280", "herobrine", ModemTypeSC7280},
		"herobrine_sc7280":   {"herobrine_sc7280", "herobrine", ModemTypeSC7280},
		"hoglin_sc7280":      {"hoglin_sc7280", "herobrine", ModemTypeSC7280},
		"piglin_sc7280":      {"piglin_sc7280", "herobrine", ModemTypeSC7280},
		"villager_sc7280":    {"villager_sc7280", "herobrine", ModemTypeSC7280},
		"zoglin_sc7280":      {"zoglin_sc7280", "herobrine", ModemTypeSC7280},
		"zombie_sc7280":      {"zombie_sc7280", "herobrine", ModemTypeSC7280},
		"gooey":              {"gooey", "keeby", ModemTypeL850},
		"nautiluslte":        {"nautiluslte", "nautilus", ModemTypeL850},
		"craask_fm101":       {"craask_fm101", "nissa", ModemTypeFM101},
		"nivviks_fm101":      {"nivviks_fm101", "nissa", ModemTypeFM101},
		"pujjo_fm101":        {"pujjo_fm101", "nissa", ModemTypeFM101},
		"dood":               {"dood", "octopus", ModemTypeL850},
		"droid":              {"droid", "octopus", ModemTypeL850},
		"fleex":              {"fleex", "octopus", ModemTypeL850},
		"garg":               {"garg", "octopus", ModemTypeL850},
		"rex_fm101":          {"rex_fm101", "rex", ModemTypeFM101},
		"rex_fm350":          {"rex_fm350", "rex", ModemTypeFM350},
		"arcada":             {"arcada", "sarien", ModemTypeL850},
		"sarien":             {"sarien", "sarien", ModemTypeL850},
		"coachz":             {"coachz", "strongbad", ModemTypeSC7180},
		"quackingstick":      {"quackingstick", "strongbad", ModemTypeSC7180},
		"kingoftown":         {"kingoftown", "trogdor", ModemTypeSC7180},
		"lazor":              {"lazor", "trogdor", ModemTypeSC7180},
		"limozeen":           {"limozeen", "trogdor", ModemTypeSC7180},
		"pazquel":            {"pazquel", "trogdor", ModemTypeSC7180},
		"pazquel360":         {"pazquel360", "trogdor", ModemTypeSC7180},
		"skyrim_fm101":       {"skyrim_fm101", "skyrim", ModemTypeFM101},
		"vilboz":             {"vilboz", "zork", ModemTypeNL668},
		"vilboz360":          {"vilboz360", "zork", ModemTypeL850},
	}
)

// GetModemTypeFromVariant gets DUT's modem type from variant.
func GetModemTypeFromVariant(variant string) (ModemType, error) {
	device, ok := KnownVariants[variant]
	if !ok {
		return 0, errors.Errorf("variant %q is not in |KnownVariants|", variant)
	}
	return device.Modem, nil
}
