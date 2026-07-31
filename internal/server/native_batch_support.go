package server

import (
	"context"
	"strings"

	batchstore "github.com/icehugh/thinroute/internal/batch"
	"github.com/icehugh/thinroute/internal/batchrewrite"
	"github.com/icehugh/thinroute/internal/core"
)

func (h *Handler) cleanupPreparedBatchInputFile(ctx context.Context, providerType, fileID string) {
	fileID = strings.TrimSpace(fileID)
	if fileID == "" {
		return
	}
	files, ok := h.provider.(core.NativeFileRoutableProvider)
	if !ok {
		return
	}
	batchrewrite.CleanupFile(ctx, files, providerType, fileID, "")
}

func (h *Handler) cleanupStoredBatchRewrittenInputFile(ctx context.Context, stored *batchstore.StoredBatch) bool {
	if stored == nil || stored.Batch == nil {
		return false
	}
	fileID := strings.TrimSpace(stored.RewrittenInputFileID)
	if fileID == "" {
		return false
	}
	nativeFiles, err := h.nativeFiles().router()
	if err != nil {
		return false
	}
	if !batchrewrite.CleanupFile(ctx, nativeFiles, stored.Batch.Provider, fileID, "", "batch_id", stored.Batch.ID) {
		return false
	}
	stored.RewrittenInputFileID = ""
	return true
}
