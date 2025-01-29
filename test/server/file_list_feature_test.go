package server

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"net"
	"os"
	"sync"
	"testing"
	"time"

	srv "tages/internal/delivery/grpc"
	pb "tages/pkg/contracts/proto"

	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
)

func TestMaxConcurrentListFiles(t *testing.T) {
	rand.Seed(time.Now().UnixNano())
	limit := rand.Intn(100)
	increment := rand.Intn(50)
	maxConnections := limit + increment

	lis, err := net.Listen("tcp", fmt.Sprintf("localhost:%d", 50052))

	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	grpcServer := grpc.NewServer()
	uploadDir := os.Getenv("UPLOAD_DIR_PATH")

	if len(uploadDir) == 0 {
		log.Fatalf("upload dir path is empty")
	}

	pb.RegisterFileServiceServer(grpcServer, srv.NewServer(0, limit, uploadDir))

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

	assert.Equal(t, increment, len(errChan))
}
