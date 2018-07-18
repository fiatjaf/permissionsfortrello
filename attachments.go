package main

import (
	"bytes"
	"io"
	"io/ioutil"
	"mime/multipart"
	"net/http"
	"os"

	"github.com/minio/minio-go"
)

func saveToS3(id, trelloURL string) (err error) {
	// download file from trello
	file, err := ioutil.TempFile("", "trello-permissions-")
	if err != nil {
		file.Close()
		return
	}

	resp, err := http.Get(trelloURL)
	if err != nil {
		file.Close()
		return
	}

	_, err = io.Copy(file, resp.Body)
	if err != nil {
		file.Close()
		return
	}
	file.Close()

	// upload to s3
	_, err = ms3.FPutObject(s.S3BucketName, id, file.Name(),
		minio.PutObjectOptions{})

	return
}

func restoreFromS3(attId, attName, cardId, token string) (err error) {
	file, err := ioutil.TempFile("", "trello-permissions-")
	if err != nil {
		return
	}

	path := file.Name()
	file.Close()

	// download file from s3
	err = ms3.FGetObject(s.S3BucketName,
		attId,
		path,
		minio.GetObjectOptions{})
	if err != nil {
		return
	}

	// upload files to trello
	file, err = os.Open(path)
	if err != nil {
		return
	}
	defer file.Close()

	post := &bytes.Buffer{}
	writer := multipart.NewWriter(post)
	part, err := writer.CreateFormFile("file", path)
	if err != nil {
		return
	}

	_, err = io.Copy(part, file)
	if err != nil {
		return
	}

	writer.WriteField("name", attName)
	writer.WriteField("key", s.TrelloApiKey)
	writer.WriteField("token", token)

	err = writer.Close()
	if err != nil {
		return
	}

	req, err := http.NewRequest("POST",
		"https://api.trello.com/1/cards/"+cardId+"/attachments", post)
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	_, err = http.DefaultClient.Do(req)
	return
}

func deleteFromS3(id string) (err error) {
	return ms3.RemoveObject(s.S3BucketName, id)
}
