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

// The "forms" package is designed to define structured schemas, known as "structs,"
// which represent the format and organization of documents intended for data exchange.
// These structs are utilized to create forms that are populated with data, acting as
// standardized payloads for transmission between different systems. This ensures that
// the data exchanged maintains a consistent structure, facilitating seamless
// integration and processing across system boundaries.

// File forms are used for the exchange of complete files.

package forms

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

// Register FileForm_v1 in the formTypeMap
func init() {
	FormTypeMap["FileForm_v1"] = reflect.TypeOf(FileForm_v1{})
}

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
	case ".html", ".htm":
		contentType = "text/html"
	case ".csv":
		contentType = "text/csv"
	case ".mp4":
		contentType = "video/mp4"
	}

	// Open the requested file from the ./files directory
	dir := http.Dir("./files")
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
