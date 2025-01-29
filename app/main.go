package main

import (
	"fmt"
	"log"
	"net"
	"os"
	"strconv"
	server "tages/internal/delivery/grpc"
	pb "tages/pkg/contracts/proto"

	"github.com/joho/godotenv"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

var (
	uploadDownloadLimit int
	listFilesLimit      int
	port                int
)

func init() {
	err := godotenv.Load(".env.test")
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	port, err = strconv.Atoi(os.Getenv("APP_PORT"))
	uploadDownloadLimit, err = strconv.Atoi(os.Getenv("UPLOAD_DOWNLOAD_LIMIT"))
	listFilesLimit, err = strconv.Atoi(os.Getenv("LIST_FILES_LIMIT"))
}

func main() {
	lis, err := net.Listen("tcp", fmt.Sprintf("localhost:%d", port))
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	s := grpc.NewServer()
	uploadDir := os.Getenv("UPLOAD_DIR_PATH")
	if len(uploadDir) == 0 {
		log.Fatalf("upload dir path is empty")
	}
	pb.RegisterFileServiceServer(s, server.NewServer(uploadDownloadLimit, listFilesLimit, uploadDir))
	reflection.Register(s)

	log.Printf("Server listening at %v", lis.Addr())
	if err := s.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
