package codec

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	sqlc "github.com/shifter-io/shifter/internal/db/sqlc"
	"github.com/shifter-io/shifter/internal/profile/codecs"
)

// placeholderCodecMarker is the string written by migration 0050 into the
// Itron+KINMY codec_js column. RunCatalogDriftCheck detects this prefix and
// overwrites the placeholder with the real embedded codec source rather than
// treating the mismatch as a customer edit (T-07-03-01 mitigation).
const placeholderCodecMarker = "// placeholder — replaced at boot by RunCatalogSeedSync"

// RunCatalogDriftCheck reconciles existing device_profile rows with the
// embedded codec catalog. For rows where catalog_source IS NOT NULL AND
// customer_edited = FALSE, it compares codec_js hash to the embedded source
// hash. Differing hashes either:
//
//   (a) overwrite codec_js with the embedded source if the existing value
//       starts with placeholderCodecMarker (migration placeholder — not an
//       operator edit), or
//   (b) flip customer_edited = TRUE (operator tweaked their codec).
//
// D-31 — drift detection at boot. Idempotent: running twice produces the
// same final state.
func RunCatalogDriftCheck(ctx context.Context, pool *pgxpool.Pool) error {
	q := sqlc.New(pool)
	rows, err := q.ListCatalogProfilesForDriftCheck(ctx)
	if err != nil {
		return err
	}
	for _, r := range rows {
		if r.CatalogSource == nil || *r.CatalogSource == "" {
			continue
		}
		embedded := codecs.CodecBySlug(*r.CatalogSource)
		if embedded == "" {
			slog.Warn("catalog drift check: no embedded codec for slug",
				"slug", r.Slug,
				"catalog_source", *r.CatalogSource)
			continue
		}

		existingHash := sha256.Sum256([]byte(normalizeLines(r.CodecJs)))
		embeddedHash := sha256.Sum256([]byte(normalizeLines(embedded)))

		if existingHash == embeddedHash {
			// Hashes match — nothing to do.
			continue
		}

		if strings.HasPrefix(strings.TrimSpace(r.CodecJs), placeholderCodecMarker) {
			// Migration placeholder — overwrite with real embedded source.
			if err := q.OverwriteProfileCodec(ctx, sqlc.OverwriteProfileCodecParams{
				ID:      r.ID,
				CodecJs: embedded,
			}); err != nil {
				slog.Error("catalog drift: overwrite placeholder failed",
					"slug", r.Slug, "err", err)
				continue
			}
			slog.Info("catalog drift: placeholder replaced with embedded codec",
				"slug", r.Slug,
				"embedded_hash", hex.EncodeToString(embeddedHash[:8]))
			continue
		}

		// Hashes differ and not a placeholder — operator has edited this codec.
		if err := q.MarkProfileCustomerEdited(ctx, r.ID); err != nil {
			slog.Error("catalog drift: mark customer_edited failed",
				"slug", r.Slug, "err", err)
			continue
		}
		slog.Info("catalog drift: customer-edited codec detected",
			"slug", r.Slug,
			"existing_hash", hex.EncodeToString(existingHash[:8]),
			"embedded_hash", hex.EncodeToString(embeddedHash[:8]))
	}
	return nil
}

// normalizeLines normalizes line endings to LF before hashing to ensure
// platform-consistent hash results (Windows CRLF safety).
func normalizeLines(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\r", "\n")
}
