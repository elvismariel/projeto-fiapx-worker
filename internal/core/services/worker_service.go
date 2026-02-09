package services

import (
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"time"
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

func (s *workerService) Start() {
	log.Println("🚀 Worker started, polling for tasks (fallback)...")
	for {
		videos, err := s.repo.GetPending()
		if err != nil {
			log.Printf("❌ Error polling videos: %v", err)
			time.Sleep(10 * time.Second)
			continue
		}

		if len(videos) == 0 {
			time.Sleep(5 * time.Second)
			continue
		}

		for _, v := range videos {
			log.Printf("🎬 Polling found video ID: %d (%s)", v.ID, v.Filename)
			s.processVideo(&v)
		}
	}
}

func (s *workerService) HandleUploadEvent(videoID int64, filename string) error {
	log.Printf("📥 Handling upload event for video ID: %d", videoID)

	// Get video from repo to ensure we have the latest state
	video, err := s.repo.GetByID(videoID)
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

	s.processVideo(video)
	return nil
}

func (s *workerService) processVideo(video *domain.Video) {
	// Update status to PROCESSING
	video.Status = domain.StatusProcessing
	video.Message = "Processamento iniciado..."
	s.repo.Update(video)

	// In the new API version, video.Filename is already the unique filename (e.g., 20260205_171420_video.mp4)
	videoPath := s.storage.GetUploadPath(video.Filename)

	// Use the unique video filename (without extension) as the base for ZIP and folders
	uniqueJobID := strings.TrimSuffix(video.Filename, filepath.Ext(video.Filename))

	frames, err := s.processor.ExtractFrames(videoPath, uniqueJobID)
	if err != nil {
		log.Printf("❌ Error processing video %d: %v", video.ID, err)
		video.Status = domain.StatusFailed
		video.Message = "Erro no processamento: " + err.Error()
		s.repo.Update(video)
		s.storage.DeleteFile(videoPath)
		return
	}

	zipFilename := fmt.Sprintf("frames_%s.zip", uniqueJobID)
	err = s.storage.SaveZip(zipFilename, frames)
	if err != nil {
		log.Printf("❌ Error saving ZIP for video %d: %v", video.ID, err)
		video.Status = domain.StatusFailed
		video.Message = "Erro ao criar ZIP: " + err.Error()
		s.repo.Update(video)
		s.storage.DeleteFile(videoPath)
		return
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
	s.repo.Update(video)

	log.Printf("✅ Video %d processed successfully", video.ID)
}
