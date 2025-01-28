package grpc

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
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

func NewServer(uploadDownloadLimit, listFilesLimit int) *Server {
	return &Server{
		uploadSem:    make(chan struct{}, uploadDownloadLimit),
		listFilesSem: make(chan struct{}, listFilesLimit),
		uploadDir:    "/Users/highjin/GolandProjects/tages/uploads",
	}
}

func (s *Server) UploadFile(stream pb.FileService_UploadFileServer) error {
	select {
	case s.uploadSem <- struct{}{}: // получаем семафор
		defer func() { <-s.uploadSem }() // освобождаем семафор
	default:
		return status.Error(codes.ResourceExhausted, "upload limit reached") // канал заполнен
	}

	var fileInfo *pb.FileInfo
	var file *os.File

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
			defer file.Close()

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
	case s.uploadSem <- struct{}{}: // получаем семафор
		defer func() { <-s.uploadSem }() // освобождаем семафор
	default:
		return status.Error(codes.ResourceExhausted, "download limit reached") // канал заполнен
	}

	filePath := filepath.Join(s.uploadDir, req.Filename)
	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	buffer := make([]byte, 1024)
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
			return err
		}
	}
	return nil
}

func (s *Server) ListFiles(ctx context.Context, req *pb.ListFilesRequest) (*pb.ListFilesResponse, error) {
	select {
	case s.listFilesSem <- struct{}{}: // получаем семафор
		defer func() { <-s.listFilesSem }() // освобождаем семафор
	default:
		return nil, errors.New("read limit") // канал заполнен
	}
	time.Sleep(5 * time.Second)
	var files []*pb.FileMetadata

	err := filepath.Walk(s.uploadDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			files = append(files, &pb.FileMetadata{
				Filename:  info.Name(),
				CreatedAt: timestamppb.New(info.ModTime()),
				UpdatedAt: timestamppb.New(info.ModTime()),
			})
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	return &pb.ListFilesResponse{Files: files}, nil
}
