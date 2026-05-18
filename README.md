# KolSense Platform

> AI-powered KOL Matching & Recommendation System for E-commerce

KolSense is a RAG (Retrieval-Augmented Generation) platform that matches KOL (Key Opinion Leaders) to campaign briefs using vector similarity search and LLM-generated reports in Vietnamese.

---

## Architecture

```
Tầng 1 — Data Ingestion:    PDF → pdfcpu parser → chunker → embedder → pgvector
Tầng 2 — RAG Core Engine:   Brief → embed → pgvector search → score → Qwen LLM
Tầng 3 — Output & API:      REST API → JSON shortlist + Markdown report
```

See [architecture.md](docs/architecture.md) for the full technical specification.

---

## Quick Start

### Prerequisites
- Go 1.23+
- Docker & Docker Compose

### 1. Set up environment
```bash
cp .env.example .env
# Edit .env — set DATABASE_URL, provider, etc.
```

### 2. Start infrastructure
```bash
make up          # starts postgres/pgvector + ollama
make pull-models # pulls bge-m3 and qwen2.5:14b into ollama
```

### 3. Ingest sample PDFs
```bash
make ingest      # ingests all PDFs in ./data/
```

### 4. Run the API server
```bash
make run         # builds and starts on :8080
# or for live-reload:
make dev
```

### 5. Test the pipeline
```bash
curl -X POST http://localhost:8080/api/v1/brief/analyze \
  -H 'Content-Type: application/json' \
  -d '{
    "brief_text": "Chiến dịch beauty tháng 6 trên TikTok và Instagram. Ngân sách 100 triệu - 200 triệu. Đối tượng nữ tuổi 18-35 tại HCM. Mục tiêu brand awareness.",
    "top_k": 5
  }'
```

---

## Project Structure

```
rag-systems/
├── cmd/
│   ├── api/            # HTTP API server entrypoint
│   └── ingest/         # CLI ingestion tool
├── internal/
│   ├── config/         # Viper-based config (env vars + .env)
│   ├── embedding/      # Embedder interface, Ollama + DashScope impls
│   ├── llm/            # LLM client interface, Ollama + DashScope impls
│   ├── vectorstore/    # pgvector Store interface + PostgreSQL impl
│   ├── ingestion/      # PDF parser, chunker, ingestion service
│   ├── brief/          # Campaign brief models + hybrid parser
│   ├── search/         # Similarity searcher (embed + filter + ANN)
│   ├── scoring/        # Two-phase KOL scorer + reranker
│   ├── prompt/         # RAG prompt builder (Go templates)
│   └── api/            # Gin server, handlers, middleware
├── data/               # Sample PDF documents
├── docker-compose.yml
├── Makefile
└── .env.example
```

---

## API Endpoints

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/v1/brief/analyze` | Analyze a campaign brief, returns KOL shortlist + report |
| `GET`  | `/api/v1/brief/:id/report` | Retrieve a past analysis session |
| `GET`  | `/api/v1/kol/:name` | Fetch KOL profile |
| `POST` | `/api/v1/ingest/pdf` | Trigger PDF ingestion |
| `GET`  | `/api/v1/health` | Health check |

---

## Provider Configuration

| Setting | Ollama (local dev) | DashScope (production) | Gemini (API) |
|---------|-------------------|----------------------|--------------|
| `EMBEDDING_PROVIDER` | `ollama` | `dashscope` | `ollama` or `dashscope` |
| `LLM_PROVIDER` | `ollama` | `dashscope` | `gemini` |
| Model (embed) | `bge-m3` | `text-embedding-v3` | N/A |
| Model (LLM) | `qwen2.5:14b` | `qwen-plus` | `gemini-2.5-flash` |

---

## Tech Stack

- **Go 1.23** · **Gin** · **pgx/v5** · **pgvector-go** · **pdfcpu** · **Viper** · **Zap**
- **PostgreSQL 16 + pgvector** (HNSW index, m=16, ef_construction=128)
- **Ollama** (local) / **Alibaba DashScope** (production)
