/*******************************************************************************
 * Copyright (c) 2024 Synecdoque
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

package forms

// Systems: how a cloud describes the systems it holds.

import "reflect"

type SystemRecordList_v1 struct {
	List    []string `json:"systemurl"`
	Version string   `json:"version"`
}

func (f *SystemRecordList_v1) NewForm() Form {
	f.Version = "SystemRecordList_v1"
	return f
}

func (f *SystemRecordList_v1) FormVersion() string {
	return f.Version
}

// Register SystemRecordList_v1 in the formTypeMap
func init() {
	FormTypeMap["SystemRecordList_v1"] = reflect.TypeOf(SystemRecordList_v1{})
}
