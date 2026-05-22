package vectorstore

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pgvector/pgvector-go"
)

// PostgresStore implements Store using PostgreSQL + pgvector.
type PostgresStore struct {
	pool *pgxpool.Pool
}

// NewPostgresStore creates a PostgresStore from an existing pgxpool.
func NewPostgresStore(pool *pgxpool.Pool) *PostgresStore {
	return &PostgresStore{pool: pool}
}

// Connect opens a new pgxpool and returns a PostgresStore.
func Connect(ctx context.Context, url string, maxConns, minConns int32) (*PostgresStore, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("postgres: parse config: %w", err)
	}
	cfg.MaxConns = maxConns
	cfg.MinConns = minConns

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("postgres: create pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("postgres: ping: %w", err)
	}
	return NewPostgresStore(pool), nil
}

func (s *PostgresStore) Close() { s.pool.Close() }

// UpsertChunks inserts or updates chunks in a single batch transaction.
func (s *PostgresStore) UpsertChunks(ctx context.Context, chunks []Chunk) error {
	if len(chunks) == 0 {
		return nil
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("upsert chunks: begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	for _, c := range chunks {
		meta, _ := json.Marshal(c.Metadata)
		var vec pgvector.Vector
		if c.Embedding != nil {
			vec = pgvector.NewVector(c.Embedding)
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO kol_chunks
				(kol_name, doc_type, source_file, page_num, text, token_count, embedding, metadata)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
			ON CONFLICT (id) DO UPDATE SET
				text        = EXCLUDED.text,
				embedding   = EXCLUDED.embedding,
				token_count = EXCLUDED.token_count,
				metadata    = EXCLUDED.metadata`,
			c.KOLName, string(c.DocType), c.SourceFile, c.PageNum,
			c.Text, c.TokenCount, vec, meta,
		)
		if err != nil {
			return fmt.Errorf("upsert chunks: insert chunk: %w", err)
		}
	}
	return tx.Commit(ctx)
}

// SimilaritySearch performs HNSW-accelerated cosine similarity search.
// efSearch sets the beam width at query time; higher = better recall, slower.
func (s *PostgresStore) SimilaritySearch(
	ctx context.Context,
	queryVec []float32,
	filter SearchFilter,
	topK int,
	efSearch int,
) ([]ChunkResult, error) {
	// Set HNSW query-time beam width
	if _, err := s.pool.Exec(ctx, fmt.Sprintf("SET LOCAL hnsw.ef_search = %d", efSearch)); err != nil {
		return nil, fmt.Errorf("similarity search: set ef_search: %w", err)
	}

	vec := pgvector.NewVector(queryVec)

	rows, err := s.pool.Query(ctx, `
		SELECT
			c.id, c.kol_name, c.doc_type, c.source_file, c.page_num,
			c.text, c.token_count, c.metadata, c.created_at,
			c.embedding <-> $1 AS distance
		FROM kol_chunks c
		JOIN kol_profiles p ON p.name = c.kol_name
		WHERE
			($2::text[] IS NULL OR p.platform && $2) AND
			($3::text[] IS NULL OR p.category && $3)
		ORDER BY distance ASC
		LIMIT $4`,
		vec, filter.Platforms, filter.Categories, topK,
	)
	if err != nil {
		return nil, fmt.Errorf("similarity search: query: %w", err)
	}
	defer rows.Close()

	var results []ChunkResult
	for rows.Next() {
		var (
			c        Chunk
			meta     json.RawMessage
			distance float64
		)
		if err := rows.Scan(
			&c.ID, &c.KOLName, &c.DocType, &c.SourceFile, &c.PageNum,
			&c.Text, &c.TokenCount, &meta, &c.CreatedAt, &distance,
		); err != nil {
			return nil, fmt.Errorf("similarity search: scan row: %w", err)
		}
		if meta != nil {
			_ = json.Unmarshal(meta, &c.Metadata)
		}
		results = append(results, ChunkResult{
			Chunk:    c,
			Distance: distance,
			SimScore: 1.0 - distance/2.0,
		})
	}
	return results, rows.Err()
}

// UpsertKOLProfile stores or updates a KOL master profile.
func (s *PostgresStore) UpsertKOLProfile(ctx context.Context, p KOLProfile) error {
	meta, _ := json.Marshal(p.Metadata)
	_, err := s.pool.Exec(ctx, `
		INSERT INTO kol_profiles
			(name, platform, category, avg_engagement, avg_reach, avg_roi,
			 follower_count, fee_min_vnd, fee_max_vnd, metadata)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		ON CONFLICT (name) DO UPDATE SET
			platform       = EXCLUDED.platform,
			category       = EXCLUDED.category,
			avg_engagement = EXCLUDED.avg_engagement,
			avg_reach      = EXCLUDED.avg_reach,
			avg_roi        = EXCLUDED.avg_roi,
			follower_count = EXCLUDED.follower_count,
			fee_min_vnd    = EXCLUDED.fee_min_vnd,
			fee_max_vnd    = EXCLUDED.fee_max_vnd,
			metadata       = EXCLUDED.metadata,
			updated_at     = NOW()`,
		p.Name, p.Platform, p.Category, p.AvgEngagement, p.AvgReach, p.AvgROI,
		p.FollowerCount, p.FeeMinVND, p.FeeMaxVND, meta,
	)
	if err != nil {
		return fmt.Errorf("upsert kol profile: %w", err)
	}
	return nil
}

// GetKOLProfile retrieves a single KOL profile by name.
func (s *PostgresStore) GetKOLProfile(ctx context.Context, name string) (*KOLProfile, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, name, platform, category, avg_engagement, avg_reach, avg_roi,
		       follower_count, fee_min_vnd, fee_max_vnd, metadata, updated_at
		FROM kol_profiles WHERE name = $1`, name)

	p, err := scanKOLProfile(row)
	if err != nil {
		return nil, fmt.Errorf("get kol profile: %w", err)
	}
	return p, nil
}

// ListKOLProfilesByNames retrieves multiple KOL profiles by name slice.
func (s *PostgresStore) ListKOLProfilesByNames(ctx context.Context, names []string) ([]KOLProfile, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, name, platform, category, avg_engagement, avg_reach, avg_roi,
		       follower_count, fee_min_vnd, fee_max_vnd, metadata, updated_at
		FROM kol_profiles WHERE name = ANY($1)`, names)
	if err != nil {
		return nil, fmt.Errorf("list kol profiles: %w", err)
	}
	defer rows.Close()

	var profiles []KOLProfile
	for rows.Next() {
		p, err := scanKOLProfile(rows)
		if err != nil {
			return nil, err
		}
		profiles = append(profiles, *p)
	}
	return profiles, rows.Err()
}

// scanKOLProfile is a helper that scans a KOLProfile from any pgx.Row/Rows.
func scanKOLProfile(row interface {
	Scan(dest ...any) error
}) (*KOLProfile, error) {
	var (
		p    KOLProfile
		meta json.RawMessage
	)
	if err := row.Scan(
		&p.ID, &p.Name, &p.Platform, &p.Category,
		&p.AvgEngagement, &p.AvgReach, &p.AvgROI,
		&p.FollowerCount, &p.FeeMinVND, &p.FeeMaxVND,
		&meta, &p.UpdatedAt,
	); err != nil {
		return nil, err
	}
	if meta != nil {
		_ = json.Unmarshal(meta, &p.Metadata)
	}
	return &p, nil
}
