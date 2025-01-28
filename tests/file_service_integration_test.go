package file_service_integration_test

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

	"google.golang.org/grpc/credentials/insecure"

	srv "tages/internal/delivery/grpc"
	pb "tages/pkg/contracts/proto"

	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
	"google.golang.org/grpc/test/bufconn"
)

const (
	bufSize = 1024 * 1024 // 1MB
)

var listener *bufconn.Listener

func init() {
	listener = bufconn.Listen(bufSize)
}

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
	uploadLimit := 2
	uploadInstance := 3
	// Создаём слушатель для порта
	lis, err := net.Listen("tcp", fmt.Sprintf("localhost:%d", 50052))
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	// Инициализация gRPC сервера
	grpcServer := grpc.NewServer()

	pb.RegisterFileServiceServer(grpcServer, srv.NewServer(uploadLimit, 10))

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
			filePath := fmt.Sprintf("/Users/highjin/GolandProjects/tages/uploads_test/test_file_%d.jpg", fileNum) // Предполагается, что файлы называются file0.txt, file1.txt и т.д.
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

			stream.CloseAndRecv()
		}(i)
	}

	wg.Wait()

	// Сравниваем два целых числа
	assert.NotEqual(t, 0, len(errChan), "Ожидаемые значения должны быть равны")
}

func TestMaxConcurrentListFiles(t *testing.T) {
	loadLimit := 100
	rnd := rand.Intn(10)
	maxConnections := loadLimit + rnd
	// Создаём слушатель для порта
	lis, err := net.Listen("tcp", fmt.Sprintf("localhost:%d", 50052))
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	// Инициализация gRPC сервера
	grpcServer := grpc.NewServer()

	pb.RegisterFileServiceServer(grpcServer, srv.NewServer(1, loadLimit))

	go func() {
		if err := grpcServer.Serve(lis); err != nil {
			log.Fatalf("failed to serve: %v", err)
		}
	}()

	conn := connect()
	defer conn.Close()

	client := pb.NewFileServiceClient(conn)

	var wg sync.WaitGroup
	var errChan = make(chan error, maxConnections)

	for i := 0; i < maxConnections; i++ {
		wg.Add(1)
		go func(connNum int) {
			defer wg.Done()

			_, err := client.ListFiles(context.Background(), &pb.ListFilesRequest{})
			if err != nil {
				errChan <- fmt.Errorf("failed to list files on connection %d: %v", connNum, err)
				return
			}
		}(i)
	}

	wg.Wait()
	close(errChan)

	assert.NotEqual(t, 0, len(errChan))
	// Проверка на наличие ошибок
	for err := range errChan {
		log.Println(err)
	}
}
