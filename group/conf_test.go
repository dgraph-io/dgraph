package group

import "testing"

func TestGroups(t *testing.T) {
	err := ParseGroupConfig("group_tests/filemissing.conf")
	if err != nil {
		t.Errorf("Expected nil error. Got: %v", err)
	}
	gid := BelongsTo("type.object.name.en")
	if gid != 0 {
		t.Errorf("Expected groupId to be: %v. Got: %v", 0, gid)
	}

	groupConfig = config{}
	err = ParseGroupConfig("group_tests/defaultmissing.conf")
	if err.Error() != "Cant take modulo 0." {
		t.Error("Error doesn't match expected value")
	}

	groupConfig = config{}
	err = ParseGroupConfig("group_tests/defaultwrongseq.conf")
	if err.Error() != "k in (fp mod N + k) should be <= the last groupno 3." {
		t.Error("Error doesn't match expected value")
	}

	groupConfig = config{}
	err = ParseGroupConfig("group_tests/defaultnotlast.conf")
	if err.Error() != "Default config should be specified as the last line. Found 2: type.object.name*, film.performance.*" {
		t.Error("Error doesn't match expected value")
	}

	groupConfig = config{}
	err = ParseGroupConfig("group_tests/doubledefault.conf")
	if err.Error() != "Default config can only be defined once:  fp % 10 + 3" {
		t.Error("Error doesn't match expected value")
	}

	groupConfig = config{}
	err = ParseGroupConfig("group_tests/incorrectformat.conf")
	if err.Error() != "Incorrect format for config line: _uid_" {
		t.Error("Error doesn't match expected value")
	}

	groupConfig = config{}
	err = ParseGroupConfig("group_tests/wrongformat.conf")
	if err.Error() != "Default config format should be like: default: fp % n + k" {
		t.Error("Error doesn't match expected value")
	}

	groupConfig = config{}
	err = ParseGroupConfig("group_tests/wrongsequence.conf")
	if err.Error() != "Group ids should be sequential and should start from 0. Found 7, should have been 2" {
		t.Error("Error doesn't match expected value")
	}

	groupConfig = config{}
	if err = ParseGroupConfig("group_tests/defaultright.conf"); err != nil {
		t.Errorf("Expected nil error. Got: %v", err)
	}

	groupConfig = config{}
	if err = ParseGroupConfig("group_tests/rightsequence.conf"); err != nil {
		t.Errorf("Expected nil error. Got: %v", err)
	}
	gid = BelongsTo("type.object.name.en")
	if gid != 1 {
		t.Errorf("Expected groupId to be: %v. Got: %v", 1, gid)
	}
	gid = BelongsTo("type.object.name.fr")
	if gid != 2 {
		t.Errorf("Expected groupId to be: %v. Got: %v", 2, gid)
	}
	gid = BelongsTo("film.actor.film")
	if gid != 11 {
		t.Errorf("Expected groupId to be: %v. Got: %v", 11, gid)
	}
}
