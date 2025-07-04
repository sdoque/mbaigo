package forms

import (
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

type transferFileTestStruct struct {
	inputW       http.ResponseWriter
	filename     string
	expectedBody string
	expectedCode int
	fileType     string
	testName     string
}

type mockResponseWriter struct {
	http.ResponseWriter
}

func (e *mockResponseWriter) Write(b []byte) (int, error) {
	return 0, fmt.Errorf("Forced write error")
}

func (e *mockResponseWriter) WriteHeader(statusCode int) {}

func (e *mockResponseWriter) Header() http.Header {
	return make(http.Header)
}

var transferFileTestParams = []transferFileTestStruct{
	{httptest.NewRecorder(), "test.jpeg", "\xff\xd8", 200, ".jpeg", "Good case, jpeg works"},
	{httptest.NewRecorder(), "test.zip", "\x50\x4b\x03\x04", 200, ".zip", "Good case, zip works"},
	{httptest.NewRecorder(), "test.txt", "\n", 200, ".txt", "Good case, txt works"},
	{httptest.NewRecorder(), "test.owl", `<?xml version="1.0"?><rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#"` +
		`xmlns:owl="http://www.w3.org/2002/07/owl#"><owl:Ontology rdf:about=""/></rdf:RDF>`, 200, ".owl", "Good case, owl works"},
	{httptest.NewRecorder(), "test.ttl", "@prefix : <#> .@prefix rdf: <http://www.w3.org/1999/02/22-rdf-syntax-ns#> .", 200,
		".ttl", "Good case, ttl works"},
	{httptest.NewRecorder(), "test.html", "<!DOCTYPE html><html><head><title></title></head><body></body></html>",
		200, ".html", "Good case, html works"},
	{httptest.NewRecorder(), "test.csv", "id,name\n", 200, ".csv", "Good case, csv works"},
	{httptest.NewRecorder(), "test.mp4", "\x00\x00\x00\x18\x66\x74\x79\x70\x69\x73\x6f\x6d\x00\x00\x02\x00\x69\x73\x6f\x6d\x69\x73\x6f\x32",
		200, ".mp4", "Good case, mp4 works"},
	{httptest.NewRecorder(), "test.txt", "Internal Server Error\n", 500, ".txt", "Bad case, parsing url fails"},
	{httptest.NewRecorder(), "wrong.txt", "Not Found\n", 404, ".txt", "Bad case, file not found"},
	{&mockResponseWriter{}, "test.txt", "Failed to serve requested file\n", 500, ".txt", "Bad case, copy fails"},
}

var dir = "./files"

var minimalJPEG = []byte{0xFF, 0xD8}
var minimalZIP = []byte{0x50, 0x4B, 0x03, 0x04}
var minimalTXT = []byte("\n")
var minimalOWL = []byte(`<?xml version="1.0"?><rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#"` +
	`xmlns:owl="http://www.w3.org/2002/07/owl#"><owl:Ontology rdf:about=""/></rdf:RDF>`)
var minimalTTL = []byte("@prefix : <#> .@prefix rdf: <http://www.w3.org/1999/02/22-rdf-syntax-ns#> .")
var minimalHTML = []byte("<!DOCTYPE html><html><head><title></title></head><body></body></html>")
var minimalCSV = []byte("id,name\n")
var minimalMP4 = []byte{0x00, 0x00, 0x00, 0x18, 0x66, 0x74, 0x79, 0x70, 0x69, 0x73, 0x6F, 0x6D,
	0x00, 0x00, 0x02, 0x00, 0x69, 0x73, 0x6F, 0x6D, 0x69, 0x73, 0x6F, 0x32}

func createTestFolderAndFile(filename string, fileType string) {
	fullPath := "./" + filepath.Join(dir, filename)
	err := os.MkdirAll(dir, 0755)
	if err != nil {
		log.Fatalf("Error creating test directory: %v", err)
		return
	}

	f, err := os.OpenFile(fullPath, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		if os.IsExist(err) {
			log.Fatalf("File already exists: %v", err)
		} else {
			log.Fatalf("Error creating file: %v", err)
		}
		return
	}
	defer f.Close()

	data := []byte{}
	switch fileType {
	case ".jpg", ".jpeg":
		data = minimalJPEG
	case ".zip":
		data = minimalZIP
	case ".txt":
		data = minimalTXT
	case ".owl":
		data = minimalOWL
	case ".ttl":
		data = minimalTTL
	case ".html", ".htm":
		data = minimalHTML
	case ".csv":
		data = minimalCSV
	case ".mp4":
		data = minimalMP4
	default:
		log.Fatalf("Filetype is wrong")
		return
	}

	err = os.WriteFile(fullPath, data, 0644)
	if err != nil {
		log.Fatalf("Error encoding to jpeg: %v", err)
	}
}

func removeTestFolderAndFile(filename string) {
	fullPath := "./" + filepath.Join(dir, filename)
	if err := os.Remove(fullPath); err != nil {
		log.Fatalf("Error deleting file: %v", err)
	}
	if err := os.Remove(dir); err != nil {
		log.Fatalf("Error deleting directory: %v", err)
	}
}

func TestTransferFile(t *testing.T) {
	for _, testCase := range transferFileTestParams {
		fullPath := "/" + filepath.Join(dir, testCase.filename)
		inputR := httptest.NewRequest(http.MethodPost, fullPath, nil)
		if testCase.testName == "Bad case, parsing url fails" {
			inputR.URL.Path = "/foo%ZZbar"
		}
		if testCase.testName == "Bad case, file not found" {
			inputR.URL.Path = "/files/doesNotExist.error"
		}

		createTestFolderAndFile(testCase.filename, testCase.fileType)
		TransferFile(testCase.inputW, inputR)
		removeTestFolderAndFile(testCase.filename)

		if testCase.testName == "Bad case, copy fails" {
			if _, ok := testCase.inputW.(*mockResponseWriter); !ok {
				t.Errorf("Expected inputW to be of type *mockResponseWriter")
			}
		}

		recorder, ok := testCase.inputW.(*httptest.ResponseRecorder)
		if ok {
			if recorder.Body.String() != testCase.expectedBody || recorder.Code != testCase.expectedCode {
				t.Errorf("Expected: %s and %d, got: %s and %d", testCase.expectedBody, testCase.expectedCode, recorder.Body.String(), recorder.Code)
			}
		}
	}
}
