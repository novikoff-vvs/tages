package server

import (
	"context"
	"fmt"
	"github.com/joho/godotenv"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
	"io"
	"log"
	"math/rand"
	"net"
	"os"
	"sync"
	"testing"

	srv "tages/internal/delivery/grpc"
	pb "tages/pkg/contracts/proto"
)

func init() {
	err := godotenv.Load("../.env.test")
	if err != nil {
		log.Fatal("Error loading .env file")
	}
}

func TestDownloadFile(t *testing.T) {
	uploadLimit := rand.Intn(100)
	// Создаём слушатель для порта
	lis, err := net.Listen("tcp", fmt.Sprintf("localhost:%s", os.Getenv("APP_PORT")))
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	// Инициализация gRPC сервера
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
	wg := &sync.WaitGroup{}
	client := pb.NewFileServiceClient(conn)
	var errChan = make(chan error, uploadLimit+1)
	for i := 0; i <= uploadLimit; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			// Создаем поток для получения данных
			stream, err := client.DownloadFile(context.Background(), &pb.FileDownloadRequest{Filename: "test_file_2.jpg"})
			if err != nil {
				t.Fatalf("failed to call DownloadFile: %v", err)
			}

			var receivedData []byte
			for {
				resp, err := stream.Recv()
				if err == io.EOF {
					break
				}
				if err != nil {
					errChan <- err
					return
				}
				receivedData = append(receivedData, resp.Chunk...)
			}
		}()
	}

	wg.Wait()
	assert.Equal(t, 1, len(errChan))
}
