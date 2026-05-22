package handlers

import (
	"net/http"
	"path/filepath"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"kolsense/internal/ingestion"
	"kolsense/internal/vectorstore"
)

// IngestHandler handles PDF ingestion requests.
type IngestHandler struct {
	svc *ingestion.Service
	log *zap.Logger
}

// NewIngestHandler constructs an IngestHandler.
func NewIngestHandler(svc *ingestion.Service, log *zap.Logger) *IngestHandler {
	return &IngestHandler{svc: svc, log: log}
}

type ingestRequest struct {
	FilePath string `json:"file_path" binding:"required"`
	KOLName  string `json:"kol_name"`
	DocType  string `json:"doc_type"`
}

type ingestResponse struct {
	File         string `json:"file"`
	ChunksStored int    `json:"chunks_stored"`
}

// IngestPDF is POST /api/v1/ingest/pdf
func (h *IngestHandler) IngestPDF(c *gin.Context) {
	var req ingestRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	cfg := ingestion.DefaultIngestConfig()
	cfg.KOLName = req.KOLName
	if req.DocType != "" {
		cfg.DocType = vectorstore.DocType(req.DocType)
	}

	n, err := h.svc.IngestFile(c.Request.Context(), req.FilePath, cfg)
	if err != nil {
		h.log.Error("ingest failed", zap.Error(err), zap.String("file", req.FilePath))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, ingestResponse{
		File:         filepath.Base(req.FilePath),
		ChunksStored: n,
	})
}
