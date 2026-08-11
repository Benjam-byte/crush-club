package main

import (
	"bytes"
	"encoding/base64"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func singlePhotoRequest(t *testing.T, fileName string, data []byte) *http.Request {
	t.Helper()
	var requestBody bytes.Buffer
	writer := multipart.NewWriter(&requestBody)
	part, err := writer.CreateFormFile("photo", fileName)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest("POST", "/photos", &requestBody)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	return request
}

func TestStageSinglePhotoRequest(t *testing.T) {
	pngData, err := base64.StdEncoding.DecodeString(onePixelPNG)
	if err != nil {
		t.Fatal(err)
	}
	storagePath := t.TempDir()
	api := &api{photoStoragePath: storagePath}
	request := singlePhotoRequest(t, "photo.png", pngData)

	photoList, err := api.stagePhotosFromRequest(request, "photo", 1)
	if err != nil {
		t.Fatalf("stagePhotosFromRequest() error = %v", err)
	}
	defer removeTemporaryPhotos(photoList)
	if len(photoList) != 1 || photoList[0].contentType != "image/png" ||
		photoList[0].width != 1 || photoList[0].height != 1 {
		t.Fatalf("staged photo = %#v", photoList)
	}
	if _, err := os.Stat(photoList[0].temporaryPath); err != nil {
		t.Fatalf("temporary photo is missing: %v", err)
	}
}

func TestStageSinglePhotoRejectsUnsupportedContent(t *testing.T) {
	api := &api{photoStoragePath: t.TempDir()}
	request := singlePhotoRequest(t, "photo.heic", []byte("not-a-supported-image"))

	_, err := api.stagePhotosFromRequest(request, "photo", 1)
	var requestError photoHTTPError
	if !errors.As(err, &requestError) || requestError.code != "unsupported_photo" {
		t.Fatalf("error = %#v, want unsupported_photo", err)
	}
}

func TestPlayerPhotoPositionValidation(t *testing.T) {
	for _, test := range []struct {
		value string
		want  int
		valid bool
	}{
		{"1", 1, true},
		{"4", 4, true},
		{"0", 0, false},
		{"5", 0, false},
		{"photo", 0, false},
	} {
		request := httptest.NewRequest("GET", "/photos/"+test.value, nil)
		request.SetPathValue("position", test.value)
		position, err := playerPhotoPosition(request)
		if test.valid && (err != nil || position != test.want) {
			t.Fatalf("position %q = %d, %v", test.value, position, err)
		}
		if !test.valid && err == nil {
			t.Fatalf("position %q should be rejected", test.value)
		}
	}
}
