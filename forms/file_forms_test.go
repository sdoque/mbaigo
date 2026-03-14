package forms

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"path/filepath"
	"testing"
)

type transferFileTestStruct struct {
	filename     string
	expectedBody string
	expectedCode int
	fileType     string
	testName     string
	urlOverride  string // if non-empty, use this as the request URL path instead of the constructed one
}

type mockResponseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (e *mockResponseWriter) Write(b []byte) (int, error) {
	e.WriteHeader(300)
	return 0, fmt.Errorf("Forced write error")
}

func (e *mockResponseWriter) WriteHeader(statusCode int) {
	e.statusCode = statusCode
}

func (e *mockResponseWriter) Header() http.Header {
	return make(http.Header)
}

var transferFileTestParams = []transferFileTestStruct{
	{filename: "test.jpeg", expectedBody: "\xff\xd8", expectedCode: 200, fileType: ".jpeg", testName: "Good case, jpeg works"},
	{filename: "test.zip", expectedBody: "\x50\x4b\x03\x04", expectedCode: 200, fileType: ".zip", testName: "Good case, zip works"},
	{filename: "test.txt", expectedBody: "\n", expectedCode: 200, fileType: ".txt", testName: "Good case, txt works"},
	{
		filename: "test.owl",
		expectedBody: `<?xml version="1.0"?><rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#"` +
			`xmlns:owl="http://www.w3.org/2002/07/owl#"><owl:Ontology rdf:about=""/></rdf:RDF>`,
		expectedCode: 200, fileType: ".owl", testName: "Good case, owl works",
	},
	{filename: "test.ttl", expectedBody: "@prefix : <#> .@prefix rdf: <http://www.w3.org/1999/02/22-rdf-syntax-ns#> .", expectedCode: 200, fileType: ".ttl", testName: "Good case, ttl works"},
	{filename: "test.html", expectedBody: "<!DOCTYPE html><html><head><title></title></head><body></body></html>", expectedCode: 200, fileType: ".html", testName: "Good case, html works"},
	{filename: "test.csv", expectedBody: "id,name\n", expectedCode: 200, fileType: ".csv", testName: "Good case, csv works"},
	{filename: "test.mp4", expectedBody: "\x00\x00\x00\x18\x66\x74\x79\x70\x69\x73\x6f\x6d\x00\x00\x02\x00\x69\x73\x6f\x6d\x69\x73\x6f\x32", expectedCode: 200, fileType: ".mp4", testName: "Good case, mp4 works"},
	{filename: "test.txt", expectedBody: "Internal Server Error\n", expectedCode: 500, fileType: ".txt", testName: "Bad case, parsing url fails", urlOverride: "/foo%ZZbar"},
	{filename: "wrong.txt", expectedBody: "Not Found\n", expectedCode: 404, fileType: ".txt", testName: "Bad case, file not found", urlOverride: "/files/doesNotExist.error"},
}

var fileTypeMap = map[string][]byte{
	".jpeg": {0xFF, 0xD8},
	".zip":  {0x50, 0x4B, 0x03, 0x04},
	".txt":  []byte("\n"),
	".owl": []byte(`<?xml version="1.0"?><rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#"` +
		`xmlns:owl="http://www.w3.org/2002/07/owl#"><owl:Ontology rdf:about=""/></rdf:RDF>`),
	".ttl":  []byte("@prefix : <#> .@prefix rdf: <http://www.w3.org/1999/02/22-rdf-syntax-ns#> ."),
	".html": []byte("<!DOCTYPE html><html><head><title></title></head><body></body></html>"),
	".csv":  []byte("id,name\n"),
	".mp4": {0x00, 0x00, 0x00, 0x18, 0x66, 0x74, 0x79, 0x70, 0x69, 0x73, 0x6F, 0x6D,
		0x00, 0x00, 0x02, 0x00, 0x69, 0x73, 0x6F, 0x6D, 0x69, 0x73, 0x6F, 0x32},
}

func createTestFolderAndFile(filename string, fileType string) error {
	fullPath := filepath.Join(fileDir, filename)
	err := os.MkdirAll(fileDir, 0755)
	if err != nil {
		return err
	}

	f, err := os.OpenFile(fullPath, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer f.Close()

	return os.WriteFile(fullPath, fileTypeMap[fileType], 0644)
}

func removeTestFolderAndFile() error {
	return os.RemoveAll(fileDir)
}

func TestTransferFile(t *testing.T) {
	for _, testCase := range transferFileTestParams {
		fileURL := "/" + path.Join(fileDir, testCase.filename)
		inputW := httptest.NewRecorder()
		inputR := httptest.NewRequest(http.MethodPost, fileURL, nil)
		if testCase.urlOverride != "" {
			inputR.URL.Path = testCase.urlOverride
		}

		err := createTestFolderAndFile(testCase.filename, testCase.fileType)
		if err != nil {
			t.Error(err)
			continue
		}
		TransferFile(inputW, inputR)
		err = removeTestFolderAndFile()
		if err != nil {
			t.Error(err)
		}

		if inputW.Body.String() != testCase.expectedBody || inputW.Code != testCase.expectedCode {
			t.Errorf("Expected: %s and %d, got: %s and %d",
				testCase.expectedBody, testCase.expectedCode, inputW.Body.String(), inputW.Code)
		}
	}

	// Special case
	fullPath := "/files/test.txt"
	specialRecorder := &mockResponseWriter{}
	inputR := httptest.NewRequest(http.MethodPost, fullPath, nil)
	err := createTestFolderAndFile("test.txt", ".txt")
	if err != nil {
		t.Error(err)
		return
	}
	TransferFile(specialRecorder, inputR)
	err = removeTestFolderAndFile()
	if err != nil {
		t.Error(err)
	}

	if specialRecorder.statusCode != 300 {
		t.Errorf("Expected status code 300, got: %d", specialRecorder.statusCode)
	}
}

func TestFileEscape(t *testing.T) {
	inputW := httptest.NewRecorder()
	inputR := httptest.NewRequest(http.MethodPost, "http://localhost/../signal_forms.go", nil)
	TransferFile(inputW, inputR)

	if inputW.Code != 404 {
		t.Errorf("Expected error code 404, got: %d", inputW.Code)
	}
}
