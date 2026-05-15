package handlers

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"go.uber.org/zap"

	"kolsense/internal/vectorstore"
)

// KOLHandler handles KOL profile endpoints.
type KOLHandler struct {
	store vectorstore.Store
	log   *zap.Logger
}

// NewKOLHandler constructs a KOLHandler.
func NewKOLHandler(store vectorstore.Store, log *zap.Logger) *KOLHandler {
	return &KOLHandler{store: store, log: log}
}

// GetProfile is GET /api/v1/kol/:name
func (h *KOLHandler) GetProfile(c *gin.Context) {
	name := c.Param("name")
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "kol name is required"})
		return
	}

	profile, err := h.store.GetKOLProfile(c.Request.Context(), name)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "KOL not found"})
			return
		}
		h.log.Error("get kol profile", zap.Error(err), zap.String("name", name))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch KOL profile"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"kol": gin.H{
			"id":             profile.ID,
			"name":           profile.Name,
			"platform":       profile.Platform,
			"category":       profile.Category,
			"avg_engagement": profile.AvgEngagement,
			"avg_reach":      profile.AvgReach,
			"avg_roi":        profile.AvgROI,
			"follower_count": profile.FollowerCount,
			"fee_min_vnd":    profile.FeeMinVND,
			"fee_max_vnd":    profile.FeeMaxVND,
			"metadata":       profile.Metadata,
			"updated_at":     profile.UpdatedAt,
		},
	})
}
