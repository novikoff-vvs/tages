package grpc

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"syscall"
	pb "tages/pkg/contracts/proto"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type Server struct {
	pb.UnimplementedFileServiceServer
	uploadSem    chan struct{}
	listFilesSem chan struct{}
	uploadDir    string
}

func NewServer(uploadDownloadLimit, listFilesLimit int, uploadDir string) *Server {
	return &Server{
		uploadSem:    make(chan struct{}, uploadDownloadLimit),
		listFilesSem: make(chan struct{}, listFilesLimit),
		uploadDir:    uploadDir,
	}
}

func (s *Server) UploadFile(stream pb.FileService_UploadFileServer) error {
	select {
	case s.uploadSem <- struct{}{}:
		defer func() { <-s.uploadSem }()
	default:
		return status.Error(codes.ResourceExhausted, "upload limit reached")
	}

	var fileInfo *pb.FileInfo
	var file *os.File
	defer file.Close()

	for {
		req, err := stream.Recv()
		if err == io.EOF {
			return stream.SendAndClose(&pb.FileUploadResponse{
				Success: true,
				Message: "File uploaded successfully",
			})
		}
		if err != nil {
			return err
		}

		switch data := req.Data.(type) {
		case *pb.FileUploadRequest_Info:
			fileInfo = data.Info
			filePath := filepath.Join(s.uploadDir, fileInfo.Filename)

			file, err = os.Create(filePath)
			if err != nil {
				return err
			}

		case *pb.FileUploadRequest_Chunk:
			if file == nil {
				return status.Error(codes.InvalidArgument, "file info not received")
			}
			_, err = file.Write(data.Chunk)
			if err != nil {
				return err
			}
		}
	}
}

func (s *Server) DownloadFile(req *pb.FileDownloadRequest, stream pb.FileService_DownloadFileServer) error {
	select {
	case s.uploadSem <- struct{}{}:
		defer func() { <-s.uploadSem }()
	default:
		return status.Error(codes.ResourceExhausted, "download limit reached")
	}

	filePath := filepath.Join(s.uploadDir, req.Filename)
	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer func(file *os.File) {
		err := file.Close()
		if err != nil {
			log.Printf("failed to close file: %v", err)
		}
	}(file)

	buffer := make([]byte, 1024*1024)
	for {
		n, err := file.Read(buffer)
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if err := stream.Send(&pb.FileDownloadResponse{
			Chunk: buffer[:n],
		}); err != nil {
			log.Printf("failed to send chunk: %v", err)
		}
	}
	return nil
}

func (s *Server) ListFiles(ctx context.Context, req *pb.ListFilesRequest) (*pb.ListFilesResponse, error) {
	select {
	case s.listFilesSem <- struct{}{}: // получаем семафор
		defer func() { <-s.listFilesSem }()
	default:
		return nil, errors.New("read limit")
	}

	var files []*pb.FileMetadata

	err := filepath.Walk(s.uploadDir, func(path string, info os.FileInfo, err error) error {

		if err != nil {
			return err
		}
		if !info.IsDir() {
			c, u := getFileInfo(path)
			createdAt := timestamppb.New(time.Unix(c.Sec, c.Nsec))
			updatedAt := timestamppb.New(time.Unix(u.Sec, u.Nsec))
			files = append(files, &pb.FileMetadata{
				Filename:  info.Name(),
				CreatedAt: createdAt,
				UpdatedAt: updatedAt,
			})
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return &pb.ListFilesResponse{Files: files}, nil
}

func getFileInfo(filePath string) (created, updated syscall.Timespec) {
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		fmt.Println("get file error:", err)
		return
	}
	stat := fileInfo.Sys().(*syscall.Stat_t)

	creationTime := stat.Ctimespec
	modificationTime := stat.Mtimespec

	return creationTime, modificationTime
}
