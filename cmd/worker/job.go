package main

import (
	"context"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"golang.anshulg.com/popcorntime/go_encoder/api/proto"
	"golang.anshulg.com/popcorntime/go_encoder/internal/encoder"
)

type JobServer struct {
	proto.UnimplementedJobServiceServer
	logger *zap.Logger
	cfg    aws.Config

	tempPath string

	mtx sync.Mutex
	job encoder.EncodeJob
}

func (s *JobServer) Start(ctx context.Context, request *proto.JobStartRequest) (*proto.JobStartResponse, error) {
	job := request.Job
	if job == nil {
		return nil, status.Error(codes.InvalidArgument, "job not provided")
	}

	s.mtx.Lock()
	defer s.mtx.Unlock()
	if s.job != nil {
		return nil, status.Error(codes.AlreadyExists, "job already running")
	}

	s.job = encoder.NewEncodeJob(s.logger, s.cfg, s.tempPath)
	s.job.SetSourcePath(job.SourcePath).
		SetDestPath(job.DestPath).
		SetBitrate(job.Bitrate).
		SetCodec(job.Codec)

	go s.job.Start()

	go func() {
		s.job.Wait()
		s.job = nil
	}()

	return &proto.JobStartResponse{}, nil
}

func (s *JobServer) Cancel(ctx context.Context, request *proto.JobCancelRequest) (*proto.JobCancelResponse, error) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	if s.job == nil {
		return nil, status.Errorf(codes.NotFound, "no current job found")
	}

	s.job.Cancel()
	s.job = nil

	return &proto.JobCancelResponse{}, nil
}

func (s *JobServer) Status(request *proto.JobStatusRequest, stream proto.JobService_StatusServer) error {
	if s.job == nil {
		return status.Errorf(codes.NotFound, "no current job found")
	}
	jobStatus := s.job.GetStatus()

	for msg := range jobStatus {
		if err := stream.Send(msg); err != nil {
			return err
		}
	}

	return nil
}
