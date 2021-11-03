package main

import (
	"bytes"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/gabriel-vasile/mimetype"
	"github.com/joho/godotenv"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"

	"github.com/tomekwlod/utils/env"
)

const (
	maxPartSize = int64(5 * 1024 * 1024) // 5MB
	maxRetries  = 3
)

func main() {
	filename := os.Args[1]

	if filename == "" {
		log.Fatal("filename cannot be empty. Pass it as a first argument like so: `command /local/file/path/file.ext /do/target/path/`")
	}

	if !fileExists(filename) {
		log.Fatalf("file doesnt exist, %s", filename)
	}

	remotepath := os.Args[2]

	if remotepath == "" {
		log.Fatal("do_target_path cannot be empty. Pass it as a second argument like so: `command /local/file/path/file.ext /do/target/path/`")
	}

	err := godotenv.Load()

	if err != nil {
		log.Fatal("No .env file detected")
	}

	creds := credentials.NewStaticCredentials(env.Env("DO_S3_KEY", ""), env.Env("DO_S3_SECRET", ""), "")
	_, err = creds.Get()
	if err != nil {
		fmt.Printf("bad credentials: %s", err)
	}
	cfg := aws.NewConfig().WithRegion(env.Env("DO_S3_REGION", "us-east-1")).WithCredentials(creds)
	if env.Env("DO_S3_ENDPOINT", "") != "" {
		cfg.WithEndpoint(env.Env("DO_S3_ENDPOINT", ""))
	}

	svc := s3.New(session.New(), cfg)

	file, err := os.Open(filename)
	if err != nil {
		fmt.Printf("err opening file: %s", err)
		return
	}
	defer file.Close()

	fileInfo, _ := file.Stat()
	size := fileInfo.Size()
	buffer := make([]byte, size)

	// contentType, err := GetFileContentType(file)
	// contentType := http.DetectContentType(buffer)

	// reading the content type will result in Seek to be in a wrong position
	// we have to then reset the position with file.Seek(0,0)
	contentType, err := mimetype.DetectReader(file)
	_, err = file.Seek(0, 0)
	if err != nil {
		fmt.Println(err.Error())
		return
	}
	file.Read(buffer)

	path := filepath.Join("/", remotepath, file.Name())
	input := &s3.CreateMultipartUploadInput{
		Bucket:      aws.String(env.Env("DO_S3_BUCKET", "backup")),
		Key:         aws.String(path),
		ContentType: aws.String(contentType.String()),
	}

	resp, err := svc.CreateMultipartUpload(input)
	if err != nil {
		fmt.Println(err.Error())
		return
	}
	fmt.Println("Created multipart upload request")

	var curr, partLength int64
	var remaining = size
	var completedParts []*s3.CompletedPart
	partNumber := 1
	for curr = 0; remaining != 0; curr += partLength {
		if remaining < maxPartSize {
			partLength = remaining
		} else {
			partLength = maxPartSize
		}
		completedPart, err := uploadPart(svc, resp, buffer[curr:curr+partLength], partNumber)
		if err != nil {
			fmt.Println(err.Error())
			err := abortMultipartUpload(svc, resp)
			if err != nil {
				fmt.Println(err.Error())
			}
			return
		}
		remaining -= partLength
		partNumber++
		completedParts = append(completedParts, completedPart)
	}

	completeResponse, err := completeMultipartUpload(svc, resp, completedParts)
	if err != nil {
		fmt.Println(err.Error())
		return
	}

	fmt.Printf("Successfully uploaded file: %s\n", completeResponse.String())
}

func completeMultipartUpload(svc *s3.S3, resp *s3.CreateMultipartUploadOutput, completedParts []*s3.CompletedPart) (*s3.CompleteMultipartUploadOutput, error) {
	completeInput := &s3.CompleteMultipartUploadInput{
		Bucket:   resp.Bucket,
		Key:      resp.Key,
		UploadId: resp.UploadId,
		MultipartUpload: &s3.CompletedMultipartUpload{
			Parts: completedParts,
		},
	}
	return svc.CompleteMultipartUpload(completeInput)
}

func uploadPart(svc *s3.S3, resp *s3.CreateMultipartUploadOutput, fileBytes []byte, partNumber int) (*s3.CompletedPart, error) {
	tryNum := 1
	partInput := &s3.UploadPartInput{
		Body:          bytes.NewReader(fileBytes),
		Bucket:        resp.Bucket,
		Key:           resp.Key,
		PartNumber:    aws.Int64(int64(partNumber)),
		UploadId:      resp.UploadId,
		ContentLength: aws.Int64(int64(len(fileBytes))),
	}

	for tryNum <= maxRetries {
		uploadResult, err := svc.UploadPart(partInput)
		if err != nil {
			if tryNum == maxRetries {
				if aerr, ok := err.(awserr.Error); ok {
					return nil, aerr
				}
				return nil, err
			}
			fmt.Printf("Retrying to upload part #%v\n", partNumber)
			tryNum++
		} else {
			fmt.Printf("Uploaded part #%v\n", partNumber)
			return &s3.CompletedPart{
				ETag:       uploadResult.ETag,
				PartNumber: aws.Int64(int64(partNumber)),
			}, nil
		}
	}
	return nil, nil
}

func abortMultipartUpload(svc *s3.S3, resp *s3.CreateMultipartUploadOutput) error {
	fmt.Println("Aborting multipart upload for UploadId#" + *resp.UploadId)
	abortInput := &s3.AbortMultipartUploadInput{
		Bucket:   resp.Bucket,
		Key:      resp.Key,
		UploadId: resp.UploadId,
	}
	_, err := svc.AbortMultipartUpload(abortInput)
	return err
}

func GetFileContentType(out *os.File) (string, error) {

	// Only the first 512 bytes are used to sniff the content type.
	buffer := make([]byte, 512)

	_, err := out.Read(buffer)
	if err != nil {
		return "", err
	}

	// Use the net/http package's handy DectectContentType function. Always returns a valid
	// content-type by returning "application/octet-stream" if no others seemed to match.
	contentType := http.DetectContentType(buffer)

	return contentType, nil
}

func fileExists(filename string) bool {
	info, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false
	}
	return !info.IsDir()
}
