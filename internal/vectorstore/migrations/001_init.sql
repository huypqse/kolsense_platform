-- ─── Extensions ────────────────────────────────────────────────────────────────
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS vector;

-- ─── KOL master profiles ────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS kol_profiles (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name            TEXT NOT NULL UNIQUE,
    platform        TEXT[]      NOT NULL DEFAULT '{}',   -- tiktok | instagram | youtube
    category        TEXT[]      NOT NULL DEFAULT '{}',   -- beauty | fashion | tech | food | lifestyle
    avg_engagement  FLOAT       NOT NULL DEFAULT 0,      -- ratio, e.g. 0.065 = 6.5%
    avg_reach       BIGINT      NOT NULL DEFAULT 0,      -- average post reach
    avg_roi         FLOAT       NOT NULL DEFAULT 0,      -- historical ROI multiplier
    follower_count  BIGINT      NOT NULL DEFAULT 0,
    fee_min_vnd     BIGINT      NOT NULL DEFAULT 0,
    fee_max_vnd     BIGINT      NOT NULL DEFAULT 0,
    metadata        JSONB       NOT NULL DEFAULT '{}',
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_kol_profiles_platform ON kol_profiles USING GIN (platform);
CREATE INDEX IF NOT EXISTS idx_kol_profiles_category ON kol_profiles USING GIN (category);

-- ─── KOL document chunks + embeddings ──────────────────────────────────────────
CREATE TABLE IF NOT EXISTS kol_chunks (
    id          UUID        PRIMARY KEY DEFAULT uuid_generate_v4(),
    kol_name    TEXT        NOT NULL REFERENCES kol_profiles(name) ON DELETE CASCADE,
    doc_type    TEXT        NOT NULL,   -- campaign_report | kol_performance | audience_insight | brand_guideline
    source_file TEXT        NOT NULL,
    page_num    INT         NOT NULL DEFAULT 0,
    text        TEXT        NOT NULL,
    token_count INT         NOT NULL DEFAULT 0,
    embedding   VECTOR(1024),
    metadata    JSONB       NOT NULL DEFAULT '{}',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- HNSW index tuned for 2M vectors at 1024 dims
-- m=16: connections per node; ef_construction=128: build-time recall
CREATE INDEX IF NOT EXISTS idx_kol_chunks_embedding
    ON kol_chunks USING hnsw (embedding vector_cosine_ops)
    WITH (m = 16, ef_construction = 128);

CREATE INDEX IF NOT EXISTS idx_kol_chunks_kol_name ON kol_chunks (kol_name);
CREATE INDEX IF NOT EXISTS idx_kol_chunks_doc_type  ON kol_chunks (doc_type);

-- ─── Brief analysis sessions ────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS brief_sessions (
    id               UUID        PRIMARY KEY DEFAULT uuid_generate_v4(),
    raw_text         TEXT        NOT NULL,
    parsed_brief     JSONB       NOT NULL DEFAULT '{}',
    shortlist        JSONB       NOT NULL DEFAULT '[]',
    report_markdown  TEXT        NOT NULL DEFAULT '',
    parser_strategy  TEXT        NOT NULL DEFAULT 'rule_based', -- rule_based | llm_fallback
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
