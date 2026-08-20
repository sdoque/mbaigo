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

// Files: a named payload with its media type, for services that carry documents rather than values.

import (
	"io"
	"log"
	"net/http"
	"net/url"
	"path"
	"reflect"
	"strings"
	"time"
)

// FileForm implements the form structure
type FileForm_v1 struct {
	FileURL   string    `json:"file_url"`
	Timestamp time.Time `json:"timestamp"`
	Version   string    `json:"version"`
}

func (fileForm *FileForm_v1) NewForm() Form {
	fileForm.Version = "FileForm_v1.0"
	return fileForm
}

func (fileForm *FileForm_v1) FormVersion() string {
	return fileForm.Version
}

// Register FileForm_v1 in the formTypeMap. The key must match the Version
// string set in NewForm (see SignalA/SignalB which follow the same "_v1.0"
// convention) — otherwise Unpack can never resolve an incoming payload back
// to this type and fails with "unsupported form version".
func init() {
	FormTypeMap["FileForm_v1.0"] = reflect.TypeOf(FileForm_v1{})
}

const fileDir string = "files"

// TransferFile enables the transfer of different types files when the filename is given in the URL
func TransferFile(w http.ResponseWriter, r *http.Request) {
	// Parse the URL to ensure it's valid and to easily extract parts of it
	parsedURL, err := url.Parse(r.URL.Path)
	if err != nil {
		log.Println("Error parsing URL:", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Extract the path component of the URL
	urlPath := parsedURL.Path
	filename := path.Base(urlPath)

	// Extract the file extension and determine the content type
	fileExt := strings.ToLower(path.Ext(filename))
	contentType := "application/octet-stream" // Default content type
	switch fileExt {
	case ".jpg", ".jpeg":
		contentType = "image/jpeg"
	case ".zip":
		contentType = "application/zip"
	case ".txt":
		contentType = "text/plain"
	case ".owl":
		contentType = "application/rdf+xml"
	case ".ttl":
		contentType = "text/turtle"
	case ".html", ".htm":
		contentType = "text/html"
	case ".csv":
		contentType = "text/csv"
	case ".mp4":
		contentType = "video/mp4"
	}

	// Open the requested file from the ./files directory
	dir := http.Dir(fileDir)
	reqFile, err := dir.Open(filename)
	if err != nil {
		log.Println("Requested file not found:", err)
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}
	defer reqFile.Close()

	// Set headers
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", "attachment; filename="+filename)

	// Copy the file's contents to the response writer
	if _, err := io.Copy(w, reqFile); err != nil {
		log.Println("Error serving requested file:", err)
		http.Error(w, "Failed to serve requested file", http.StatusInternalServerError)
	}
}
