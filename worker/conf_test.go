package worker

import "testing"

func TestGroups(t *testing.T) {
	ParseGroupConfig("group_tests/defaultmissing.conf")
	gid := group("film.actor.film")
	if gid != 7 {
		t.Errorf("Expected groupId to be: %v. Got: %v", 7, gid)
	}

	groupConfig = config{}
	ParseGroupConfig("group_tests/defaultwrongseq.conf")

	groupConfig = config{}
	ParseGroupConfig("group_tests/wrongsequence.conf")

	groupConfig = config{}
	ParseGroupConfig("group_tests/defaultright.conf")

	groupConfig = config{}
	ParseGroupConfig("group_tests/rightsequence.conf")
	gid = group("type.object.name.en")
	if gid != 1 {
		t.Errorf("Expected groupId to be: %v. Got: %v", 1, gid)
	}
	gid = group("type.object.name.fr")
	if gid != 2 {
		t.Errorf("Expected groupId to be: %v. Got: %v", 2, gid)
	}
	gid = group("film.actor.film")
	if gid != 7 {
		t.Errorf("Expected groupId to be: %v. Got: %v", 7, gid)
	}
}
