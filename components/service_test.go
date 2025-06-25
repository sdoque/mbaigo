/*******************************************************************************
 * Copyright (c) 2025 Synecdoque
 *
 * Permission is hereby granted, free of charge, to any person obtaining a copy
 * of this software and associated documentation files (the "Software"), to deal
 * in the Software without restriction, including without limitation the rights
 * to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
 * copies of the Software, subject to the following conditions:
 *
 * The software is licensed under the MIT License. See the LICENSE file in this repository for details.
 *
 * Contributors:
 *   Jan A. van Deventer, Luleå - initial implementation
 *   Thomas Hedeler, Hamburg - initial implementation
 ***************************************************************************SDG*/

package components

import (
	"reflect"
	"testing"
)

type mergeDetailsTestStruct struct {
	map1     map[string][]string
	map2     map[string][]string
	expected map[string][]string
}

var testService = Service{
	ID:            1,
	Definition:    "test",
	SubPath:       "testSubPath",
	Details:       make(map[string][]string),
	RegPeriod:     45,
	RegTimestamp:  "",
	RegExpiration: "",
	Description:   "A test service",
	SubscribeAble: false,
	ACost:         0,
	CUnit:         "",
}

var testOriginalService = Service{
	ID:            1,
	Definition:    "original one",
	SubPath:       "testOriginalSubPath",
	Details:       map[string][]string{"test": {"test1", "test2"}},
	RegPeriod:     45,
	RegTimestamp:  "",
	RegExpiration: "",
	Description:   "A test original service",
	SubscribeAble: false,
	ACost:         0,
	CUnit:         "",
}

var testServiceWithEmptyDetails = Service{
	ID:            1,
	Definition:    "original one",
	SubPath:       "testOriginalSubPath",
	Details:       make(map[string][]string),
	RegPeriod:     45,
	RegTimestamp:  "",
	RegExpiration: "",
	Description:   "A test original service",
	SubscribeAble: false,
	ACost:         0,
	CUnit:         "",
}

func makeNewTestService(id int, definition string) *Service {
	return &Service{
		ID:            id,
		Definition:    definition,
		SubPath:       "newTestServiceSubPath",
		Details:       make(map[string][]string),
		RegPeriod:     45,
		RegTimestamp:  "",
		RegExpiration: "",
		Description:   "A new test Service",
		SubscribeAble: false,
		ACost:         0,
		CUnit:         "",
	}
}

func makeNewMap(key string, value string) map[string][]string {
	newMap := map[string][]string{
		key: {value},
	}
	return newMap
}

var expectedRegularMerge = map[string][]string{
	"a": {"1"},
	"b": {"2"},
}

var expectedKeyOverlapMerge = map[string][]string{
	"a": {"1", "3"},
}

var expectedOneEmptyMapMerge = map[string][]string{
	"a": {"1"},
}

var expectedBothEmptyMapMerge = map[string][]string{}

var mergeDetailsTestParams = []mergeDetailsTestStruct{
	{makeNewMap("a", "1"), makeNewMap("b", "2"), expectedRegularMerge},
	{makeNewMap("a", "1"), makeNewMap("a", "3"), expectedKeyOverlapMerge},
	{makeNewMap("a", "1"), make(map[string][]string), expectedOneEmptyMapMerge},
	{make(map[string][]string), make(map[string][]string), expectedBothEmptyMapMerge},
}

func TestMerge(t *testing.T) {
	testService.Merge(&testOriginalService)
	if testService.Definition != testOriginalService.Definition ||
		testService.SubPath != testOriginalService.SubPath ||
		testService.Description != testOriginalService.Description {
		t.Errorf("Expected the test service to be the same as the original test service %s, got: %s", testOriginalService.Definition, testService.Definition)
	}
}

func TestDeepCopy(t *testing.T) {
	res := testOriginalService.DeepCopy()
	res.Details["test"][0] = "changed"
	res.Details["newkey"] = []string{"newTest"}

	if testOriginalService.Details["test"][0] == "changed" {
		t.Errorf("DeepCopy failed, expected original slice to remain, original slice was mutated")
	}
	if _, ok := testOriginalService.Details["newkey"]; ok {
		t.Errorf("DeepCopy failed, expected no new key in original, got %s", testOriginalService.Details["newkey"])
	}

	res = testServiceWithEmptyDetails.DeepCopy()
	if len(res.Details) != 0 {
		t.Errorf("DeepCopy failed, expected details map to be empty after copy, got: %v", res.Details)
	}
}

func TestCloneServices(t *testing.T) {
	test1 := makeNewTestService(1, "test")
	test2 := makeNewTestService(2, "test")

	cloned := CloneServices([]Service{*test1, *test2})
	if len(cloned) != 1 {
		t.Errorf("Expected 1 Service, got %d", len(cloned))
	}
	if cloned["test"].ID != 2 {
		t.Errorf("Second Service did not overwrite the first as expected")
	}

	cloned["test"].ID = 3
	if test1.ID == 3 || test2.ID == 3 {
		t.Errorf("DeepCopy failed: mutation of clone affected either one of the originals")
	}

	cloned = CloneServices(nil)
	if cloned == nil {
		t.Errorf("Expected non-nil empty map for nil input")
	}
	if len(cloned) != 0 {
		t.Errorf("Expected 0 Services, got %d", len(cloned))
	}

	test1 = makeNewTestService(1, "")
	test2 = makeNewTestService(2, "")

	cloned = CloneServices([]Service{*test1, *test2})

	if len(cloned) != 1 {
		t.Errorf("Expected 1 entry, got %d", len(cloned))
	}
}

func TestMergeDetails(t *testing.T) {
	for _, test := range mergeDetailsTestParams {
		merged := MergeDetails(test.map1, test.map2)

		if !reflect.DeepEqual(merged, test.expected) {
			t.Errorf("Expected %v, got %v", test.expected, merged)
		}

		if len(merged) != 0 {
			merged["a"][0] = "changed"

			if reflect.DeepEqual(merged, test.map1) || reflect.DeepEqual(merged, test.map2) {
				t.Errorf("A change in the merged map resulted in a change in the input maps")
			}
		}
	}
}
