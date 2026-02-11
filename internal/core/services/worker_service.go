package services

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"video-processor-worker/internal/core/domain"
	"video-processor-worker/internal/core/ports"
)

type workerService struct {
	processor ports.VideoProcessor
	storage   ports.Storage
	repo      ports.VideoRepository
}

func NewWorkerService(p ports.VideoProcessor, s ports.Storage, r ports.VideoRepository) *workerService {
	return &workerService{
		processor: p,
		storage:   s,
		repo:      r,
	}
}

func (s *workerService) ProcessVideoByID(ctx context.Context, videoID int64) error {
	log.Printf("📥 Processing request for video ID: %d", videoID)

	video, err := s.repo.GetByID(ctx, videoID)
	if err != nil {
		return fmt.Errorf("error fetching video %d: %w", videoID, err)
	}

	if video == nil {
		return fmt.Errorf("video %d not found", videoID)
	}

	if video.Status != domain.StatusPending {
		log.Printf("ℹ️ Video %d already in status %s, skipping", videoID, video.Status)
		return nil
	}

	return s.processVideo(ctx, video)
}

func (s *workerService) processVideo(ctx context.Context, video *domain.Video) error {
	// Update status to PROCESSING
	video.Status = domain.StatusProcessing
	video.Message = "Processamento iniciado..."
	if err := s.repo.Update(ctx, video); err != nil {
		return fmt.Errorf("error updating video status: %w", err)
	}

	videoPath := s.storage.GetUploadPath(video.Filename)
	uniqueJobID := strings.TrimSuffix(video.Filename, filepath.Ext(video.Filename))

	log.Printf("🎬 Extracting frames for video ID: %d (%s)", video.ID, video.Filename)
	frames, err := s.processor.ExtractFrames(videoPath, uniqueJobID)
	if err != nil {
		log.Printf("❌ Error extracting frames for video %d: %v", video.ID, err)
		video.Status = domain.StatusFailed
		video.Message = "Erro no processamento: " + err.Error()
		s.repo.Update(ctx, video)
		s.storage.DeleteFile(videoPath)
		return err
	}

	log.Printf("📦 Creating ZIP for video ID: %d", video.ID)
	zipFilename := fmt.Sprintf("frames_%s.zip", uniqueJobID)
	err = s.storage.SaveZip(zipFilename, frames)
	if err != nil {
		log.Printf("❌ Error saving ZIP for video %d: %v", video.ID, err)
		video.Status = domain.StatusFailed
		video.Message = "Erro ao criar ZIP: " + err.Error()
		s.repo.Update(ctx, video)
		s.storage.DeleteFile(videoPath)
		return err
	}

	// Cleanup
	s.storage.DeleteFile(videoPath)
	if len(frames) > 0 {
		tempDir := filepath.Dir(frames[0])
		s.storage.DeleteDir(tempDir)
	}

	// Final Update
	video.Status = domain.StatusCompleted
	video.ZipPath = zipFilename
	video.FrameCount = len(frames)
	video.Message = fmt.Sprintf("Processamento concluído! %d frames extraídos.", len(frames))
	if err := s.repo.Update(ctx, video); err != nil {
		log.Printf("❌ Error final updating video %d: %v", video.ID, err)
		return err
	}

	log.Printf("✅ Video %d processed successfully", video.ID)
	return nil
}
