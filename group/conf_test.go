package group

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGroups(t *testing.T) {
	err := ParseGroupConfig("group_tests/filemissing.conf")
	if err != nil {
		t.Errorf("Expected nil error. Got: %v", err)
	}
	gid := BelongsTo("type.object.name.en")
	if gid != 1 {
		t.Errorf("Expected groupId to be: %v. Got: %v", 1, gid)
	}
	gid = BelongsTo("_uid_")
	if gid != 1 {
		t.Errorf("Expected groupId to be: %v. Got: %v", 1, gid)
	}

	groupConfig = config{}
	err = ParseGroupConfig("group_tests/defaultmissing.conf")
	if err.Error() != "Cant take modulo 0." {
		t.Error("Error doesn't match expected value")
	}

	groupConfig = config{}
	err = ParseGroupConfig("group_tests/defaultwrongseq.conf")
	require.Contains(t, err.Error(), "k in (fp mod N + k) should be")

	groupConfig = config{}
	err = ParseGroupConfig("group_tests/defaultnotlast.conf")
	require.Contains(t, err.Error(), "Default config should be specified as the last line.")

	groupConfig = config{}
	err = ParseGroupConfig("group_tests/doubledefault.conf")
	require.Contains(t, err.Error(), "Default config can only be defined once:")

	groupConfig = config{}
	err = ParseGroupConfig("group_tests/zerok.conf")
	require.Contains(t, err.Error(), "k in fp")

	groupConfig = config{}
	err = ParseGroupConfig("group_tests/incorrectformat.conf")
	if err.Error() != "Incorrect format for config line: _uid_" {
		t.Error("Error doesn't match expected value")
	}

	groupConfig = config{}
	err = ParseGroupConfig("group_tests/wrongformat.conf")
	require.Contains(t, err.Error(), "Default config format should be like:")

	groupConfig = config{}
	err = ParseGroupConfig("group_tests/wrongsequence.conf")
	require.Contains(t, err.Error(), "Group ids should be sequential and should start from 1")

	groupConfig = config{}
	if err = ParseGroupConfig("group_tests/defaultright.conf"); err != nil {
		t.Errorf("Expected nil error. Got: %v", err)
	}

	groupConfig = config{}
	err = ParseGroupConfig("group_tests/zeropred.conf")
	require.Contains(t, err.Error(), "Group ids should be greater than zero.")

	groupConfig = config{}
	if err = ParseGroupConfig("group_tests/rightsequence.conf"); err != nil {
		t.Errorf("Expected nil error. Got: %v", err)
	}
	gid = BelongsTo("_uid_")
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
