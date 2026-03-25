package nvr

import (
	"fmt"
	"os"
	"time"

	"github.com/bluenviron/mediamtx/internal/nvr/api"
)

// startFragmentBackfill runs a background goroutine that indexes any recordings
// that don't yet have fragment metadata. It processes newest-first so recent
// playback benefits immediately.
func (n *NVR) startFragmentBackfill() {
	go func() {
		// Small delay to let the server finish starting up.
		time.Sleep(5 * time.Second)

		recs, err := n.database.GetUnindexedRecordings()
		if err != nil {
			fmt.Fprintf(os.Stderr, "NVR: fragment backfill query failed: %v\n", err)
			return
		}

		if len(recs) == 0 {
			return
		}

		fmt.Fprintf(os.Stderr, "NVR: starting fragment backfill for %d recordings\n", len(recs))

		indexed := 0
		for _, rec := range recs {
			if rec.Format != "fmp4" {
				continue
			}

			// Check file exists.
			if _, err := os.Stat(rec.FilePath); os.IsNotExist(err) {
				continue
			}

			initSize, fragments, err := api.ScanFragments(rec.FilePath)
			if err != nil {
				fmt.Fprintf(os.Stderr, "NVR: backfill scan failed for %s: %v\n", rec.FilePath, err)
				continue
			}

			if err := n.database.UpdateRecordingInitSize(rec.ID, initSize); err != nil {
				fmt.Fprintf(os.Stderr, "NVR: backfill init_size update failed for recording %d: %v\n", rec.ID, err)
			}

			dbFrags := buildDBFragments(rec.ID, fragments)

			if err := n.database.InsertFragments(rec.ID, dbFrags); err != nil {
				fmt.Fprintf(os.Stderr, "NVR: backfill insert failed for recording %d: %v\n", rec.ID, err)
				continue
			}

			indexed++
			if indexed%100 == 0 {
				fmt.Fprintf(os.Stderr, "NVR: fragment backfill progress: %d/%d\n", indexed, len(recs))
			}
		}

		fmt.Fprintf(os.Stderr, "NVR: fragment backfill complete: indexed %d recordings\n", indexed)
	}()
}
