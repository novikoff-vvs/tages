package server

import (
	"context"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net"
	"os"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc/credentials/insecure"

	srv "tages/internal/delivery/grpc"
	pb "tages/pkg/contracts/proto"

	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
)

const (
	bufSize = 1024 * 1024 // 1MB
)

func connect() *grpc.ClientConn {
	var opts = []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	}

	conn, err := grpc.NewClient("localhost:50052", opts...)
	if err != nil {
		panic(err)
	}
	return conn
}

func TestMaxConcurrentUploads(t *testing.T) {
	rand.Seed(time.Now().UnixNano())
	uploadLimit := rand.Intn(4)
	increment := rand.Intn(5)

	uploadInstance := uploadLimit + increment
	lis, err := net.Listen("tcp", fmt.Sprintf("localhost:%s", os.Getenv("APP_PORT")))
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	grpcServer := grpc.NewServer()
	uploadDir := os.Getenv("UPLOAD_DIR_PATH")
	if len(uploadDir) == 0 {
		log.Fatalf("upload dir path is empty")
	}
	pb.RegisterFileServiceServer(grpcServer, srv.NewServer(uploadLimit, 0, uploadDir))

	go func() {
		if err := grpcServer.Serve(lis); err != nil {
			log.Fatalf("failed to serve: %v", err)
		}
	}()

	conn := connect()
	defer conn.Close()

	client := pb.NewFileServiceClient(conn)

	var errChan = make(chan error, 10)

	var wg sync.WaitGroup
	for i := 0; i < uploadInstance; i++ {
		wg.Add(1)
		go func(fileNum int) {
			defer wg.Done()
			stream, err := client.UploadFile(context.Background())
			if err != nil {
				t.Errorf("failed to upload file %d: %v", fileNum, err)
				return
			}

			// Чтение файла из папки uploads_test
			filePath := fmt.Sprintf(os.Getenv("UPLOAD_FROM_DIR_PATH_PATTERN"), fileNum) // Предполагается, что файлы называются file0.txt, file1.txt и т.д.
			file, err := os.Open(filePath)
			if err != nil {
				errChan <- err
				return
			}
			defer file.Close()
			err = stream.Send(&pb.FileUploadRequest{Data: &pb.FileUploadRequest_Info{Info: &pb.FileInfo{Filename: fmt.Sprintf("test_file_%d.jpg", fileNum)}}})
			// Отправка данных файла
			buffer := make([]byte, 1024) // Размер буфера
			for {
				n, err := file.Read(buffer)
				if err != nil {
					if err == io.EOF {
						break
					}
					errChan <- err
					return
				}
				err = stream.Send(&pb.FileUploadRequest{Data: &pb.FileUploadRequest_Chunk{Chunk: buffer[:n]}})
				if err != nil {
					errChan <- err
					return
				}
			}

			_, err = stream.CloseAndRecv()
			if err != nil {
				log.Fatalf("failed to close stream: %v", err)
			}
		}(i)
	}

	wg.Wait()

	// Сравниваем два целых числа
	assert.Equal(t, increment, len(errChan), "Ожидаемые значения должны быть равны")
}
